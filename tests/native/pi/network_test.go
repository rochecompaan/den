//go:build native

package pi

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type dnsFixture struct {
	connection *net.UDPConn
	mu         sync.Mutex
	queries    []string
	done       chan struct{}
}

func startDNSFixture(t *testing.T) *dnsFixture {
	t.Helper()
	port := os.Getenv("DEN_NATIVE_DNS_PORT")
	if port == "" {
		port = "53"
	}
	address, err := net.ResolveUDPAddr("udp4", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		t.Fatal(err)
	}
	connection, err := net.ListenUDP("udp4", address)
	if err != nil {
		t.Fatalf("start loopback DNS fixture: %v", err)
	}
	fixture := &dnsFixture{connection: connection, done: make(chan struct{})}
	go fixture.serve()
	return fixture
}

func (fixture *dnsFixture) close() {
	if fixture == nil {
		return
	}
	_ = fixture.connection.Close()
	<-fixture.done
}

func (fixture *dnsFixture) names() []string {
	if fixture == nil {
		return nil
	}
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return append([]string(nil), fixture.queries...)
}

func (fixture *dnsFixture) serve() {
	defer close(fixture.done)
	buffer := make([]byte, 512)
	for {
		count, client, err := fixture.connection.ReadFromUDP(buffer)
		if err != nil {
			return
		}
		response := fixture.response(buffer[:count])
		if response != nil {
			_, _ = fixture.connection.WriteToUDP(response, client)
		}
	}
}

func (fixture *dnsFixture) response(request []byte) []byte {
	name, questionEnd, ok := dnsQuestion(request)
	if !ok {
		return nil
	}
	fixture.mu.Lock()
	fixture.queries = append(fixture.queries, name)
	fixture.mu.Unlock()

	allowed := name == brokerHostname() || name == "registry.npmjs.org" || name == "api.anthropic.com" || name == "api.github.com" || name == "github.com"
	response := append([]byte(nil), request[:questionEnd]...)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[6:8], 0)
	if !allowed {
		binary.BigEndian.PutUint16(response[2:4], 0x8183)
		return response
	}
	binary.BigEndian.PutUint16(response[6:8], 1)
	answer := []byte{
		0xc0, 0x0c,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x04,
		127, 0, 0, 1,
	}
	return append(response, answer...)
}

func dnsQuestion(request []byte) (string, int, bool) {
	if len(request) < 17 || binary.BigEndian.Uint16(request[4:6]) != 1 {
		return "", 0, false
	}
	labels := make([]string, 0, 4)
	position := 12
	for {
		if position >= len(request) {
			return "", 0, false
		}
		length := int(request[position])
		position++
		if length == 0 {
			break
		}
		if length > 63 || position+length > len(request) {
			return "", 0, false
		}
		labels = append(labels, string(request[position:position+length]))
		position += length
	}
	if position+4 > len(request) {
		return "", 0, false
	}
	return strings.ToLower(strings.Join(labels, ".")), position + 4, true
}

func TestPiProviderRouteAndDirectNetworkDenials(t *testing.T) {
	fixture := newPiFixture(t)
	network := startPiNetwork(t, fixture)
	result := fixture.enforcementProbe(t, `
 const url = "https://registry.npmjs.org:38414/provider";
 const allowed = run("curl", ["--fail", "--silent", "--show-error", "--max-time", "3", "--cacert", process.env.REPOWOLF_CA_FILE!, url]);
 assert.equal(allowed.status, 0, "provider route: " + allowed.stderr);
 assert.equal(allowed.stdout, "local fixture\n"); record("provider-routed");
 for (const host of ["api.anthropic.com", "api.github.com", "github.com", "registry.npmjs.org"]) {
  const direct = run("curl", ["--noproxy", "*", "--silent", "--max-time", "2", "--cacert", process.env.REPOWOLF_CA_FILE!, "--resolve", host + ":38414:127.0.0.1", "https://" + host + ":38414/direct"]);
  assert.notEqual(direct.status, 0, "direct route escaped: " + host); record("direct-denied:" + host);
 }
 const github = run("curl", ["--fail", "--silent", "--max-time", "2", "https://api.github.com:38414/github"]);
 assert.notEqual(github.status, 0); record("github-proxy-denied");
 const net = await import("node:net");
 await new Promise<void>((resolve, reject) => {
  const socket = net.connect(38416, "127.0.0.1");
  socket.setTimeout(1500); socket.once("connect", () => {socket.destroy(); reject(new Error("arbitrary TCP escaped"));});
  socket.once("error", () => resolve()); socket.once("timeout", () => {socket.destroy(); resolve();});
 }); record("tcp-denied");
 `, network.environment())
	requireReportLines(t, filepath.Join(fixture.agentDir, "enforcement.report"), "provider-routed", "github-proxy-denied", "tcp-denied")
	network.mu.Lock()
	defer network.mu.Unlock()
	if strings.Join(network.requests, ",") != "registry.npmjs.org:38414/provider" {
		t.Fatalf("unexpected routed requests: %v", network.requests)
	}
	if network.tcpConnections.Load() != 0 {
		t.Fatal("direct TCP reached live recorder")
	}
	requireNoCredential(t, result, fixtureToken)
}

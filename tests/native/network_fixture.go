//go:build native

package native

import (
	"encoding/binary"
	"net"
	"os"
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

	allowed := name == brokerHostname() || name == "registry.npmjs.org"
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

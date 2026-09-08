//go:build native

package pi

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fixtureToken = "rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE"

type certificateFiles struct{ ca, certificate, key string }
type piNetwork struct {
	fixture        *piFixture
	certificate    certificateFiles
	operations     string
	mu             sync.Mutex
	requests       []string
	tcpConnections atomic.Int32
}

func brokerHostname() string {
	if runtime.GOOS == "darwin" {
		return "registry.npmjs.org"
	}
	return "broker.den.invalid"
}
func (network *piNetwork) environment() []string {
	return []string{"REPOWOLF_ENDPOINT=https://" + brokerHostname() + ":38413", "REPOWOLF_CA_FILE=" + network.certificate.ca,
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "ALL_PROXY=http://127.0.0.1:1", "NO_PROXY=", "no_proxy="}
}
func startPiNetwork(t *testing.T, fixture *piFixture) *piNetwork {
	t.Helper()
	network := &piNetwork{fixture: fixture, certificate: generateCertificate(t, filepath.Join(fixture.root, "tls")), operations: filepath.Join(fixture.root, "operations")}
	dns := startDNSFixture(t)
	t.Cleanup(dns.close)
	remote := filepath.Join(fixture.root, "remote.git")
	command := exec.Command("git", "-c", "init.defaultBranch=main", "init", "--bare", remote)
	command.Env = fixture.environment("GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_COUNT=0", "GIT_CONFIG_PARAMETERS=")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("init fixture: %v %s", err, output)
	}
	command = exec.Command(os.Getenv("DEN_NATIVE_REPOWOLF_FIXTURE"), "--listen", "127.0.0.1:38413", "--certificate", network.certificate.certificate, "--key", network.certificate.key, "--remote", remote, "--log", network.operations)
	var errors bytes.Buffer
	command.Stderr = &errors
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill(); _ = command.Wait() })
	roots := x509.NewCertPool()
	data, _ := os.ReadFile(network.certificate.ca)
	roots.AppendCertsFromPEM(data)
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		connection, err := tls.Dial("tcp", "127.0.0.1:38413", &tls.Config{RootCAs: roots, ServerName: brokerHostname(), MinVersion: tls.VersionTLS13})
		if err == nil {
			connection.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("RepoWolf readiness timeout")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:38414")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		network.mu.Lock()
		network.requests = append(network.requests, r.Host+r.URL.Path)
		network.mu.Unlock()
		_, _ = w.Write([]byte("local fixture\n"))
	})}
	t.Cleanup(func() { _ = server.Close() })
	go func() { _ = server.ServeTLS(listener, network.certificate.certificate, network.certificate.key) }()
	tcp, err := net.Listen("tcp4", "127.0.0.1:38416")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tcp.Close() })
	go func() {
		for {
			conn, err := tcp.Accept()
			if err != nil {
				return
			}
			network.tcpConnections.Add(1)
			conn.Close()
		}
	}()
	// An invoking-side control proves direct denials cannot pass due to a dead server.
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13}}}
	response, err := client.Get("https://127.0.0.1:38414/control")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	network.mu.Lock()
	network.requests = nil
	network.mu.Unlock()
	return network
}

func TestPiRepoWolfHelpersAreTheOnlyGitHubRoute(t *testing.T) {
	fixture := newPiFixture(t)
	network := startPiNetwork(t, fixture)
	result := fixture.enforcementProbe(t, `
 const gh = run("gh", ["issue", "list", "--repo", "alpha/repo"]);
 assert.notEqual(gh.status, 0); assert.match(gh.stderr, /GitHub operation failed/); record("github-brokered");
 for (const remote of ["https://github.com/alpha/repo.git", "git@github.com:alpha/repo.git"]) {
  const git = run("git", ["ls-remote", remote]); assert.equal(git.status, 0, git.stderr); record("git-brokered:" + remote);
 }
 const directGit = run("git", ["-c", "url.https://github.com/.insteadOf=", "ls-remote", "git://127.0.0.1:38416/alpha/repo.git"]);
 assert.notEqual(directGit.status, 0); record("direct-git-denied");
 const ssh = run("ssh", ["-F", "/dev/null", "-o", "BatchMode=yes", "-o", "ConnectTimeout=2", "-p", "38416", "git@127.0.0.1", "git-upload-pack alpha/repo.git"]);
 assert.notEqual(ssh.status, 0); assert.ok(!ssh.error, "packaged SSH must execute"); record("direct-ssh-denied");
 `, network.environment())
	contents, err := os.ReadFile(network.operations)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "gh\ngit-upload-pack\ngit-upload-pack\n" {
		t.Fatalf("unexpected RepoWolf operations: %q", contents)
	}
	if network.tcpConnections.Load() != 0 {
		t.Fatal("direct Git/SSH reached live listener")
	}
	requireNoCredential(t, result, fixtureToken)
	if strings.Contains(string(contents), fixtureToken) {
		t.Fatal("RepoWolf telemetry leaked fixture token")
	}
}

func generateCertificate(t *testing.T, directory string) certificateFiles {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(-time.Minute)
	caKey := newKey(t)
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Den native fixture CA"},
		NotBefore: now, NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER := createCertificate(t, ca, ca, &caKey.PublicKey, caKey)
	serverKey := newKey(t)
	server := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "localhost"},
		DNSNames:    []string{"localhost", "broker.localhost", "broker.den.invalid", "registry.npmjs.org", "github.com", "gitlab.com", "bitbucket.org"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: now, NotAfter: now.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature,
	}
	serverDER := createCertificate(t, server, ca, &serverKey.PublicKey, caKey)
	files := certificateFiles{
		ca: filepath.Join(directory, "ca.pem"), certificate: filepath.Join(directory, "server.pem"), key: filepath.Join(directory, "server-key.pem"),
	}
	writePEM(t, files.ca, 0o600, "CERTIFICATE", caDER)
	writePEM(t, files.certificate, 0o600, "CERTIFICATE", serverDER)
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, files.key, 0o600, "PRIVATE KEY", keyDER)
	return files
}

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func createCertificate(t *testing.T, template, parent *x509.Certificate, public, signer any) []byte {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, template, parent, public, signer)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

func writePEM(t *testing.T, path string, mode os.FileMode, kind string, der []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		t.Fatal(err)
	}
	if err := pem.Encode(file, &pem.Block{Type: kind, Bytes: der}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

//go:build native

package native

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	fixtureToken    = "rw1_AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE"
	repoWolfPort    = "38413"
	tlsRecorderPort = "38414"
	containerSocket = "/tmp/den-native-container.sock"
)

type certificateFiles struct {
	ca, certificate, key string
}

type nativeFixture struct {
	root, worktree, home, remote, operations string
	certificate                              certificateFiles
	repoWolfEndpoint                         string
	repoWolf                                 *exec.Cmd
	tlsServer                                *http.Server
	tlsListener                              net.Listener
	containerListener                        net.Listener
	dns                                      *dnsFixture
	provider                                 *claudeProvider
	requestsMu                               sync.Mutex
	requests                                 []string
}

type commandResult struct {
	stdout, stderr string
	err            error
}

func newNativeFixture(t *testing.T) *nativeFixture {
	t.Helper()
	base := os.Getenv("DEN_NATIVE_HOST_ROOT")
	if base == "" {
		base = "/tmp"
	}
	root, err := os.MkdirTemp(base, "den-native-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture := &nativeFixture{
		root: root, worktree: filepath.Join(root, "worktree"), home: filepath.Join(root, "home"),
		remote: filepath.Join(root, "remote.git"), operations: filepath.Join(root, "repowolf.operations"),
		provider: newClaudeProvider(),
	}
	t.Cleanup(func() {
		fixture.close()
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove fixture: %v", err)
		}
	})
	for _, directory := range []string{fixture.worktree, fixture.home} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixture.certificate = generateCertificate(t, filepath.Join(root, "tls"))
	fixture.initializeRemote(t)
	fixture.startContainerSocket(t)
	fixture.startRepoWolf(t)
	fixture.startTLSRecorder(t)
	return fixture
}

func (fixture *nativeFixture) close() {
	fixture.dns.close()
	if fixture.tlsServer != nil {
		_ = fixture.tlsServer.Close()
	}
	if fixture.containerListener != nil {
		_ = fixture.containerListener.Close()
		_ = os.Remove(containerSocket)
	}
	if fixture.repoWolf != nil && fixture.repoWolf.Process != nil {
		_ = fixture.repoWolf.Process.Kill()
		_ = fixture.repoWolf.Wait()
	}
}

func (fixture *nativeFixture) initializeRemote(t *testing.T) {
	t.Helper()
	fixture.hostGit(t, fixture.root, "init", "--bare", fixture.remote)
	seed := filepath.Join(fixture.root, "seed")
	fixture.hostGit(t, fixture.root, "init", seed)
	fixture.hostGit(t, seed, "config", "user.name", "Den native fixture")
	fixture.hostGit(t, seed, "config", "user.email", "native@invalid")
	if err := os.WriteFile(filepath.Join(seed, "seed"), []byte("native\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture.hostGit(t, seed, "add", "seed")
	fixture.hostGit(t, seed, "commit", "-m", "native fixture")
	fixture.hostGit(t, seed, "branch", "-M", "main")
	fixture.hostGit(t, seed, "push", fixture.remote, "main")
	fixture.hostGit(t, fixture.remote, "symbolic-ref", "HEAD", "refs/heads/main")
	fixture.hostGit(t, fixture.worktree, "init")
}

func (fixture *nativeFixture) hostGit(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = replaceEnvironment(os.Environ(),
		"HOME="+fixture.home, "XDG_CONFIG_HOME="+filepath.Join(fixture.home, ".config"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_COUNT=0", "GIT_CONFIG_PARAMETERS=",
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func (fixture *nativeFixture) startDNS(t *testing.T) {
	t.Helper()
	fixture.dns = startDNSFixture(t)
}

func (fixture *nativeFixture) startContainerSocket(t *testing.T) {
	t.Helper()
	_ = os.Remove(containerSocket)
	listener, err := net.Listen("unix", containerSocket)
	if err != nil {
		t.Fatalf("start check-only container socket fixture: %v", err)
	}
	fixture.containerListener = listener
}

func (fixture *nativeFixture) startRepoWolf(t *testing.T) {
	t.Helper()
	address := net.JoinHostPort("127.0.0.1", repoWolfPort)
	fixture.repoWolfEndpoint = "https://" + brokerHostname() + ":" + repoWolfPort
	fixture.repoWolf = exec.Command(os.Getenv("DEN_NATIVE_REPOWOLF_FIXTURE"),
		"--listen", address,
		"--certificate", fixture.certificate.certificate,
		"--key", fixture.certificate.key,
		"--remote", fixture.remote,
		"--log", fixture.operations,
	)
	fixture.repoWolf.Env = os.Environ()
	var stderr bytes.Buffer
	fixture.repoWolf.Stderr = &stderr
	if err := fixture.repoWolf.Start(); err != nil {
		t.Fatal(err)
	}
	waitForTLS(t, address, fixture.certificate.ca, &stderr, fixture.repoWolf)
}

func (fixture *nativeFixture) startTLSRecorder(t *testing.T) {
	t.Helper()
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", tlsRecorderPort))
	if err != nil {
		t.Fatal(err)
	}
	fixture.tlsListener = listener
	fixture.tlsServer = &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/v1/messages") {
			fixture.provider.serveHTTP(writer, request)
			return
		}
		fixture.requestsMu.Lock()
		fixture.requests = append(fixture.requests, request.Host)
		fixture.requestsMu.Unlock()
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("local fixture\n"))
	})}
	go func() {
		_ = fixture.tlsServer.ServeTLS(listener, fixture.certificate.certificate, fixture.certificate.key)
	}()
}

func (fixture *nativeFixture) tlsPort(t *testing.T) string {
	t.Helper()
	return portOf(t, fixture.tlsListener.Addr().String())
}

func (fixture *nativeFixture) requestCount() int {
	fixture.requestsMu.Lock()
	defer fixture.requestsMu.Unlock()
	return len(fixture.requests)
}

func (fixture *nativeFixture) launch(arguments ...string) commandResult {
	return fixture.launchWith(nil, arguments...)
}

func (fixture *nativeFixture) launchWith(extraEnvironment []string, arguments ...string) commandResult {
	command := exec.Command(os.Getenv("DEN_NATIVE_SANDBOX"), arguments...)
	command.Dir = fixture.worktree
	command.Env = fixture.launchEnvironment(extraEnvironment)
	return runCommand(command)
}

func (fixture *nativeFixture) launchEnvironment(extra []string) []string {
	environment := []string{
		"HOME=" + fixture.home,
		"REPOWOLF_ENDPOINT=" + fixture.repoWolfEndpoint,
		"REPOWOLF_TOKEN=" + fixtureToken,
		"REPOWOLF_CA_FILE=" + fixture.certificate.ca,
		"HTTP_PROXY=http://127.0.0.1:1", "HTTPS_PROXY=http://127.0.0.1:1", "ALL_PROXY=http://127.0.0.1:1",
		"NO_PROXY=", "no_proxy=",
	}
	return replaceEnvironment(os.Environ(), append(environment, extra...)...)
}

func runCommand(command *exec.Cmd) commandResult {
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return commandResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
}

func replaceEnvironment(base []string, replacements ...string) []string {
	values := make(map[string]string, len(replacements))
	for _, entry := range replacements {
		name, _, _ := strings.Cut(entry, "=")
		values[name] = entry
	}
	result := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "CLAUDE_CONFIG_DIR" {
			continue
		}
		if _, replaced := values[name]; !replaced {
			result = append(result, entry)
		}
	}
	for _, entry := range values {
		result = append(result, entry)
	}
	return result
}

func requireSuccess(t *testing.T, result commandResult) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("native launch failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
	}
}

func reserveAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func portOf(t *testing.T, address string) string {
	t.Helper()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func waitForTLS(t *testing.T, address, caFile string, stderr *bytes.Buffer, command *exec.Cmd) {
	t.Helper()
	caPEM, err := os.ReadFile(caFile)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("load fixture CA")
	}
	configuration := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: brokerHostname()}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		connection, dialErr := tls.Dial("tcp", address, configuration)
		if dialErr == nil {
			_ = connection.Close()
			return
		}
		if command.ProcessState != nil {
			t.Fatalf("RepoWolf fixture exited: %s", stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("RepoWolf fixture TLS readiness timeout: %s", stderr.String())
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

func fixtureURL(port, host string) string {
	return fmt.Sprintf("https://%s:%s/", host, port)
}

func brokerHostname() string {
	if runtime.GOOS == "linux" {
		return "broker.den.invalid"
	}
	return "registry.npmjs.org"
}

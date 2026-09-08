//go:build native

package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPiExactRuntimeEnvironmentAndScrubbing(t *testing.T) {
	fixture := newPiFixture(t)
	secret := "den9-synthetic-secret-never-log"
	extra := []string{"ANTHROPIC_API_KEY=" + secret, "HTTP_PROXY=http://runtime-proxy.invalid:8123", "HTTPS_PROXY=http://runtime-proxy.invalid:8124", "ALL_PROXY=socks5://runtime-proxy.invalid:8125", "NO_PROXY=runtime.invalid", "REPOWOLF_UNSAFE=" + secret, "REPOWOLF_CLIENT_DIR=/untrusted", "PI_PACKAGE_DIR=/untrusted", "PI_OFFLINE=0", "NODE_EXTRA_CA_CERTS=/untrusted"}
	scrubbed := []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "SSH_AUTH_SOCK", "GIT_ASKPASS", "SSH_ASKPASS", "GIT_SSH", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_KEY_99", "GIT_CONFIG_VALUE_99"}
	for _, name := range scrubbed {
		extra = append(extra, name+"="+secret)
	}
	expected := map[string]string{
		"PI_CODING_AGENT_DIR": fixture.agentDir, "PI_CODING_AGENT_SESSION_DIR": fixture.sessionDir, "PI_PACKAGE_DIR": os.Getenv("DEN_NATIVE_PI_PACKAGE_ROOT"), "PI_OFFLINE": "1",
		"REPOWOLF_ENDPOINT": "https://broker.example.test/", "REPOWOLF_TOKEN": fixtureToken, "REPOWOLF_CLIENT_DIR": os.Getenv("DEN_NATIVE_REPOWOLF_CLIENT_DIR"),
		"ANTHROPIC_API_KEY":   secret,
		"GIT_TERMINAL_PROMPT": "0", "GIT_CONFIG_COUNT": "3", "GIT_CONFIG_KEY_0": "url.git@github.com:.insteadOf", "GIT_CONFIG_VALUE_0": "https://github.com/", "GIT_CONFIG_KEY_1": "credential.helper", "GIT_CONFIG_VALUE_1": "", "GIT_CONFIG_KEY_2": "core.sshCommand",
	}
	for _, name := range []string{"GIT_SSH_COMMAND", "GIT_CONFIG_VALUE_2"} {
		expected[name] = filepath.Join(os.Getenv("DEN_NATIVE_REPOWOLF_CLIENT_DIR"), "bin/repowolf-git-ssh")
	}
	inputReport := filepath.Join(fixture.root, "fence-input.report")
	boundary := fixture.launch("", append(extra, "DEN_NATIVE_PI_FENCE_INPUT_REPORT="+inputReport),
		func(document map[string]any) {
			document["fenceExecutable"] = os.Getenv("DEN_NATIVE_PI_FENCE_INPUT_RECORDER")
		}, "--version")
	if boundary.err != nil || strings.TrimSpace(boundary.stdout) != "0.84.4" {
		t.Fatalf("real Fence input probe failed: %v %s%s", boundary.err, boundary.stdout, boundary.stderr)
	}
	input, err := os.ReadFile(inputReport)
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != "HTTP_PROXY=http://runtime-proxy.invalid:8123\nHTTPS_PROXY=http://runtime-proxy.invalid:8124\nALL_PROXY=socks5://runtime-proxy.invalid:8125\nNO_PROXY=runtime.invalid\n" {
		t.Fatalf("ordinary proxy inputs changed before real Fence: %q", input)
	}
	encoded, _ := json.Marshal(expected)
	blocked, _ := json.Marshal(append(scrubbed, "REPOWOLF_UNSAFE"))
	// Expected synthetic values are inputs only: the report never serializes them.
	body := `const expected = ` + string(encoded) + `; const blocked = ` + string(blocked) + `;
 for (const [key,value] of Object.entries(expected)) assert.ok(process.env[key] === value, "environment mismatch: " + key);
 for (const key of blocked) assert.ok(!(key in process.env), "unscrubbed name: " + key);
 for (const [key,value] of Object.entries({HTTP_PROXY:"http://runtime-proxy.invalid:8123",HTTPS_PROXY:"http://runtime-proxy.invalid:8124",ALL_PROXY:"socks5://runtime-proxy.invalid:8125",NO_PROXY:"runtime.invalid"})) assert.notEqual(process.env[key], value, "inherited proxy bypassed Fence: " + key);
 assert.equal(process.env.NODE_EXTRA_CA_CERTS, process.env.REPOWOLF_CA_FILE);
 assert.ok(process.env.REPOWOLF_CA_FILE !== ` + jsonString(fixture.caFile) + `);
 assert.equal(fs.readFileSync(process.env.REPOWOLF_CA_FILE!, "utf8"), "fixture CA\\n");
 const environment = run("env", []); assert.equal(environment.status, 0, "controlled PATH must execute env");
 const names = environment.stdout.trim().split("\n").map(line => line.split("=",1)[0]);
 for (const key of [...Object.keys(expected), "NODE_EXTRA_CA_CERTS", "REPOWOLF_CA_FILE"]) assert.equal(names.filter(name=>name===key).length,1,"duplicate environment: " + key);
 record("environment-exact");`
	result := fixture.enforcementProbe(t, body, extra)
	requireNoCredential(t, result, secret)
	requireNoCredential(t, result, fixtureToken)
	contents, err := os.ReadFile(filepath.Join(fixture.agentDir, "enforcement.report"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), secret) || strings.Contains(string(contents), fixtureToken) {
		t.Fatal("probe telemetry leaked secret")
	}
	for _, name := range []string{"REPOWOLF_ENDPOINT", "REPOWOLF_TOKEN", "REPOWOLF_CA_FILE"} {
		result := fixture.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), "", []string{name + "=" + secret}, "--version")
		requireDenied(t, result)
		requireNoCredential(t, result, secret)
		requireNoCredential(t, result, fixtureToken)
	}
}
func jsonString(value string) string { encoded, _ := json.Marshal(value); return string(encoded) }

func TestPiWrapperPrecedesExtraPackage(t *testing.T) {
	fixture := newPiFixture(t)
	result := fixture.sandbox("", "--version")
	if result.err != nil || strings.TrimSpace(result.stdout) != "0.84.4" {
		t.Fatalf("extra package replaced wrapper: %v %s%s", result.err, result.stdout, result.stderr)
	}
	// The fixture supplies an actual conflicting extra package. Verify that it is
	// present rather than letting the precedence assertion pass vacuously.
	fixture.enforcementProbe(t, `
 const fake = run("den9-extra-present", []);
 assert.equal(fake.status, 0, "conflicting extra package must be installed");
 assert.equal(fake.stdout, "extra-package\n"); record("extra-package-present");
 `, nil)
}

func TestPiExtensionProcessesKeepOuterControls(t *testing.T) {
	fixture := newPiFixture(t)
	outside := filepath.Join(fixture.root, "process-secret")
	if err := os.WriteFile(outside, []byte("den9-process-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := fixture.enforcementProbe(t, `
 const allowed = run(process.execPath, ["-e", "require('fs').writeFileSync('child-control','allowed')"]);
 assert.equal(allowed.status,0,allowed.stderr);
 const denied = run(process.execPath,["-e", "require('fs').readFileSync(process.argv[1])", `+jsonString(outside)+`]);
 assert.notEqual(denied.status,0); assert.match(denied.stderr,/EACCES|EPERM|ENOENT/); record("extension-filesystem-denied");
 const child = run(process.execPath,["-e", "const s=require('net').connect(38416,'127.0.0.1'); s.on('connect',()=>process.exit(42)); s.on('error',()=>process.exit(0)); setTimeout(()=>process.exit(0),1000)"]);
 assert.equal(child.status,0); record("extension-network-denied");
 // Deliberately no argv-aware claim for arbitrary extension processes: this
 // uses a shared Node binary, not Pi bash/user_bash command authorization.
 `, nil)
	if data, err := os.ReadFile(filepath.Join(fixture.worktree, "child-control")); err != nil || string(data) != "allowed" {
		t.Fatal("extension control did not run")
	}
	requireNoCredential(t, result, "den9-process-secret")
}

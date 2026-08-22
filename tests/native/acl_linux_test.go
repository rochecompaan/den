//go:build native && linux

package native

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testValidationTimePathSwap(t *testing.T, fixture *nativeFixture) {
	t.Helper()
	probe := os.Getenv("DEN_NATIVE_GETFACL")
	if probe == "" {
		t.Fatal("DEN_NATIVE_GETFACL is required")
	}
	mutation := os.Getenv("DEN_NATIVE_ACL")
	if mutation == "" {
		t.Fatal("DEN_NATIVE_ACL is required")
	}
	validationTimePathSwap(t, fixture, []string{probe}, []string{mutation, "-m", "d:g::r-x"})
}

func launchRegistryFixture(t *testing.T, fixture *nativeFixture, port string) commandResult {
	t.Helper()
	return fixture.launch("network-allow", fixtureURL(port, "registry.npmjs.org"))
}

func testImplicitHostWrites(t *testing.T, fixture *nativeFixture) {
	t.Helper()
	parent := "/tmp/fence"
	if err := os.Mkdir(parent, 0o700); err != nil && !os.IsExist(err) {
		t.Fatal("outside-Fence host temporary control is unavailable")
	}
	sentinel := filepath.Join(parent, "den-native-host-sentinel")
	child := filepath.Join(parent, "den-native-host-child")
	baseline := []byte("host-sentinel\n")
	if err := os.WriteFile(sentinel, baseline, 0o600); err != nil {
		t.Fatal("outside-Fence host sentinel is not writable")
	}
	t.Cleanup(func() {
		_ = os.Remove(child)
		_ = os.Remove(sentinel)
		_ = os.Remove(parent)
	})
	result := launchWithManifest(t, fixture, func(document map[string]any) {
		document["basePolicy"] = linuxTmpfsControlPolicy(t, fixture, document["basePolicy"].(string))
	}, "implicit-host-linux", sentinel, child)
	requireSuccess(t, result)
	contents, err := os.ReadFile(sentinel)
	if err != nil || string(contents) != string(baseline) {
		t.Fatal("Fence inner tmpfs mutated the host temporary sentinel")
	}
	if fileExists(child) {
		t.Fatal("Fence inner tmpfs created a host temporary child")
	}
}

func linuxTmpfsControlPolicy(t *testing.T, fixture *nativeFixture, base string) string {
	t.Helper()
	contents, err := os.ReadFile(base)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(contents, &document); err != nil {
		t.Fatal(err)
	}
	filesystem := document["filesystem"].(map[string]any)
	entries := filesystem["denyWrite"].([]any)
	filtered := entries[:0]
	for _, entry := range entries {
		path, _ := entry.(string)
		if !strings.HasPrefix(path, "/tmp/fence") {
			filtered = append(filtered, entry)
		}
	}
	filesystem["denyWrite"] = filtered
	delete(document["command"].(map[string]any), "runtimeExecPolicy")
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(fixture.root, "linux-tmpfs-control-policy.json")
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLinuxACLGrantRejected(t *testing.T) {
	fixture := newNativeFixture(t)
	state := os.Getenv("DEN_NATIVE_ACL_FIXTURE")
	if state == "" || !filepath.IsAbs(state) {
		t.Fatal("DEN_NATIVE_ACL_FIXTURE is required; Linux ACL checks never skip")
	}
	probe := os.Getenv("DEN_NATIVE_GETFACL")
	if probe == "" {
		t.Fatal("DEN_NATIVE_GETFACL is required; Linux ACL checks never skip")
	}
	output, err := exec.Command(probe, "-cp", state).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect real POSIX ACL: %v\n%s", err, output)
	}
	acl := string(output)
	if !strings.Contains(acl, "user:nobody:r-x") &&
		!strings.Contains(acl, "user:65534:r-x") &&
		!strings.Contains(acl, "user:4294967295:r-x") {
		t.Fatalf("runner did not provide the required u:nobody:r-x ACL: %s", acl)
	}
	marker := filepath.Join(fixture.worktree, "acl-started")
	result := fixture.launchWith([]string{"CLAUDE_CONFIG_DIR=" + state}, "marker", marker)
	if result.err == nil || fileExists(marker) {
		t.Fatalf("named-user POSIX ACL grant did not fail closed: %v\n%s", result.err, result.stderr)
	}
}

func TestLinuxFeaturePreflight(t *testing.T) {
	output, err := exec.Command(os.Getenv("DEN_NATIVE_FENCE"), "--linux-features").CombinedOutput()
	if err != nil {
		t.Fatalf("real Fence feature probe: %v\n%s", err, output)
	}
	line := ""
	for _, candidate := range strings.Split(string(output), "\n") {
		if strings.Contains(candidate, "Network namespace") {
			line = candidate
			break
		}
	}
	if line == "" || !strings.Contains(line, "direct network isolation") || !strings.Contains(line, "ok") {
		t.Fatalf("Network namespace did not report ok: %q", line)
	}
	fixture := newNativeFixture(t)
	marker := filepath.Join(fixture.worktree, "preflight-started")
	requireSuccess(t, fixture.launch("marker", marker))
	if !fileExists(marker) {
		t.Fatal("successful real preflight did not start agent")
	}
}

func TestLinuxFeaturePreflightFailsClosed(t *testing.T) {
	fixture := newNativeFixture(t)
	marker := filepath.Join(fixture.worktree, "invalid-preflight-started")
	cases := map[string]string{
		"unavailable": filepath.Join(fixture.root, "missing-fence"),
		"malformed":   filepath.Join(fixture.root, "malformed-fence"),
	}
	if err := os.WriteFile(cases["malformed"], []byte("#!/bin/sh\nprintf 'Network namespace  direct network isolation  okay  malformed\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, fence := range cases {
		t.Run(name, func(t *testing.T) {
			_ = os.Remove(marker)
			result := launchWithManifest(t, fixture, func(document map[string]any) {
				document["fenceExecutable"] = fence
			}, "marker", marker)
			if result.err == nil || fileExists(marker) {
				t.Fatalf("%s preflight did not fail before launch: %v\n%s", name, result.err, result.stderr)
			}
			if !strings.Contains(result.stderr, "Fence network namespace is unavailable") {
				t.Fatalf("%s preflight lacked corrective diagnostic: %s", name, result.stderr)
			}
		})
	}
}

func TestLinuxCAReexposedReadOnly(t *testing.T) {
	fixture := newNativeFixture(t)
	contents, err := os.ReadFile(fixture.certificate.ca)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := os.CreateTemp("/tmp", "den-native-ca-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	caPath := ca.Name()
	t.Cleanup(func() { _ = os.Remove(caPath) })
	if _, err := ca.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := ca.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(caPath, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(caPath)
	if err != nil || !info.Mode().IsRegular() || !strings.HasPrefix(caPath, "/tmp/") {
		t.Fatalf("CA fixture is not a regular /tmp file: %v / %v", info, err)
	}
	requireSuccess(t, fixture.launchWith([]string{"REPOWOLF_CA_FILE=" + caPath}, "ca-read-only"))
}

func TestLinuxMultiTokenArgvDenied(t *testing.T) {
	fixture := newNativeFixture(t)
	result := fixture.launch("argv-deny")
	if result.err == nil {
		t.Fatalf("multi-token descendant command escaped argv enforcement: %s", result.stdout)
	}
}

//go:build native

package pi

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type filesystemPolicy struct {
	Filesystem struct {
		AllowRead  []string `json:"allowRead"`
		AllowWrite []string `json:"allowWrite"`
		DenyRead   []string `json:"denyRead"`
		DenyWrite  []string `json:"denyWrite"`
	} `json:"filesystem"`
}

func policyHasPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func filesystemDiagnosticOutcomes(t *testing.T, path string) []string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prefixes := []string{
		"native-tools-direct-access:",
		"native-tools-direct-read:",
		"native-tools-raw-read:",
	}
	if strings.HasSuffix(os.Getenv("DEN_NATIVE_HOST_SYSTEM"), "-darwin") {
		prefixes = append(prefixes, "darwin-profile-ps-status:", "darwin-profile-allow-subpath:", "darwin-profile-deny-subpath:")
	}
	var outcomes []string
	for _, line := range strings.Split(string(contents), "\n") {
		for _, prefix := range prefixes {
			if strings.HasPrefix(line, prefix) {
				outcomes = append(outcomes, line)
			}
		}
	}
	if len(outcomes) != len(prefixes) {
		t.Fatalf("missing sanitized filesystem diagnostic outcomes: got %d, want %d", len(outcomes), len(prefixes))
	}
	return outcomes
}

func TestFenceDenyReadMaskIsReadOnly(t *testing.T) {
	for name, relativePath := range map[string]string{"same": ".", "descendant": "child"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			maskedDir := filepath.Join(root, "masked")
			if err := os.Mkdir(maskedDir, 0o700); err != nil {
				t.Fatal(err)
			}
			if relativePath != "." {
				if err := os.WriteFile(filepath.Join(maskedDir, relativePath), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			policyPath := filepath.Join(root, "fence.json")
			policy, err := json.Marshal(map[string]any{
				"allowPty": true,
				"filesystem": map[string]any{
					"allowRead":    []string{"/nix", root, policyPath},
					"allowExecute": []string{"/nix"},
					"allowWrite":   []string{root},
					"denyRead":     []string{maskedDir},
					"denyWrite":    []string{filepath.Join(maskedDir, relativePath)},
				},
				"command": map[string]any{"useDefaults": true},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(policyPath, policy, 0o400); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Getenv("DEN_NATIVE_FENCE"), "--settings", policyPath, "--",
				os.Getenv("DEN_NATIVE_BASH"), "-c", `printf escaped > "$1"`, "den-mask-test", filepath.Join(maskedDir, "created"))
			if output, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("Fence allowed a write inside a denyRead directory mask with denyWrite %q: %s", relativePath, output)
			}
		})
	}
}

func TestPiNativeFileToolsAndHomeAliasesStayInsideFence(t *testing.T) {
	fixture := newPiFixture(t)
	var denied []string
	for _, home := range []string{fixture.invokingHome, fixture.runtimeHome} {
		for _, relative := range []string{".pi/agent/auth.json", ".agents/skills/host/SKILL.md", ".ssh/id_ed25519", ".aws/credentials", ".config/gcloud/credentials.db", ".netrc"} {
			path := filepath.Join(home, relative)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("den9-host-secret"), 0o600); err != nil {
				t.Fatal(err)
			}
			denied = append(denied, path)
		}
		// Both homes are below the writable worktree. A parent alias and a final
		// symlink exercise canonical denies, not a missing/ungranted home.
		alias := home + "-alias"
		if err := os.Symlink(home, alias); err != nil {
			t.Fatal(err)
		}
		denied = append(denied, filepath.Join(alias, ".pi/agent/auth.json"), filepath.Join(alias, ".agents/skills/host/SKILL.md"))
		final := home + "-credential-link"
		if err := os.Symlink(filepath.Join(home, ".pi/agent/auth.json"), final); err != nil {
			t.Fatal(err)
		}
		denied = append(denied, final)
	}
	unrelated := filepath.Join(fixture.root, "unrelated-host")
	if err := os.WriteFile(unrelated, []byte("den9-host-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := os.Getenv("DEN_NATIVE_UNRELATED_STORE_FILE")
	if _, err := os.Stat(store); err != nil {
		t.Fatal(err)
	}
	denied = append(denied, unrelated, store)
	before := snapshotPaths(t, fixture.invokingHome, fixture.runtimeHome, unrelated, store)
	encoded, _ := json.Marshal(denied)
	policyReport := filepath.Join(fixture.root, "fence-policy.json")
	result := fixture.runEnforcementProbe(t, `
 const tools = await import(process.env.PI_PACKAGE_DIR + "/dist/core/tools/index.js");
 const read = tools.createReadToolDefinition(process.cwd()), write = tools.createWriteToolDefinition(process.cwd()), edit = tools.createEditToolDefinition(process.cwd());
 const control = process.cwd() + "/native-tool-control";
 await write.execute("write", {path: control, content: "before"});
 await edit.execute("edit", {path: control, edits: [{oldText: "before", newText: "after"}]});
 const allowed = await read.execute("read", {path:control}); assert.match(JSON.stringify(allowed),/after/);
 for (const root of [process.env.PI_CODING_AGENT_DIR,process.env.PI_CODING_AGENT_SESSION_DIR]) {
  await write.execute("state",{path:root+"/file-tool-state",content:"selected-state"});
  assert.match(JSON.stringify(await read.execute("state",{path:root+"/file-tool-state"})),/selected-state/);
 }
 const firstProtectedPath = `+string(jsonString(denied[0]))+`;
 if (process.env.DEN_NATIVE_HOST_SYSTEM?.endsWith("-darwin")) {
  const ps = run(process.env.DEN_NATIVE_PS!, ["-axo", "pid=,ppid=,command="]);
  const profile = (ps.stdout ?? "").replace(/\s+/g, " ");
  const protectedRoot = firstProtectedPath.slice(0, firstProtectedPath.lastIndexOf("/"));
  record("darwin-profile-ps-status:" + String(ps.status));
  record("darwin-profile-allow-subpath:" + (profile.includes("(allow file-read-data (subpath \"" + process.cwd() + "\")") ? "present" : "absent"));
  record("darwin-profile-deny-subpath:" + (profile.includes("(deny file-read* (subpath \"" + protectedRoot + "\")") ? "present" : "absent"));
 }
 const directOutcome = async (operation: () => Promise<unknown>) => {
  try { await operation(); return "allowed"; }
  catch (error: any) { return "denied:" + (error?.code ?? error?.name ?? "unknown"); }
 };
 record("native-tools-direct-access:" + await directOutcome(() => fs.promises.access(firstProtectedPath, fs.constants.R_OK)));
 record("native-tools-direct-read:" + await directOutcome(() => fs.promises.readFile(firstProtectedPath)));
 try {
  const raw = await read.execute("diagnostic-read", {path:firstProtectedPath});
  record("native-tools-raw-read:allowed:" + Object.keys(raw).sort().join(","));
 } catch (error: any) {
  record("native-tools-raw-read:denied:" + (error?.code ?? error?.name ?? "unknown"));
 }
 for (const path of `+string(encoded)+`) {
  record("native-tools-checking:"+path);
  await assert.rejects(read.execute("deny",{path}), /EACCES|EPERM|ENOENT|not found|permission/i, "native-tools-read unexpectedly resolved");
  record("native-tools-read-denied:"+path);
  await assert.rejects(write.execute("deny",{path,content:"escaped"}), /EACCES|EPERM|ENOENT|EROFS|permission/i, "native-tools-write unexpectedly resolved");
  record("native-tools-write-denied:"+path);
  await assert.rejects(edit.execute("deny",{path,edits:[{oldText:"den9-host-secret",newText:"escaped"}]}), /EACCES|EPERM|ENOENT|not found|permission/i, "native-tools-edit unexpectedly resolved");
  record("native-tools-edit-denied:"+path);
  record("native-tools-denied:"+path);
 }
 record("native-tools-control");
 `, []string{"DEN_NATIVE_PI_FENCE_POLICY_REPORT=" + policyReport}, func(document map[string]any) {
		document["fenceExecutable"] = os.Getenv("DEN_NATIVE_PI_FENCE_INPUT_RECORDER")
	})
	contents, err := os.ReadFile(policyReport)
	if err != nil {
		t.Fatal(err)
	}
	var policy filesystemPolicy
	if err := json.Unmarshal(contents, &policy); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(fixture.invokingHome, ".pi/agent"),
		filepath.Join(fixture.invokingHome, ".agents"),
		filepath.Join(fixture.invokingHome, ".agents/skills"),
		filepath.Join(fixture.runtimeHome, ".pi/agent"),
		filepath.Join(fixture.runtimeHome, ".agents"),
		filepath.Join(fixture.runtimeHome, ".agents/skills"),
	} {
		if !policyHasPath(policy.Filesystem.DenyRead, path) || !policyHasPath(policy.Filesystem.DenyWrite, path) {
			t.Fatalf("generated Fence policy omitted protected path %q", path)
		}
	}
	if !policyHasPath(policy.Filesystem.AllowRead, fixture.worktree) || !policyHasPath(policy.Filesystem.AllowWrite, fixture.worktree) {
		t.Fatal("generated Fence policy omitted writable worktree")
	}
	for _, outcome := range filesystemDiagnosticOutcomes(t, filepath.Join(fixture.agentDir, "enforcement.report")) {
		t.Logf("%s", outcome)
	}
	requireEnforcementProbeSuccess(t, fixture, result)
	if after := snapshotPaths(t, fixture.invokingHome, fixture.runtimeHome, unrelated, store); after != before {
		t.Fatal("native tools changed protected host/store state")
	}
	requireNoCredential(t, result, "den9-host-secret")
}

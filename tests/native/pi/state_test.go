//go:build native

package pi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPiStateSelectionPrecedenceAndDefaultCreation(t *testing.T) {
	t.Run("inherited", func(t *testing.T) {
		fixture := newPiFixture(t)
		result := fixture.rpc([]string{"--model", "den-native/fixture"},
			`{"id":"inherited","type":"prompt","message":"state selection"}`)
		requireStateLaunch(t, result, fixture.agentDir, fixture.sessionDir)
	})

	t.Run("explicit_over_inherited", func(t *testing.T) {
		fixture := newPiFixture(t)
		explicitAgent := filepath.Join(fixture.root, "explicit-agent")
		explicitSessions := filepath.Join(fixture.root, "explicit-sessions")
		for _, path := range []string{explicitAgent, explicitSessions} {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		result := fixture.launch(rpcInput(`{"id":"explicit","type":"prompt","message":"state selection"}`), nil,
			func(document map[string]any) {
				bindings := document["stateBindings"].([]any)
				bindings[0].(map[string]any)["explicitPath"] = explicitAgent
				bindings[1].(map[string]any)["explicitPath"] = explicitSessions
			}, "--mode", "rpc")
		requireStateLaunch(t, result, explicitAgent, explicitSessions)
		requireReportLines(t, filepath.Join(explicitAgent, "pi-resources.report"), "session-start:startup:worktree:true:explicit-sessions")
		if pathExists(fixture.reportPath()) || directoryHasEntries(t, fixture.sessionDir) {
			t.Fatal("inherited Pi state was used despite explicit state bindings")
		}
	})

	t.Run("default", func(t *testing.T) {
		fixture := newPiFixture(t)
		defaultRoot := filepath.Join(fixture.runtimeHome, ".local/state/den/pi")
		if err := os.MkdirAll(defaultRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		defaultAgent := filepath.Join(defaultRoot, "agent")
		defaultSessions := filepath.Join(defaultRoot, "sessions")
		result := fixture.launch(rpcInput(`{"id":"default","type":"prompt","message":"state selection"}`),
			[]string{"PI_CODING_AGENT_DIR", "PI_CODING_AGENT_SESSION_DIR"}, nil, "--mode", "rpc")
		requireStateLaunch(t, result, defaultAgent, defaultSessions)
	})
}

func TestPiStateRejectsProtectedHomesAliasesAndOverlaps(t *testing.T) {
	fixture := newPiFixture(t)
	account, err := user.Current()
	if err != nil || account.HomeDir == "" {
		t.Fatalf("invoking account home unavailable: %v", err)
	}
	protected := []string{
		filepath.Join(account.HomeDir, ".pi/agent"),
		filepath.Join(account.HomeDir, ".agents"),
		filepath.Join(fixture.runtimeHome, ".pi/agent"),
		filepath.Join(fixture.runtimeHome, ".agents/skills"),
	}
	if account.HomeDir == fixture.runtimeHome {
		t.Fatal("home-difference fixture is ineffective")
	}
	for _, path := range protected {
		for _, binding := range []string{"PI_CODING_AGENT_DIR", "PI_CODING_AGENT_SESSION_DIR"} {
			t.Run(binding+strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
				result := fixture.launch("", []string{binding + "=" + path}, nil, "--mode", "rpc")
				requireDeniedWith(t, result, "configuration directory")
			})
		}
	}

	realProtected := filepath.Join(fixture.runtimeHome, ".agents")
	if err := os.MkdirAll(realProtected, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(fixture.root, "agents-alias")
	if err := os.Symlink(realProtected, alias); err != nil {
		t.Fatal(err)
	}
	requireDenied(t, fixture.launch("", []string{"PI_CODING_AGENT_DIR=" + alias}, nil, "--mode", "rpc"))

	for name, paths := range map[string][2]string{
		"same":             {filepath.Join(fixture.root, "same"), filepath.Join(fixture.root, "same")},
		"session_in_agent": {filepath.Join(fixture.root, "outer"), filepath.Join(fixture.root, "outer/sessions")},
		"agent_in_session": {filepath.Join(fixture.root, "outer/agent"), filepath.Join(fixture.root, "outer")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.MkdirAll(paths[0], 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(paths[1], 0o700); err != nil {
				t.Fatal(err)
			}
			result := fixture.launch("", []string{
				"PI_CODING_AGENT_DIR=" + paths[0], "PI_CODING_AGENT_SESSION_DIR=" + paths[1],
			}, nil, "--mode", "rpc")
			requireDenied(t, result)
		})
	}

	for _, selected := range []string{"agent", "sessions"} {
		t.Run("final_component_link_"+selected, func(t *testing.T) {
			target := filepath.Join(fixture.root, selected+"-target")
			link := filepath.Join(fixture.root, selected+"-link")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			extra := []string{"PI_CODING_AGENT_DIR=" + fixture.agentDir, "PI_CODING_AGENT_SESSION_DIR=" + fixture.sessionDir}
			if selected == "agent" {
				extra[0] = "PI_CODING_AGENT_DIR=" + link
			} else {
				extra[1] = "PI_CODING_AGENT_SESSION_DIR=" + link
			}
			requireDenied(t, fixture.launch("", extra, nil, "--mode", "rpc"))
		})
	}

	parentTarget := filepath.Join(fixture.root, "parent-target")
	if err := os.MkdirAll(filepath.Join(parentTarget, "agent"), 0o700); err != nil {
		t.Fatal(err)
	}
	parentAlias := filepath.Join(fixture.root, "parent-alias")
	if err := os.Symlink(parentTarget, parentAlias); err != nil {
		t.Fatal(err)
	}
	parentResult := fixture.launch("", []string{"PI_CODING_AGENT_DIR=" + filepath.Join(parentAlias, "agent")}, nil, "--mode", "rpc")
	if parentResult.err != nil || !pathExists(filepath.Join(parentTarget, "agent/pi-resources.report")) {
		t.Fatalf("canonical parent alias was not safely contained: %v\n%s", parentResult.err, parentResult.stderr)
	}
}

func TestPiIsolatedCredentialTrustWritesAndRuntimeHomeDenies(t *testing.T) {
	fixture := newPiFixture(t)
	for _, relative := range []string{".pi/agent/auth.json", ".agents/skills/host/SKILL.md"} {
		path := filepath.Join(fixture.runtimeHome, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture-only host sentinel\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotPaths(t, fixture.runtimeHome)
	result := fixture.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), rpcInput(`{"id":"state-probe","type":"prompt","message":"/native-switch state-probe"}`), nil, "--mode", "rpc")
	requireRPCResponse(t, result, "state-probe", true, "")
	requireReportLines(t, fixture.reportPath(), "state-probe:credential-and-trust-written", "state-probe:home-denied:2")
	for _, name := range []string{"auth.json", "trust.json"} {
		if !pathExists(filepath.Join(fixture.agentDir, name)) {
			t.Fatalf("isolated %s was not written", name)
		}
	}
	if snapshotPaths(t, fixture.runtimeHome) != before {
		t.Fatal("Pi changed host-global state")
	}
	requireNoCredential(t, result, "native-fixture-only-key")
}

func TestPiProjectResourcesRequireSavedTrust(t *testing.T) {
	fixture := newPiFixture(t)
	extensions := filepath.Join(fixture.worktree, ".pi/extensions")
	if err := os.MkdirAll(extensions, 0o700); err != nil {
		t.Fatal(err)
	}
	projectMarker := filepath.Join(fixture.agentDir, "project-resource.loaded")
	projectExtension := fmt.Sprintf(`import { writeFileSync } from "node:fs";
export default function () { writeFileSync(%s, %s); }
`, strconv.Quote(projectMarker), strconv.Quote("loaded\n"))
	if err := os.WriteFile(filepath.Join(extensions, "project.ts"), []byte(projectExtension), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(fixture.worktree, ".agents/skills/project-skill"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.worktree, ".agents/skills/project-skill/SKILL.md"), []byte("---\nname: project-skill\ndescription: Trusted project skill\n---\nproject only\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result := fixture.sandbox("", "--mode", "rpc")
	if result.err != nil {
		t.Fatalf("untrusted startup failed instead of gating project resources: %v\n%s", result.err, result.stderr)
	}
	if pathExists(projectMarker) {
		t.Fatal("untrusted project extension loaded")
	}
	requireReportLines(t, fixture.reportPath(), "session-start:startup:worktree:false:")
	if report, _ := os.ReadFile(fixture.reportPath()); strings.Contains(string(report), "inventory:skill:project-skill:") {
		t.Fatal("untrusted project skill loaded")
	}
	_ = os.Remove(fixture.reportPath())
	trustContents, err := json.Marshal(map[string]bool{fixture.worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.agentDir, "trust.json"), trustContents, 0o600); err != nil {
		t.Fatal(err)
	}
	result = fixture.sandbox("", "--mode", "rpc")
	if result.err != nil {
		t.Fatalf("trusted startup failed: %v\n%s", result.err, result.stderr)
	}
	requireReportLines(t, fixture.reportPath(), "session-start:startup:worktree:true:", "inventory:skill:project-skill:ambient")
	if contents, err := os.ReadFile(projectMarker); err != nil || string(contents) != "loaded\n" {
		t.Fatalf("saved trust did not permit the real project extension: %q / %v", contents, err)
	}
}

func TestPiRPCSessionSwitchContainmentAndOrdering(t *testing.T) {
	fixture := newPiFixture(t)
	valid := writeSession(t, fixture.sessionDir, "valid.jsonl", "valid", fixture.worktree)
	result := fixture.rpc(nil, rpcSwitch("valid", valid))
	requireRPCSuccess(t, result)
	requireReportLines(t, fixture.reportPath(), "session-before:valid.jsonl", "session-start:resume:worktree:true:")

	outsideDir := filepath.Join(fixture.root, "outside")
	if err := os.Mkdir(outsideDir, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(outsideDir, "outside.jsonl")
	if err := os.WriteFile(outside, []byte("this must not be parsed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	siblingDir := fixture.sessionDir + "-sibling"
	if err := os.Mkdir(siblingDir, 0o700); err != nil {
		t.Fatal(err)
	}
	sibling := writeSession(t, siblingDir, "sibling.jsonl", "sibling", fixture.worktree)
	symlink := filepath.Join(fixture.sessionDir, "outside-link.jsonl")
	if err := os.Symlink(outside, symlink); err != nil {
		t.Fatal(err)
	}
	for name, target := range map[string]string{"outside_before_read": outside, "sibling_prefix": sibling, "symlink": symlink, "parent_alias": outside} {
		t.Run(name, func(t *testing.T) {
			attempt := newPiFixture(t)
			if name == "symlink" {
				target = filepath.Join(attempt.sessionDir, "outside-link.jsonl")
				if err := os.Symlink(outside, target); err != nil {
					t.Fatal(err)
				}
			}
			if name == "sibling_prefix" {
				attempt.sessionDir = filepath.Join(attempt.worktree, "sessions")
				if err := os.Mkdir(attempt.sessionDir, 0o700); err != nil {
					t.Fatal(err)
				}
				directory := attempt.sessionDir + "-sibling"
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				target = writeSession(t, directory, "sibling.jsonl", "sibling", attempt.worktree)
			}
			if name == "outside_before_read" {
				target = filepath.Join(attempt.worktree, "outside.jsonl")
				if err := os.WriteFile(target, []byte("must not be parsed\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if name == "parent_alias" {
				link := filepath.Join(attempt.sessionDir, "outside-parent")
				if err := os.Symlink(attempt.worktree, link); err != nil {
					t.Fatal(err)
				}
				writeSession(t, attempt.worktree, "parent-target.jsonl", "parent", attempt.worktree)
				target = filepath.Join(link, "parent-target.jsonl")
			}
			result := attempt.rpc(nil, rpcSwitch(name, target))
			message := "Pi session target escapes the configured directory"
			if name == "symlink" {
				message = "Pi session target must be an existing regular file"
			}
			requireRPCResponse(t, result, name, false, message)
			report, _ := os.ReadFile(attempt.reportPath())
			if strings.Contains(string(report), "session-before:"+filepath.Base(target)) {
				t.Fatalf("invalid switch reached extension event: %s", report)
			}
			if name == "outside_before_read" && strings.Contains(result.stdout+result.stderr, "valid session") {
				t.Fatal("outside session bytes were parsed before containment rejection")
			}
		})
	}
}

func TestPiSessionTargetSwapUsesValidatedBytes(t *testing.T) {
	fixture := newPiFixture(t)
	trustedCwd := filepath.Join(fixture.worktree, "trusted-cwd")
	hostileCwd := filepath.Join(fixture.worktree, "hostile-cwd")
	for _, path := range []string{trustedCwd, hostileCwd} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	target := writeSession(t, fixture.sessionDir, "swap.jsonl", "trusted", trustedCwd)
	outside := writeSession(t, fixture.worktree, "hostile.jsonl", "hostile", hostileCwd)
	runInteractiveCommand(t, fixture, "interactive", target, "DEN_PI_SWAP_TARGET="+target, "DEN_PI_SWAP_OUTSIDE="+outside)
	requireReportLines(t, fixture.reportPath(), "session-target-swapped", "session-start:resume:trusted-cwd:true")
	if report, _ := os.ReadFile(fixture.reportPath()); strings.Contains(string(report), "session-start:resume:hostile-cwd") {
		t.Fatal("Pi reopened attacker-replaced session bytes")
	}
}

func TestPiExtensionMutationCannotChangeCapturedSessionRoot(t *testing.T) {
	fixture := newPiFixture(t)
	hostileDir := filepath.Join(fixture.root, "hostile-sessions")
	if err := os.Mkdir(hostileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := writeSession(t, fixture.sessionDir, "extension.jsonl", "extension", fixture.worktree)
	runInteractiveCommand(t, fixture, "interactive", valid, "DEN_PI_MUTATE_SESSION_ENV="+hostileDir)
	requireReportLines(t, fixture.reportPath(), "session-loaded:interactive:worktree")

	attempt := newPiFixture(t)
	hostileDir = filepath.Join(attempt.worktree, "hostile-sessions")
	if err := os.Mkdir(hostileDir, 0o700); err != nil {
		t.Fatal(err)
	}
	hostile := writeSession(t, hostileDir, "hostile.jsonl", "hostile", attempt.worktree)
	result := attempt.run(os.Getenv("DEN_NATIVE_PI_SANDBOX"), rpcInput(rpcSwitch("hostile", hostile)),
		[]string{"DEN_PI_MUTATE_SESSION_ENV=" + hostileDir}, "--mode", "rpc")
	requireRPCResponse(t, result, "hostile", false, "Pi session target escapes the configured directory")
	report, _ := os.ReadFile(attempt.reportPath())
	if strings.Contains(string(report), "session-before:hostile.jsonl") {
		t.Fatal("environment escape reached the before-switch event")
	}
}

func TestPiSessionRootIdentityChangeIsRejectedBeforeEvent(t *testing.T) {
	fixture := newPiFixture(t)
	if err := os.Remove(fixture.sessionDir); err != nil {
		t.Fatal(err)
	}
	fixture.sessionDir = filepath.Join(fixture.worktree, "sessions")
	if err := os.Mkdir(fixture.sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := writeSession(t, fixture.sessionDir, "root-identity.jsonl", "root", fixture.worktree)
	// The host adversary replaces the inode after real Fence/Pi startup. No
	// passthrough Fence and no extra sandbox grants are needed for this attack.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Getenv("DEN_NATIVE_PI_SANDBOX"), "--mode", "rpc")
	command.Dir, command.Env = fixture.worktree, fixture.environment()
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output lockedBuffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = stdin.Close() })
	waitForReportLine(t, fixture.reportPath(), "session-start:startup:", 10*time.Second)
	held := fixture.sessionDir + ".held"
	if err := os.Rename(fixture.sessionDir, held); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(fixture.sessionDir); _ = os.Rename(held, fixture.sessionDir) }()
	if err := os.Mkdir(fixture.sessionDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeSession(t, fixture.sessionDir, "root-identity.jsonl", "replacement", fixture.worktree)
	if _, err := fmt.Fprintln(stdin, rpcSwitch("root-identity", target)); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	if err := command.Wait(); err != nil {
		t.Fatalf("real Fence RPC failed: %v", err)
	}
	requireRPCResponse(t, commandResult{stdout: output.String()}, "root-identity", false, "Pi session directory changed after startup")
	if report, _ := os.ReadFile(fixture.reportPath()); strings.Contains(string(report), "session-before:root-identity.jsonl") {
		t.Fatal("changed session root reached the before-switch event")
	}
}

func TestPiExtensionSessionSwitchRejectsAdversarialTargets(t *testing.T) {
	for _, kind := range []string{"outside", "sibling", "symlink", "parent-alias"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newPiFixture(t)
			fixture.sessionDir = filepath.Join(fixture.worktree, "sessions")
			if err := os.Mkdir(fixture.sessionDir, 0o700); err != nil {
				t.Fatal(err)
			}
			target := writeSession(t, fixture.worktree, "hostile.jsonl", "hostile", fixture.worktree)
			if kind == "sibling" {
				directory := fixture.sessionDir + "-sibling"
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
				target = writeSession(t, directory, "hostile.jsonl", "hostile", fixture.worktree)
			}
			if kind == "symlink" {
				link := filepath.Join(fixture.sessionDir, "link.jsonl")
				if err := os.Symlink(target, link); err != nil {
					t.Fatal(err)
				}
				target = link
			}
			if kind == "parent-alias" {
				link := filepath.Join(fixture.sessionDir, "parent")
				if err := os.Symlink(fixture.worktree, link); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(link, "hostile.jsonl")
			}
			request, _ := json.Marshal(map[string]string{"id": "extension-reject", "type": "prompt", "message": "/native-switch reject " + target})
			result := fixture.rpc(nil, string(request))
			requireRPCResponse(t, result, "extension-reject", true, "")
			requireReportLines(t, fixture.reportPath(), "session-rejected:reject:")
			if report, _ := os.ReadFile(fixture.reportPath()); strings.Contains(string(report), "session-before:"+filepath.Base(target)) {
				t.Fatal("invalid extension switch reached before-switch event")
			}
		})
	}
}

func TestPiInteractiveRejectsOutOfRootSwitch(t *testing.T) {
	fixture := newPiFixture(t)
	target := writeSession(t, fixture.worktree, "hostile.jsonl", "hostile", fixture.worktree)
	runInteractiveCommand(t, fixture, "interactive-reject", target)
	if report, _ := os.ReadFile(fixture.reportPath()); strings.Contains(string(report), "session-before:hostile.jsonl") {
		t.Fatal("invalid interactive switch reached before-switch event")
	}
}

func TestPiRPCExtensionSessionSwitch(t *testing.T) {
	fixture := newPiFixture(t)
	target := writeSession(t, fixture.sessionDir, "extension-rpc.jsonl", "extension", fixture.worktree)
	request, _ := json.Marshal(map[string]string{"id": "extension-valid", "type": "prompt", "message": "/native-switch rpc-extension " + target})
	result := fixture.rpc(nil, string(request))
	requireRPCResponse(t, result, "extension-valid", true, "")
	requireReportLines(t, fixture.reportPath(), "session-before:extension-rpc.jsonl", "session-loaded:rpc-extension:worktree")
}

func TestPiInteractiveExtensionSessionSwitch(t *testing.T) {
	fixture := newPiFixture(t)
	target := writeSession(t, fixture.sessionDir, "interactive.jsonl", "interactive", fixture.worktree)
	runInteractiveCommand(t, fixture, "interactive", target)
	requireReportLines(t, fixture.reportPath(), "session-before:interactive.jsonl", "session-loaded:interactive:worktree")
}

func runInteractiveCommand(t *testing.T, fixture *piFixture, mode, target string, extra ...string) string {
	t.Helper()
	return runInteractiveBinary(t, fixture, os.Getenv("DEN_NATIVE_PI_SANDBOX"), nil, mode, target, extra...)
}

func runInteractiveBinary(t *testing.T, fixture *piFixture, binary string, arguments []string, mode, target string, extra ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	script := os.Getenv("DEN_NATIVE_SCRIPT")
	piArguments := append(append([]string{}, arguments...), "--mode", "interactive")
	var command *exec.Cmd
	if strings.HasSuffix(os.Getenv("DEN_NATIVE_HOST_SYSTEM"), "-linux") {
		line := shellQuote(binary)
		for _, argument := range piArguments {
			line += " " + shellQuote(argument)
		}
		command = exec.CommandContext(ctx, script, "--quiet", "--return", "--command", line, "/dev/null")
	} else {
		command = exec.CommandContext(ctx, script, append([]string{"-q", "/dev/null", binary}, piArguments...)...)
	}
	command.Dir = fixture.worktree
	if mode == "interactive-reject" {
		extra = append(extra, "DEN_PI_OBSERVE_INTERACTIVE_REJECTION=1")
	}
	command.Env = fixture.environment(append(extra, "TERM=xterm-256color")...)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output lockedBuffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	ready := time.NewTicker(25 * time.Millisecond)
	defer ready.Stop()
	for {
		if contents, err := os.ReadFile(fixture.reportPath()); err == nil && strings.Contains(string(contents), "session-start:startup:") {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("interactive Pi exited before startup report: %v\n%s", err, output.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for interactive startup report\n%s", output.String())
		case <-ready.C:
		}
	}
	if _, err := fmt.Fprint(stdin, "/native-switch "+mode+" "+target+"\r"); err != nil {
		t.Fatal(err)
	}
	wanted := "session-loaded:" + mode + ":"
	if mode == "interactive-reject" {
		wanted = "session-interactive-error:outside-root"
	}
	deadline.Reset(10 * time.Second)
	for {
		if contents, err := os.ReadFile(fixture.reportPath()); err == nil && strings.Contains(string(contents), wanted) {
			break
		}
		select {
		case err := <-done:
			if contents, readErr := os.ReadFile(fixture.reportPath()); readErr == nil && strings.Contains(string(contents), wanted) && interactiveExitMatches(mode, err) {
				return output.String()
			}
			t.Fatalf("interactive Pi exited before %q report: %v\n%s", wanted, err, output.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for interactive report %q\n%s", wanted, output.String())
		case <-ready.C:
		}
	}
	_ = stdin.Close()
	if err := <-done; !interactiveExitMatches(mode, err) {
		t.Fatalf("interactive extension switch failed: %v\n%s", err, output.String())
	}
	return output.String()
}

func interactiveExitMatches(mode string, err error) bool {
	if mode != "interactive-reject" {
		return err == nil
	}
	exit, ok := err.(*exec.ExitError)
	return ok && exit.ExitCode() == 1
}

func requireStateLaunch(t *testing.T, result commandResult, agentDir, sessionDir string) {
	t.Helper()
	if result.err != nil {
		t.Fatalf("Pi state launch failed: %v\n%s%s", result.err, result.stdout, result.stderr)
	}
	if !pathExists(filepath.Join(agentDir, "pi-resources.report")) {
		t.Fatalf("selected agent state was not writable: %s", agentDir)
	}
	if !directoryHasEntries(t, sessionDir) {
		t.Fatalf("selected session state was not written: %s", sessionDir)
	}
}

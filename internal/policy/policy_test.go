package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var allowedDomains = []string{
	"api.anthropic.com", "*.anthropic.com", "claude.ai", "*.claude.ai",
	"registry.npmjs.org", "*.npmjs.org", "registry.yarnpkg.com", "pypi.org",
	"files.pythonhosted.org", "crates.io", "static.crates.io", "index.crates.io",
	"proxy.golang.org", "sum.golang.org", "formulae.brew.sh",
}

var deniedDomains = []string{
	"github.com", "*.github.com", "githubusercontent.com", "*.githubusercontent.com",
	"gitlab.com", "*.gitlab.com", "bitbucket.org", "*.bitbucket.org",
	"169.254.169.254", "metadata.google.internal", "instance-data.ec2.internal",
	"statsig.anthropic.com",
}

func TestBasePolicySecurityInvariants(t *testing.T) {
	base := readBase(t)
	var got document
	decoder := json.NewDecoder(strings.NewReader(string(base)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("base policy is not supported strict JSON: %v", err)
	}
	if !got.AllowPty || !got.Filesystem.DefaultDenyRead || !got.Filesystem.StrictDenyRead || !got.Filesystem.AllowGitConfig {
		t.Fatalf("required static policy booleans missing: %#v", got)
	}
	assertSameStrings(t, got.Network.AllowedDomains, allowedDomains)
	assertSameStrings(t, got.Network.DeniedDomains, deniedDomains)
	if want := protectedPaths(t); !reflect.DeepEqual(got.Filesystem.DenyRead, want) {
		t.Fatalf("denyRead = %#v, want exact protected list %#v", got.Filesystem.DenyRead, want)
	}
	if len(got.Filesystem.AllowRead)+len(got.Filesystem.AllowExecute)+len(got.Filesystem.AllowWrite) != 0 {
		t.Fatalf("base policy contains dynamic filesystem grants: %#v", got.Filesystem)
	}
	for _, required := range []string{"**/.env", "**/.env.*", "**/*.key", "**/*.pem", "**/*.p12", "**/*.pfx", "~/.npm/_logs", "~/.fence/debug", "/tmp/fence", "/private/tmp/fence"} {
		if !contains(got.Filesystem.DenyWrite, required) {
			t.Errorf("denyWrite missing %q", required)
		}
	}
	for _, ineffective := range []string{"$HOME/.npm/_logs", "$HOME/.fence/debug"} {
		if contains(got.Filesystem.DenyWrite, ineffective) {
			t.Errorf("denyWrite retained Fence-ineffective path %q", ineffective)
		}
	}
	text := string(base)
	for _, forbidden := range []string{
		"rw1_", "/nix/store/**", "/nix/var/nix/profiles/**", "~/.nix-profile/**",
		`"."`, `"/tmp"`, "~/.cache/**", "~/.claude*", "~/.claude/**",
		"~/.codex/**", "~/.cursor/**", "~/.opencode/**", "~/.local/state/**",
		"~/.gemini/**", "~/.pi/**", "~/.npm/_cacache", "~/.npm/_npx", `"~/.cache"`,
		"~/.bun/**", "~/.cargo/registry/**", "~/.cargo/git/**", "~/.cargo/.package-cache",
		"~/.zcompdump*", "~/.local/share/**", "~/.config/**", "~/.1password/agent.sock",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("base policy contains removed grant/value %q", forbidden)
		}
	}
	if got.Command.RuntimeExecPolicy != "argv" {
		t.Fatalf("runtimeExecPolicy = %q, want argv", got.Command.RuntimeExecPolicy)
	}
	if got.Command.UseDefaults == nil || !*got.Command.UseDefaults || len(got.Command.Deny) == 0 {
		t.Fatal("reference command denials/defaults were not retained")
	}
}

func TestGenerateDynamicPolicyByPlatform(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(resolv, []byte("nameserver 127.0.0.1\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	originalResolver := resolveResolvConf
	resolveResolvConf = func() (string, error) { return resolv, nil }
	t.Cleanup(func() { resolveResolvConf = originalResolver })
	base := Base(readBase(t))

	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			dynamic := Dynamic{
				Platform: platform, RepoWolfHostname: "broker.example.test", CAFile: paths.ca,
				ClosurePaths: []string{paths.closureRead, paths.closureExec},
				Worktree:     paths.worktree, ScratchDir: paths.scratch, StatePaths: []string{paths.state + string(os.PathSeparator)},
				DefaultStatePaths: []string{paths.defaultState + string(os.PathSeparator)}, CustomMode: true,
				UnixSockets: []string{paths.socket}, HostPorts: []uint16{5432, 6379}, PolicyFile: paths.policy,
			}
			encoded, err := Generate(base, dynamic)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "rw1_") {
				t.Fatal("generated policy contains token prefix")
			}
			var got document
			decoder := json.NewDecoder(strings.NewReader(string(encoded)))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(&got); err != nil {
				t.Fatalf("generated JSON: %v", err)
			}

			for _, path := range []string{paths.ca, paths.closureRead, paths.closureExec, paths.worktree, paths.scratch, paths.state, paths.socket, paths.policy} {
				if !contains(got.Filesystem.AllowRead, path) {
					t.Errorf("allowRead missing %q", path)
				}
			}
			for _, path := range []string{paths.closureRead, paths.closureExec} {
				if !contains(got.Filesystem.AllowExecute, path) {
					t.Errorf("allowExecute missing %q", path)
				}
			}
			for _, path := range []string{paths.worktree, paths.scratch, paths.state, paths.socket} {
				if !contains(got.Filesystem.AllowWrite, path) {
					t.Errorf("allowWrite missing %q", path)
				}
			}
			for _, path := range []string{paths.defaultState, filepath.Join(paths.worktree, ".git/config"), filepath.Join(paths.worktree, ".git/config.worktree"), paths.policy, filepath.Dir(paths.policy)} {
				if !contains(got.Filesystem.DenyWrite, path) {
					t.Errorf("denyWrite missing %q", path)
				}
			}
			if contains(got.Filesystem.AllowWrite, paths.ca) || contains(got.Filesystem.AllowWrite, paths.policy) || contains(got.Filesystem.AllowWrite, filepath.Dir(paths.policy)) {
				t.Fatal("read-only CA/policy path became writable")
			}
			if !contains(got.Network.AllowedDomains, "broker.example.test") {
				t.Fatal("broker hostname missing")
			}
			if platform == "linux" {
				if got.Command.RuntimeExecPolicy != "argv" {
					t.Fatal("Linux argv policy missing")
				}
				if !reflect.DeepEqual(got.Network.AllowLocalOutboundPorts, []uint16{5432, 6379}) || got.Network.AllowLocalOutbound == nil || !*got.Network.AllowLocalOutbound {
					t.Fatalf("Linux ports = %#v", got.Network)
				}
				for _, path := range linuxOperationalPaths(resolv) {
					if !contains(got.Filesystem.AllowRead, path) {
						t.Errorf("Linux operational read missing %q", path)
					}
				}
				if len(got.Network.AllowUnixSockets) != 0 {
					t.Fatal("Linux policy emitted Darwin socket network grants")
				}
			} else {
				if got.Command.RuntimeExecPolicy != "" {
					t.Fatalf("Darwin runtime policy = %q", got.Command.RuntimeExecPolicy)
				}
				if len(got.Network.AllowLocalOutboundPorts) != 0 || got.Network.AllowLocalOutbound == nil || !*got.Network.AllowLocalOutbound {
					t.Fatalf("Darwin ports = %#v", got.Network)
				}
				assertSameStrings(t, got.Network.AllowUnixSockets, []string{paths.socket})
				for _, path := range darwinOperationalReads {
					if !contains(got.Filesystem.AllowRead, path) {
						t.Errorf("Darwin operational read missing %q", path)
					}
				}
			}
		})
	}
}

func TestGenerateRejectsInvalidInputsAndUnknownFields(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	valid := Dynamic{Platform: "linux", RepoWolfHostname: "broker.example.test", CAFile: paths.ca, Worktree: paths.worktree, ScratchDir: paths.scratch, PolicyFile: paths.policy}
	for name, mutate := range map[string]func(*Dynamic){
		"unknown platform": func(d *Dynamic) { d.Platform = "windows" },
		"relative path":    func(d *Dynamic) { d.Worktree = "relative" },
		"unclean path":     func(d *Dynamic) { d.ScratchDir += "/../scratch" },
		"bad hostname":     func(d *Dynamic) { d.RepoWolfHostname = "github.com" },
		"bad port":         func(d *Dynamic) { d.HostPorts = []uint16{0} },
	} {
		t.Run(name, func(t *testing.T) {
			d := valid
			mutate(&d)
			if _, err := Generate(Base(readBase(t)), d); err == nil {
				t.Fatal("Generate succeeded")
			}
		})
	}
	badBase := Base(strings.Replace(string(readBase(t)), `"allowPty": true`, `"allowPty": true, "token": "rw1_secret"`, 1))
	if _, err := Generate(badBase, valid); err == nil {
		t.Fatal("unknown/token base field accepted")
	}
}

func TestAllowedDomainsDoNotMatchDeniedGitHosts(t *testing.T) {
	for _, denied := range deniedDomains[:8] {
		for _, allowed := range allowedDomains {
			if domainMatches(allowed, strings.TrimPrefix(denied, "*.")) {
				t.Errorf("allowed %q matches denied %q", allowed, denied)
			}
		}
	}
}

func readBase(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "policy", "fence.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func protectedPaths(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "nix", "lib", "protected-paths.nix"))
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`"([^"\\]*(?:\\.[^"\\]*)*)"`).FindAllStringSubmatch(string(data), -1)
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		result = append(result, match[1])
	}
	return result
}

type testPaths struct{ ca, closureRead, closureExec, worktree, scratch, state, defaultState, socket, policy string }

func makePaths(t *testing.T, root string) testPaths {
	t.Helper()
	p := testPaths{ca: filepath.Join(root, "ca.pem"), closureRead: filepath.Join(root, "closure-read"), closureExec: filepath.Join(root, "closure-exec"), worktree: filepath.Join(root, "worktree"), scratch: filepath.Join(root, "scratch"), state: filepath.Join(root, "state"), defaultState: filepath.Join(root, "default-state"), socket: filepath.Join(root, "daemon.sock"), policy: filepath.Join(root, "policy", "fence.json")}
	for _, dir := range []string{p.closureRead, p.closureExec, p.worktree, p.scratch, p.state, p.defaultState, filepath.Dir(p.policy)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(p.ca, []byte("ca"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.socket, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func linuxOperationalPaths(resolv string) []string {
	return []string{resolv, "/etc/hosts", "/etc/nsswitch.conf", "/etc/services", "/etc/protocols"}
}

func assertSameStrings(t *testing.T, got, want []string) {
	t.Helper()
	a, b := append([]string(nil), got...), append([]string(nil), want...)
	sort.Strings(a)
	sort.Strings(b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("strings = %#v, want %#v", got, want)
	}
}
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

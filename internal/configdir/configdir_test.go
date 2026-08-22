package configdir

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

var protectedPatterns = []string{
	"~/.ssh/id_*",
	"~/.ssh/config",
	"~/.ssh/*.pem",
	"~/.gnupg/**",
	"~/.aws/**",
	"~/.config/gcloud/**",
	"~/.kube/**",
	"~/.docker/**",
	"~/.pypirc",
	"~/.netrc",
	"~/.git-credentials",
	"~/.cargo/credentials",
	"~/.cargo/credentials.toml",
	"~/.gitconfig",
	"~/.config/git/**",
}

func TestSelectPrecedence(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	explicit := privateDir(t, root, "explicit")
	inherited := privateDir(t, root, "inherited")
	deps := safeDependencies(t)

	tests := []struct {
		name      string
		explicit  *string
		inherited *string
		want      string
	}{
		{"explicit over inherited", &explicit, &inherited, explicit},
		{"inherited over fallback", nil, &inherited, inherited},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection, err := Select(test.explicit, test.inherited, home, nil, deps)
			if err != nil {
				t.Fatalf("Select() error = %v", err)
			}
			if selection.Mode != Custom || selection.CanonicalPath != test.want {
				t.Fatalf("selection = %#v, want custom %q", selection, test.want)
			}
		})
	}
}

func TestSelectRejectsExplicitlyEmptyAndRelativeCustomValues(t *testing.T) {
	home := privateDir(t, t.TempDir(), "home")
	sentinel := "sensitive-relative-config"
	tests := []struct {
		name      string
		explicit  *string
		inherited *string
	}{
		{"empty explicit", stringPointer(""), nil},
		{"relative explicit", &sentinel, nil},
		{"empty inherited", nil, stringPointer("")},
		{"relative inherited", nil, &sentinel},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Select(test.explicit, test.inherited, home, nil, safeDependencies(t))
			if err == nil {
				t.Fatal("Select() error = nil")
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatalf("Select() disclosed custom value: %v", err)
			}
		})
	}
}

func TestSelectFallbackUsesExactlyDefaultClaudePaths(t *testing.T) {
	home := filepath.Join(t.TempDir(), "missing-home")
	selection, err := Select(nil, nil, home, []string{"~/.ssh/id_*"}, Dependencies{ACLProbe: []string{"/missing/probe"}})
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	want := []string{
		filepath.Join(home, ".claude") + string(os.PathSeparator),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".config", "claude") + string(os.PathSeparator),
	}
	if selection.Mode != Default {
		t.Fatalf("Mode = %q, want %q", selection.Mode, Default)
	}
	if !reflect.DeepEqual(selection.WritablePaths, want) {
		t.Fatalf("WritablePaths = %#v, want %#v", selection.WritablePaths, want)
	}
	if len(selection.DeniedDefaultPaths) != 0 {
		t.Fatalf("DeniedDefaultPaths = %#v, want empty", selection.DeniedDefaultPaths)
	}
}

func TestSelectCreatesMissingCustomDirectoryAt0700(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := filepath.Join(root, "created")

	selection, err := Select(&path, nil, home, nil, safeDependencies(t))
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if info.Mode().Perm() != 0o700 || !info.IsDir() {
		t.Fatalf("created mode = %v, want drwx------", info.Mode())
	}
	if !selection.Created {
		t.Fatal("Created = false")
	}
	if got, want := selection.WritablePaths, []string{path + string(os.PathSeparator)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("WritablePaths = %#v, want %#v", got, want)
	}
	if got, want := selection.DeniedDefaultPaths, defaultPaths(home); !reflect.DeepEqual(got, want) {
		t.Fatalf("DeniedDefaultPaths = %#v, want %#v", got, want)
	}
}

func TestSelectAcceptsExistingOwnerOnlyDirectoryWithoutChangingMode(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")

	selection, err := Select(&path, nil, home, nil, safeDependencies(t))
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if selection.Created {
		t.Fatal("Created = true")
	}
	info, _ := os.Lstat(path)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("existing mode changed to %o", info.Mode().Perm())
	}
}

func TestSelectRejectsEveryGroupOrOtherPermissionWithoutChmod(t *testing.T) {
	for bit := os.FileMode(0o001); bit <= 0o040; bit <<= 1 {
		t.Run(bit.String(), func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			path := privateDir(t, root, "config")
			mode := os.FileMode(0o700) | bit
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			_, err := Select(&path, nil, home, nil, safeDependencies(t))
			if err == nil {
				t.Fatalf("Select() accepted mode %o", mode)
			}
			info, statErr := os.Lstat(path)
			if statErr != nil || info.Mode().Perm() != mode {
				t.Fatalf("existing mode changed: info=%v err=%v", info, statErr)
			}
		})
	}
}

func TestDirectoryWritableRejectsReadOnlyFilesystem(t *testing.T) {
	create := func(string, string) (*os.File, error) { return nil, syscall.EROFS }
	if directoryWritableWith("/read-only", create) {
		t.Fatal("directoryWritableWith() = true after a read-only filesystem error")
	}
}

func TestDirectoryWritableChecksEffectiveAccessWithoutArtifacts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses ordinary owner write checks")
	}
	path := privateDir(t, t.TempDir(), "config")
	before, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if !directoryWritable(path) {
		t.Fatal("directoryWritable() = false for writable owner directory")
	}
	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}
	if directoryWritable(path) {
		t.Fatal("directoryWritable() = true for effectively read-only directory")
	}
	after, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("writability check left artifacts: before=%d after=%d", len(before), len(after))
	}
}

func TestSelectRejectsMissingOwnerPermission(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o500, 0o300} {
		t.Run(mode.String(), func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			path := privateDir(t, root, "config")
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := Select(&path, nil, home, nil, safeDependencies(t)); err == nil {
				t.Fatalf("Select() accepted mode %o", mode)
			}
		})
	}
}

func TestSelectRejectsNonDirectoryAndFinalSymlink(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	target := privateDir(t, root, "target")
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{file, link} {
		if _, err := Select(&path, nil, home, nil, safeDependencies(t)); err == nil {
			t.Fatalf("Select(%q) error = nil", filepath.Base(path))
		}
	}
}

func TestSelectCanonicalizesParentSymlink(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	realParent := privateDir(t, root, "real")
	link := filepath.Join(root, "parent-link")
	if err := os.Symlink(realParent, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "config")

	selection, err := Select(&path, nil, home, nil, safeDependencies(t))
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	want := filepath.Join(realParent, "config")
	if selection.CanonicalPath != want {
		t.Fatalf("CanonicalPath = %q, want %q", selection.CanonicalPath, want)
	}
	if _, err := os.Lstat(want); err != nil {
		t.Fatalf("canonical directory missing: %v", err)
	}
}

func TestSelectValidatesWritableAncestors(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		ok   bool
	}{
		{"private", 0o700, true},
		{"group writable", 0o720, false},
		{"other writable", 0o702, false},
		{"sticky user-owned", 0o702 | os.ModeSticky, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			parent := privateDir(t, root, "parent")
			if err := os.Chmod(parent, test.mode); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "config")
			_, err := Select(&path, nil, home, nil, safeDependencies(t))
			if (err == nil) != test.ok {
				t.Fatalf("Select() error = %v, want success %t", err, test.ok)
			}
		})
	}
}

func TestSelectRejectsProtectedPathOverlaps(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	defaults := []string{
		filepath.Join(home, ".claude"),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".config", "claude"),
	}
	roots := []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".ssh", "config"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".config", "gcloud"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".docker"),
		filepath.Join(home, ".pypirc"),
		filepath.Join(home, ".netrc"),
		filepath.Join(home, ".git-credentials"),
		filepath.Join(home, ".cargo", "credentials"),
		filepath.Join(home, ".cargo", "credentials.toml"),
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, ".config", "git"),
	}
	protected := append(defaults, roots...)

	for _, protectedPath := range protected {
		name := strings.ReplaceAll(strings.TrimPrefix(protectedPath, "/"), "/", "_")
		if name == "" {
			name = "root"
		}
		candidates := []string{protectedPath, filepath.Join(protectedPath, "descendant")}
		if parent := filepath.Dir(protectedPath); parent != protectedPath {
			candidates = append(candidates, parent)
		}
		for index, candidate := range candidates {
			t.Run(name+"_relationship_"+string(rune('0'+index)), func(t *testing.T) {
				_, err := Select(&candidate, nil, home, protectedPatterns, safeDependencies(t))
				if err == nil {
					t.Fatalf("Select() accepted overlap relationship for protected entry %q", name)
				}
			})
		}
	}
	for _, candidate := range []string{"/", home} {
		if _, err := Select(&candidate, nil, home, protectedPatterns, safeDependencies(t)); err == nil {
			t.Fatalf("Select() accepted protected ancestor %q", filepath.Base(candidate))
		}
	}
}

func TestSelectRejectsConcreteMatchForEveryDenyGlob(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	matches := []string{
		"~/.ssh/id_test", "~/.ssh/config", "~/.ssh/key.pem", "~/.gnupg/key",
		"~/.aws/credentials", "~/.config/gcloud/config", "~/.kube/config",
		"~/.docker/config.json", "~/.pypirc", "~/.netrc", "~/.git-credentials",
		"~/.cargo/credentials", "~/.cargo/credentials.toml", "~/.gitconfig",
		"~/.config/git/config",
	}
	for index, match := range matches {
		path := filepath.Join(home, strings.TrimPrefix(match, "~/"))
		t.Run(strings.ReplaceAll(match, "/", "_"), func(t *testing.T) {
			_, err := Select(&path, nil, home, []string{protectedPatterns[index]}, safeDependencies(t))
			if err == nil {
				t.Fatal("Select() accepted a concrete deny-pattern match")
			}
		})
	}
}

func TestSelectRejectsGlobWithoutCompleteProtectedRoot(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	if _, err := Select(&path, nil, home, []string{"*"}, safeDependencies(t)); err == nil {
		t.Fatal("Select() accepted a glob without a complete root")
	}
}

func TestSelectAcceptsDisjointInWorktreeAndOutOfWorktreePaths(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	worktree := privateDir(t, home, "worktree")
	outside := privateDir(t, root, "outside")
	for _, path := range []string{
		privateDir(t, worktree, "claude-state"),
		privateDir(t, outside, "claude-state"),
	} {
		selection, err := Select(&path, nil, home, protectedPatterns, safeDependencies(t))
		if err != nil {
			t.Fatalf("Select() rejected disjoint path: %v", err)
		}
		if selection.CanonicalPath != path {
			t.Fatalf("CanonicalPath = %q, want %q", selection.CanonicalPath, path)
		}
	}
}

func TestSelectionRevalidateDetectsFinalReplacementAndModeChange(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{"replacement", func(t *testing.T, path string) {
			if err := os.Rename(path, path+"-old"); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"mode change", func(t *testing.T, path string) {
			if err := os.Chmod(path, 0o500); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			home := privateDir(t, root, "home")
			path := privateDir(t, root, "config")
			selection, err := Select(&path, nil, home, nil, safeDependencies(t))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(t, path)
			if err := selection.Revalidate(); err == nil {
				t.Fatal("Revalidate() error = nil")
			}
		})
	}
}

func TestSelectionRevalidateDetectsAncestorModeChange(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	parent := privateDir(t, root, "parent")
	path := privateDir(t, parent, "config")
	selection, err := Select(&path, nil, home, nil, safeDependencies(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o720); err != nil {
		t.Fatal(err)
	}
	if err := selection.Revalidate(); err == nil {
		t.Fatal("Revalidate() error = nil")
	}
}

func TestSelectionRollbackLifecycle(t *testing.T) {
	t.Run("created directory removed", func(t *testing.T) {
		root := t.TempDir()
		home := privateDir(t, root, "home")
		path := filepath.Join(root, "created")
		selection, err := Select(&path, nil, home, nil, safeDependencies(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := selection.Rollback(); err != nil {
			t.Fatalf("Rollback() error = %v", err)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("created path remains: %v", err)
		}
	})

	t.Run("existing directory retained", func(t *testing.T) {
		root := t.TempDir()
		home := privateDir(t, root, "home")
		path := privateDir(t, root, "existing")
		selection, err := Select(&path, nil, home, nil, safeDependencies(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := selection.Rollback(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("existing path removed: %v", err)
		}
	})

	t.Run("commit retains created directory", func(t *testing.T) {
		root := t.TempDir()
		home := privateDir(t, root, "home")
		path := filepath.Join(root, "committed")
		selection, err := Select(&path, nil, home, nil, safeDependencies(t))
		if err != nil {
			t.Fatal(err)
		}
		selection.Commit()
		if err := selection.Rollback(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("committed path removed: %v", err)
		}
	})

	t.Run("replacement is not removed", func(t *testing.T) {
		root := t.TempDir()
		home := privateDir(t, root, "home")
		path := filepath.Join(root, "created")
		selection, err := Select(&path, nil, home, nil, safeDependencies(t))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, path+"-old"); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := selection.Rollback(); err == nil {
			t.Fatal("Rollback() error = nil after replacement")
		}
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("replacement was removed: %v", err)
		}
	})
}

func TestRollbackWithoutCapturedIdentityNeverDeletesCurrentPath(t *testing.T) {
	path := privateDir(t, t.TempDir(), "replacement")
	state := &selectionState{path: path, created: true}

	if err := rollbackCreated(state); err == nil {
		t.Fatal("rollbackCreated() error = nil without a captured identity")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("rollback removed a path without proving its identity: %v", err)
	}
}

func TestSelectPostCreationFailureDoesNotRemoveReplacement(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := filepath.Join(root, "created")
	body := "case \"$DEN_CONFIGDIR_ACL_ORIGINAL_PATH\" in\n" + shellQuote(path) + ")\n" +
		"mv " + shellQuote(path) + " " + shellQuote(path+"-old") + "\n" +
		"mkdir -m 700 " + shellQuote(path) + "\n" +
		"cat <<'ACL_EOF'\n" + unsafeNonOwnerACL(t) + "ACL_EOF\n;;\n" +
		"*)\ncat <<'ACL_EOF'\n" + safeACL(t) + "ACL_EOF\n;;\nesac"
	deps := Dependencies{ACLProbe: []string{writeProbe(t, body)}}
	if _, err := Select(&path, nil, home, nil, deps); err == nil {
		t.Fatal("Select() error = nil")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("replacement was removed after post-creation failure: %v", err)
	}
}

func TestSelectRollsBackCreatedDirectoryOnPostCreationFailure(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := filepath.Join(root, "created")
	deps := targetedDependencies(t, path, unsafeNonOwnerACL(t), safeACL(t))
	if _, err := Select(&path, nil, home, nil, deps); err == nil {
		t.Fatal("Select() error = nil")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("created directory was not rolled back: %v", err)
	}
}

func privateDir(t *testing.T, parent, name string) string {
	t.Helper()
	path := filepath.Join(parent, name)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir(%q): %v", name, err)
	}
	return path
}

func defaultPaths(home string) []string {
	return []string{
		filepath.Join(home, ".claude") + string(os.PathSeparator),
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".config", "claude") + string(os.PathSeparator),
	}
}

func stringPointer(value string) *string { return &value }

func safeDependencies(t *testing.T) Dependencies {
	t.Helper()
	return outputDependencies(t, safeACL(t))
}

func safeACL(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "darwin" {
		return ""
	}
	return "user::rwx\ngroup::---\nother::---\n"
}

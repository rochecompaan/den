//go:build darwin

package configdir

import "testing"

func TestSelectRejectsDarwinCaseAliasOfProtectedRoot(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	candidate := privateDir(t, home, ".SSH")

	if _, err := Select(&candidate, nil, home, []string{"~/.ssh/**"}, safeDependencies(t)); err == nil {
		t.Fatal("Select() accepted a case alias of a protected root")
	}
}

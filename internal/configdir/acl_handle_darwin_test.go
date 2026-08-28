//go:build darwin

package configdir

import "testing"

func TestSelectAndRevalidateWithDarwinACLProbe(t *testing.T) {
	root := t.TempDir()
	home := privateDir(t, root, "home")
	path := privateDir(t, root, "config")
	deps := Dependencies{ACLProbe: []string{"/bin/ls", "-lde"}}

	selection, err := Select(&path, nil, home, nil, deps)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if err := selection.Revalidate(); err != nil {
		t.Fatalf("Revalidate() error = %v", err)
	}
}

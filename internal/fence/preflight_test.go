package fence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const featureHeader = `Linux Sandbox Features:

  Capability                 Required For                     Status       Details
  -------------------------  -------------------------------  -----------  -------
`

func TestParseLinuxFeaturesRequiresExactlyOneOKNetworkNamespace(t *testing.T) {
	valid := featureHeader +
		"  Bubblewrap                  core sandbox                   ok           /bin/bwrap\n" +
		"  Network namespace           direct network isolation       ok           bwrap --unshare-net works\n" +
		"  Seccomp user notification   runtimeExecPolicy: \"argv\"      ok           listener filter installs\n"
	if err := parseLinuxFeatures(valid); err != nil {
		t.Fatalf("valid output rejected: %v", err)
	}

	for name, output := range map[string]string{
		"missing":           featureHeader + "  Bubblewrap  core sandbox  ok  /bin/bwrap\n",
		"unavailable":       strings.Replace(valid, "network isolation       ok", "network isolation       unavailable", 1),
		"duplicate":         valid + "  Network namespace           direct network isolation       ok           duplicate\n",
		"malformed columns": featureHeader + "  Network namespace direct network isolation ok bwrap --unshare-net works\n",
		"unknown status":    strings.Replace(valid, "network isolation       ok", "network isolation       detected", 1),
		"wrong requirement": strings.Replace(valid, "direct network isolation", "some other requirement  ", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := parseLinuxFeatures(output); err == nil {
				t.Fatal("output accepted")
			}
		})
	}
}

func TestPreflightExecutesPinnedFenceAndFailsClosed(t *testing.T) {
	root := t.TempDir()
	valid := featureHeader + "  Network namespace           direct network isolation       ok           bwrap --unshare-net works\n"
	good := writeProbe(t, root, "good", "printf '%s' '"+valid+"'")
	if err := Preflight(context.Background(), good); err != nil {
		t.Fatalf("Preflight: %v", err)
	}

	bad := writeProbe(t, root, "bad", "echo probe-failed >&2; exit 7")
	err := Preflight(context.Background(), bad)
	if err == nil || !strings.Contains(err.Error(), "network namespace") || strings.Contains(err.Error(), "probe-failed") {
		t.Fatalf("unavailable probe error = %v", err)
	}
}

func writeProbe(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nif [ \"$1\" != --linux-features ]; then exit 9; fi\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

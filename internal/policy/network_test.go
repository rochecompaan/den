package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateAllowsOnlyConfiguredRepoWolfIP(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(resolv, []byte("nameserver 127.0.0.1\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	originalResolver := resolveResolvConf
	resolveResolvConf = func() (string, error) { return resolv, nil }
	t.Cleanup(func() { resolveResolvConf = originalResolver })
	for _, platform := range []string{"linux", "darwin"} {
		for _, host := range []string{"172.17.0.1", "127.0.0.1", "2001:db8::1", "::1"} {
			t.Run(platform+"/"+host, func(t *testing.T) {
				dynamic := testDynamic(paths)
				dynamic.Platform = platform
				dynamic.RepoWolfHostname = host
				dynamic.HostPorts = nil
				encoded, err := Generate(Base(readBase(t)), dynamic)
				if err != nil {
					t.Fatalf("Generate() error = %v", err)
				}
				var got document
				if err := json.Unmarshal(encoded, &got); err != nil {
					t.Fatal(err)
				}
				wantDomains := append(append([]string{}, allowedDomains...), host)
				assertSameStrings(t, got.Network.AllowedDomains, wantDomains)
				assertSameStrings(t, got.Network.DeniedDomains, deniedDomains)
				if got.Network.AllowLocalOutbound == nil || *got.Network.AllowLocalOutbound || len(got.Network.AllowLocalOutboundPorts) != 0 {
					t.Fatalf("RepoWolf IP enabled local outbound: %#v", got.Network)
				}
			})
		}
	}
}

func TestGenerateWithEmptyHostPortsDoesNotEnableLocalOutbound(t *testing.T) {
	root := t.TempDir()
	paths := makePaths(t, root)
	resolv := filepath.Join(root, "resolv.conf")
	if err := os.WriteFile(resolv, []byte("nameserver 127.0.0.1\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	originalResolver := resolveResolvConf
	resolveResolvConf = func() (string, error) { return resolv, nil }
	t.Cleanup(func() { resolveResolvConf = originalResolver })

	for _, platform := range []string{"linux", "darwin"} {
		t.Run(platform, func(t *testing.T) {
			dynamic := testDynamic(paths)
			dynamic.Platform = platform
			dynamic.HostPorts = []uint16{}
			encoded, err := Generate(Base(readBase(t)), dynamic)
			if err != nil {
				t.Fatal(err)
			}
			var got document
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if got.Network.AllowLocalOutbound == nil || *got.Network.AllowLocalOutbound || len(got.Network.AllowLocalOutboundPorts) != 0 {
				t.Fatalf("empty ports enabled local outbound: %#v", got.Network)
			}
			if !strings.Contains(string(encoded), `"allowLocalOutbound": false`) || strings.Contains(string(encoded), "allowLocalOutboundPorts") {
				t.Fatalf("empty ports policy = %s", encoded)
			}
		})
	}
}

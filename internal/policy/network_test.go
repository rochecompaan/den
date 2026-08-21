package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
			if got.Network.AllowLocalOutbound != nil || len(got.Network.AllowLocalOutboundPorts) != 0 {
				t.Fatalf("empty ports enabled local outbound: %#v", got.Network)
			}
			if strings.Contains(string(encoded), "allowLocalOutbound") {
				t.Fatalf("empty ports emitted local outbound fields: %s", encoded)
			}
		})
	}
}

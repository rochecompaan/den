package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatsigDenyTakesPrecedenceOverAnthropicWildcard(t *testing.T) {
	network := baseNetwork(t)
	const host = "statsig.anthropic.com"
	if !matchesAnyDomain(network.AllowedDomains, host) {
		t.Fatal("test setup: Statsig must overlap the allowed Anthropic wildcard")
	}
	if !matchesAnyDomain(network.DeniedDomains, host) {
		t.Fatal("test setup: Statsig deny rule missing")
	}
	if policyAllowsDomain(network, host) {
		t.Fatal("Statsig was allowed despite matching deniedDomains")
	}
}

func TestUnmatchedOutboundIsDeniedByDefault(t *testing.T) {
	network := baseNetwork(t)
	const host = "unlisted.example.test"
	if matchesAnyDomain(network.AllowedDomains, host) || matchesAnyDomain(network.DeniedDomains, host) {
		t.Fatal("test setup: unmatched host unexpectedly matches a static rule")
	}
	if policyAllowsDomain(network, host) {
		t.Fatal("unmatched outbound host was allowed")
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
			if got.Network.AllowLocalOutbound != nil || len(got.Network.AllowLocalOutboundPorts) != 0 {
				t.Fatalf("empty ports enabled local outbound: %#v", got.Network)
			}
			if strings.Contains(string(encoded), "allowLocalOutbound") {
				t.Fatalf("empty ports emitted local outbound fields: %s", encoded)
			}
		})
	}
}

func baseNetwork(t *testing.T) network {
	t.Helper()
	var policy document
	if err := json.Unmarshal(readBase(t), &policy); err != nil {
		t.Fatal(err)
	}
	return policy.Network
}

func policyAllowsDomain(network network, host string) bool {
	if matchesAnyDomain(network.DeniedDomains, host) {
		return false
	}
	return matchesAnyDomain(network.AllowedDomains, host)
}

func matchesAnyDomain(patterns []string, host string) bool {
	for _, pattern := range patterns {
		if domainMatches(pattern, host) {
			return true
		}
	}
	return false
}

// Package policy generates one private Fence policy from the immutable base.
package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Base is the immutable, strict-JSON Fence policy.
type Base []byte

// Dynamic contains canonical values approved for one launch. It intentionally
// has no RepoWolf token field.
type Dynamic struct {
	Platform          string
	RepoWolfHostname  string
	CAFile            string
	ClosurePaths      []string
	Worktree          string
	ScratchDir        string
	StatePaths        []string
	DefaultStatePaths []string
	CustomMode        bool
	UnixSockets       []string
	HostPorts         []uint16
	PolicyFile        string
}

type document struct {
	AllowPty   bool       `json:"allowPty"`
	Network    network    `json:"network"`
	Filesystem filesystem `json:"filesystem"`
	Command    command    `json:"command"`
}

type network struct {
	AllowedDomains          []string `json:"allowedDomains"`
	DeniedDomains           []string `json:"deniedDomains"`
	AllowUnixSockets        []string `json:"allowUnixSockets,omitempty"`
	AllowLocalOutbound      *bool    `json:"allowLocalOutbound,omitempty"`
	AllowLocalOutboundPorts []uint16 `json:"allowLocalOutboundPorts,omitempty"`
}

type filesystem struct {
	DefaultDenyRead bool     `json:"defaultDenyRead"`
	StrictDenyRead  bool     `json:"strictDenyRead"`
	AllowGitConfig  bool     `json:"allowGitConfig"`
	AllowRead       []string `json:"allowRead"`
	AllowExecute    []string `json:"allowExecute"`
	AllowWrite      []string `json:"allowWrite"`
	DenyRead        []string `json:"denyRead"`
	DenyWrite       []string `json:"denyWrite"`
}

type command struct {
	Deny                                []string `json:"deny"`
	UseDefaults                         *bool    `json:"useDefaults"`
	AcceptSharedBinaryCannotRuntimeDeny []string `json:"acceptSharedBinaryCannotRuntimeDeny,omitempty"`
	RuntimeExecPolicy                   string   `json:"runtimeExecPolicy,omitempty"`
}

var resolveResolvConf = func() (string, error) {
	return filepath.EvalSymlinks("/etc/resolv.conf")
}

var darwinOperationalReads = []string{
	"/System/Library", "/usr/lib", "/usr/share/icu", "/private/etc", "/private/var/db/timezone",
}

var deniedBrokerHosts = []string{
	"github.com", "*.github.com", "githubusercontent.com", "*.githubusercontent.com",
	"gitlab.com", "*.gitlab.com", "bitbucket.org", "*.bitbucket.org",
}

// Generate validates and adds only per-launch values to base.
func Generate(base Base, dynamic Dynamic) ([]byte, error) {
	policy, err := decodeBase(base)
	if err != nil {
		return nil, err
	}
	if dynamic.Platform != "linux" && dynamic.Platform != "darwin" {
		return nil, errors.New("policy: platform must be linux or darwin")
	}
	if err := validateHostname(dynamic.RepoWolfHostname); err != nil {
		return nil, err
	}
	policy.Network.AllowedDomains = appendUnique(policy.Network.AllowedDomains, dynamic.RepoWolfHostname)

	ca, err := canonicalPath("CA file", dynamic.CAFile)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(ca)
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("policy: CA file must be an existing regular file")
	}
	policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, ca)

	for _, path := range dynamic.ClosurePaths {
		resolved, err := canonicalPath("closure path", path)
		if err != nil {
			return nil, err
		}
		policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, resolved)
		policy.Filesystem.AllowExecute = appendUnique(policy.Filesystem.AllowExecute, resolved)
	}

	worktree, err := addWritable(&policy, "worktree", dynamic.Worktree)
	if err != nil {
		return nil, err
	}
	if _, err := addWritable(&policy, "scratch directory", dynamic.ScratchDir); err != nil {
		return nil, err
	}
	for _, path := range dynamic.StatePaths {
		if _, err := addWritable(&policy, "state path", path); err != nil {
			return nil, err
		}
	}
	for _, path := range dynamic.UnixSockets {
		resolved, err := addUnixSocket(&policy, path)
		if err != nil {
			return nil, err
		}
		if dynamic.Platform == "darwin" {
			policy.Network.AllowUnixSockets = appendUnique(policy.Network.AllowUnixSockets, resolved)
		}
	}

	if dynamic.CustomMode {
		for _, path := range dynamic.DefaultStatePaths {
			resolved, err := canonicalPath("default state path", path)
			if err != nil {
				return nil, err
			}
			policy.Filesystem.DenyWrite = appendUnique(policy.Filesystem.DenyWrite, resolved)
		}
	}
	gitConfigPaths, err := gitConfigDenyPaths(worktree)
	if err != nil {
		return nil, err
	}
	policy.Filesystem.DenyWrite = appendUnique(policy.Filesystem.DenyWrite, gitConfigPaths...)

	policyFile, err := canonicalPath("policy file", dynamic.PolicyFile)
	if err != nil {
		return nil, err
	}
	policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, policyFile)
	policy.Filesystem.DenyWrite = appendUnique(policy.Filesystem.DenyWrite, policyFile, filepath.Dir(policyFile))

	if dynamic.Platform == "linux" {
		reads, err := linuxOperationalReads()
		if err != nil {
			return nil, err
		}
		policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, reads...)
	} else {
		policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, darwinOperationalReads...)
		policy.Command.RuntimeExecPolicy = ""
	}
	if err := addHostPorts(&policy, dynamic.Platform, dynamic.HostPorts); err != nil {
		return nil, err
	}

	encoded, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("policy: encode: %w", err)
	}
	return append(encoded, '\n'), nil
}

func decodeBase(base Base) (document, error) {
	decoder := json.NewDecoder(bytes.NewReader(base))
	decoder.DisallowUnknownFields()
	var policy document
	if err := decoder.Decode(&policy); err != nil {
		return document{}, errors.New("policy: base must be supported strict JSON")
	}
	if err := requireEOF(decoder); err != nil {
		return document{}, err
	}
	return policy, nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("policy: base must contain one JSON value")
	}
	return nil
}

func addWritable(policy *document, name, path string) (string, error) {
	resolved, err := canonicalPath(name, path)
	if err != nil {
		return "", err
	}
	policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, resolved)
	policy.Filesystem.AllowWrite = appendUnique(policy.Filesystem.AllowWrite, resolved)
	return resolved, nil
}

func addUnixSocket(policy *document, path string) (string, error) {
	resolved, err := canonicalPath("Unix socket", path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return "", errors.New("policy: Unix socket must be an existing Unix socket")
	}
	policy.Filesystem.AllowRead = appendUnique(policy.Filesystem.AllowRead, resolved)
	policy.Filesystem.AllowWrite = appendUnique(policy.Filesystem.AllowWrite, resolved)
	return resolved, nil
}

func canonicalPath(name, path string) (string, error) {
	clean := filepath.Clean(path)
	directoryForm := clean != string(os.PathSeparator) && path == clean+string(os.PathSeparator)
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) || (path != clean && !directoryForm) {
		return "", fmt.Errorf("policy: %s must be an absolute clean path", name)
	}

	missing := make([]string, 0)
	current := clean
	for {
		if _, err := os.Lstat(current); err == nil {
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("policy: resolve %s", name)
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("policy: resolve %s", name)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("policy: resolve %s", name)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func linuxOperationalReads() ([]string, error) {
	resolv, err := resolveResolvConf()
	if err != nil {
		return nil, errors.New("policy: resolve /etc/resolv.conf")
	}
	return []string{resolv, "/etc/hosts", "/etc/nsswitch.conf", "/etc/services", "/etc/protocols"}, nil
}

func addHostPorts(policy *document, platform string, ports []uint16) error {
	value := false
	policy.Network.AllowLocalOutbound = &value
	policy.Network.AllowLocalOutboundPorts = nil
	if len(ports) == 0 {
		return nil
	}
	seen := make(map[uint16]struct{}, len(ports))
	for _, port := range ports {
		if port == 0 {
			return errors.New("policy: host port must be between 1 and 65535")
		}
		seen[port] = struct{}{}
	}
	value = true
	policy.Network.AllowLocalOutbound = &value
	if platform == "linux" {
		policy.Network.AllowLocalOutboundPorts = make([]uint16, 0, len(seen))
		for port := range seen {
			policy.Network.AllowLocalOutboundPorts = append(policy.Network.AllowLocalOutboundPorts, port)
		}
		sort.Slice(policy.Network.AllowLocalOutboundPorts, func(i, j int) bool {
			return policy.Network.AllowLocalOutboundPorts[i] < policy.Network.AllowLocalOutboundPorts[j]
		})
	}
	return nil
}

func validateHostname(host string) error {
	if host == "" || strings.ToLower(host) != host || strings.HasSuffix(host, ".") || len(host) > 253 {
		return errors.New("policy: RepoWolf hostname is invalid")
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return errors.New("policy: RepoWolf hostname is invalid")
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return errors.New("policy: RepoWolf hostname is invalid")
			}
		}
	}
	for _, denied := range deniedBrokerHosts {
		if domainMatches(denied, host) {
			return errors.New("policy: RepoWolf hostname is denied")
		}
	}
	return nil
}

func domainMatches(pattern, host string) bool {
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(host, strings.TrimPrefix(pattern, "*"))
	}
	return pattern == host
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; !exists {
			values = append(values, value)
			seen[value] = struct{}{}
		}
	}
	return values
}

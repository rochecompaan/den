// Package repowolf validates the environment inputs required by the RepoWolf client.
package repowolf

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"net"
	"net/url"
	"path/filepath"
	"strings"
)

// Config contains the validated values needed by the RepoWolf client and policy generation.
type Config struct {
	Endpoint string
	Token    string
	CAFile   string
	Hostname string
}

// LoadEnv reads and validates the RepoWolf environment without disclosing its values in errors.
func LoadEnv(lookup func(string) (string, bool), lstat func(string) (fs.FileInfo, error)) (Config, error) {
	endpoint, ok := lookup("REPOWOLF_ENDPOINT")
	if !ok || endpoint == "" || !validEndpoint(endpoint) {
		return Config{}, errors.New("REPOWOLF_ENDPOINT is invalid")
	}
	parsed, _ := url.Parse(endpoint)

	token, ok := lookup("REPOWOLF_TOKEN")
	if !ok || !validToken(token) {
		return Config{}, errors.New("REPOWOLF_TOKEN is invalid")
	}

	caFile, ok := lookup("REPOWOLF_CA_FILE")
	if !ok || caFile == "" {
		return Config{}, errors.New("REPOWOLF_CA_FILE is invalid")
	}
	caFile, err := filepath.Abs(caFile)
	if err != nil {
		return Config{}, errors.New("REPOWOLF_CA_FILE is invalid")
	}
	info, err := lstat(caFile)
	if err != nil || info == nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o444 == 0 {
		return Config{}, errors.New("REPOWOLF_CA_FILE is invalid")
	}

	return Config{
		Endpoint: endpoint,
		Token:    token,
		CAFile:   caFile,
		Hostname: parsed.Hostname(),
	}, nil
}

func validEndpoint(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return false
	}
	hostname := parsed.Hostname()
	if hostname == "" || net.ParseIP(hostname) != nil || !validHostname(hostname) || blockedHostname(hostname) {
		return false
	}
	port := parsed.Port()
	if port != "" && !validPort(port) {
		return false
	}
	return parsed.Host == hostname || (port != "" && parsed.Host == hostname+":"+port)
}

func validHostname(hostname string) bool {
	if hostname != strings.ToLower(hostname) || strings.HasSuffix(hostname, ".") || len(hostname) > 253 {
		return false
	}
	labels := strings.Split(hostname, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
				return false
			}
		}
	}
	return true
}

func blockedHostname(hostname string) bool {
	for _, blocked := range []string{"github.com", "gitlab.com", "bitbucket.org"} {
		if hostname == blocked || strings.HasSuffix(hostname, "."+blocked) {
			return true
		}
	}
	return false
}

func validPort(port string) bool {
	if port == "" {
		return true
	}
	value := 0
	for _, character := range port {
		if character < '0' || character > '9' {
			return false
		}
		value = value*10 + int(character-'0')
		if value > 65535 {
			return false
		}
	}
	return value > 0
}

func validToken(token string) bool {
	const prefix = "rw1_"
	encoded, ok := strings.CutPrefix(token, prefix)
	if !ok || len(encoded) != 43 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == encoded
}

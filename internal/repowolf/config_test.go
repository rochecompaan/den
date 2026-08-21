package repowolf

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testEndpoint = "https://broker.example.test:8443/"
const testToken = "rw1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const testCA = "/certs/ca.pem"

func TestLoadEnvValidatesRequiredValues(t *testing.T) {
	for _, variable := range []string{"REPOWOLF_ENDPOINT", "REPOWOLF_TOKEN", "REPOWOLF_CA_FILE"} {
		for _, test := range []struct {
			name string
			set  func(map[string]string)
		}{
			{"missing", func(values map[string]string) { delete(values, variable) }},
			{"empty", func(values map[string]string) { values[variable] = "" }},
		} {
			t.Run(variable+" "+test.name, func(t *testing.T) {
				values := validValues()
				test.set(values)
				_, err := LoadEnv(lookup(values), readableCA)
				assertRedacted(t, err, variable, testEndpoint, testToken, testCA)
			})
		}
	}
}

func TestLoadEnvAcceptsOnlyCanonicalEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"https://broker.example.test",
		"https://broker.example.test/",
		"https://broker.example.test:443/",
	} {
		t.Run(endpoint, func(t *testing.T) {
			values := validValues()
			values["REPOWOLF_ENDPOINT"] = endpoint
			config, err := LoadEnv(lookup(values), readableCA)
			if err != nil {
				t.Fatalf("LoadEnv() error = %v", err)
			}
			if config.Endpoint != endpoint || config.Hostname != "broker.example.test" {
				t.Fatalf("LoadEnv() config = %#v", config)
			}
		})
	}

	for _, endpoint := range []string{
		"http://broker.example.test",
		"https://user@broker.example.test",
		"https://broker.example.test/non-root",
		"https://broker.example.test/?query=yes",
		"https://broker.example.test/#fragment",
		"https:broker.example.test",
		"https://Broker.example.test",
		"https://broker.example.test.",
		"https://büroker.example.test",
		"https://127.0.0.1",
		"https://[::1]",
		"https://github.com",
		"https://api.github.com",
		"https://gitlab.com",
		"https://internal.gitlab.com",
		"https://bitbucket.org",
		"https://git.bitbucket.org",
		"https://broker.example.test:",
		"https://broker.example.test:0",
		"https://broker.example.test:65536",
	} {
		t.Run("reject "+endpoint, func(t *testing.T) {
			values := validValues()
			values["REPOWOLF_ENDPOINT"] = endpoint
			_, err := LoadEnv(lookup(values), readableCA)
			assertRedacted(t, err, "REPOWOLF_ENDPOINT", endpoint, testToken, testCA)
		})
	}
}

func TestLoadEnvAcceptsOnlyCanonicalToken(t *testing.T) {
	for _, token := range []string{
		"rw1_" + strings.Repeat("A", 42),
		"rw1_" + strings.Repeat("A", 44),
		"rw1_" + strings.Repeat("*", 43),
		"rw1_" + strings.Repeat("A", 42) + "=",
		"rw1_" + strings.Repeat("A", 43) + "\n",
		"wrong_" + strings.Repeat("A", 43),
	} {
		t.Run("reject token", func(t *testing.T) {
			values := validValues()
			values["REPOWOLF_TOKEN"] = token
			_, err := LoadEnv(lookup(values), readableCA)
			assertRedacted(t, err, "REPOWOLF_TOKEN", testEndpoint, token, testCA)
		})
	}
}

func TestLoadEnvValidatesCanonicalCAFile(t *testing.T) {
	values := validValues()
	config, err := LoadEnv(lookup(values), func(path string) (fs.FileInfo, error) {
		if path != testCA {
			t.Fatalf("lstat path = %q, want %q", path, testCA)
		}
		return fakeFileInfo{mode: 0o444}, nil
	})
	if err != nil {
		t.Fatalf("LoadEnv() error = %v", err)
	}
	if config.CAFile != testCA {
		t.Fatalf("CAFile = %q, want %q", config.CAFile, testCA)
	}

	values["REPOWOLF_CA_FILE"] = "relative-ca.pem"
	wantAbsolute, err := filepath.Abs("relative-ca.pem")
	if err != nil {
		t.Fatal(err)
	}
	config, err = LoadEnv(lookup(values), func(path string) (fs.FileInfo, error) {
		if path != wantAbsolute {
			t.Fatalf("lstat path = %q, want %q", path, wantAbsolute)
		}
		return fakeFileInfo{mode: 0o444}, nil
	})
	if err != nil || config.CAFile != wantAbsolute {
		t.Fatalf("relative CA config = %#v, error = %v", config, err)
	}
	values["REPOWOLF_CA_FILE"] = testCA

	for _, test := range []struct {
		name  string
		lstat func(string) (fs.FileInfo, error)
	}{
		{"missing", func(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }},
		{"nil file info", func(string) (fs.FileInfo, error) { return nil, nil }},
		{"unreadable", func(string) (fs.FileInfo, error) { return fakeFileInfo{mode: 0}, nil }},
		{"directory", func(string) (fs.FileInfo, error) { return fakeFileInfo{mode: fs.ModeDir | 0o555}, nil }},
		{"symbolic link", func(string) (fs.FileInfo, error) { return fakeFileInfo{mode: fs.ModeSymlink | 0o777}, nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadEnv(lookup(values), test.lstat)
			assertRedacted(t, err, "REPOWOLF_CA_FILE", testEndpoint, testToken, testCA)
		})
	}
}

func TestLoadEnvRedactsAllEnvironmentValues(t *testing.T) {
	values := validValues()
	values["REPOWOLF_ENDPOINT"] = "https://secret-endpoint.example.invalid/no"
	values["REPOWOLF_TOKEN"] = "rw1_secret-token-value"
	values["REPOWOLF_CA_FILE"] = "/secret/ca.pem"
	_, err := LoadEnv(lookup(values), func(string) (fs.FileInfo, error) { return nil, errors.New("secret lstat error") })
	assertRedacted(t, err, "REPOWOLF_ENDPOINT", values["REPOWOLF_ENDPOINT"], values["REPOWOLF_TOKEN"], values["REPOWOLF_CA_FILE"], "secret lstat error")
}

func validValues() map[string]string {
	return map[string]string{
		"REPOWOLF_ENDPOINT": testEndpoint,
		"REPOWOLF_TOKEN":    testToken,
		"REPOWOLF_CA_FILE":  testCA,
	}
}

func lookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func readableCA(string) (fs.FileInfo, error) { return fakeFileInfo{mode: 0o444}, nil }

func assertRedacted(t *testing.T, err error, field string, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("LoadEnv() error = nil")
	}
	if !strings.Contains(err.Error(), field) {
		t.Fatalf("error = %q, want field %q", err, field)
	}
	for _, secret := range secrets {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaked %q: %q", secret, err)
		}
	}
}

type fakeFileInfo struct{ mode fs.FileMode }

func (f fakeFileInfo) Name() string       { return "ca.pem" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeFileInfo) Sys() any           { return nil }

var _ fs.FileInfo = fakeFileInfo{}

package container

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestResolveDockerPrecedenceAndCanonicalEndpoint(t *testing.T) {
	root := t.TempDir()
	explicit := listenSocket(t, filepath.Join(root, "explicit.sock"))
	environment := listenSocket(t, filepath.Join(root, "environment.sock"))
	xdg := listenSocket(t, filepath.Join(root, "runtime", "docker.sock"))
	home := listenSocket(t, filepath.Join(root, "home", ".docker", "run", "docker.sock"))
	if err := os.Symlink(explicit, filepath.Join(root, "explicit-link.sock")); err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name   string
		config Config
		env    Env
		want   string
	}{
		{"explicit", Config{Enable: true, SocketPath: pointer(filepath.Join(root, "explicit-link.sock"))}, Env{"DOCKER_HOST": "unix:///ignored.sock", "XDG_RUNTIME_DIR": filepath.Join(root, "runtime")}, explicit},
		{"environment", Config{Enable: true}, Env{"DOCKER_HOST": "unix://" + environment, "XDG_RUNTIME_DIR": filepath.Join(root, "runtime")}, environment},
		{"XDG runtime", Config{Enable: true}, Env{"XDG_RUNTIME_DIR": filepath.Join(root, "runtime")}, xdg},
		{"Docker Desktop", Config{Enable: true}, Env{}, home},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ResolveDocker(test.config, test.env, Home(filepath.Join(root, "home")))
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != test.want || got.Endpoint != "unix://"+test.want {
				t.Fatalf("socket = %#v, want path %q and canonical endpoint", got, test.want)
			}
		})
	}
}

func TestResolveDockerRejectsInvalidEndpointsAndTargets(t *testing.T) {
	root := t.TempDir()
	valid := listenSocket(t, filepath.Join(root, "valid.sock"))
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []string{
		"tcp://127.0.0.1:2375", "ssh://host", "http://host", "npipe:////./pipe/docker_engine",
		"unix:/relative.sock", "unix://relative.sock", "unix://host" + valid, "unix:///relative.sock",
		"unix:///tmp/socket?query", "unix:///tmp/socket#fragment", "unix:///tmp/%2fescape", "unix:///tmp/%5Cescape", "unix:///tmp//socket",
	} {
		t.Run(strings.ReplaceAll(endpoint, "/", "_"), func(t *testing.T) {
			_, err := ResolveDocker(Config{Enable: true}, Env{"DOCKER_HOST": endpoint}, Home(root))
			if err == nil {
				t.Fatalf("accepted endpoint %q", endpoint)
			}
		})
	}
	for _, path := range []string{filepath.Join(root, "missing.sock"), regular} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := ResolveDocker(Config{Enable: true, SocketPath: pointer(path)}, Env{}, Home(root))
			if err == nil {
				t.Fatalf("accepted target %q", path)
			}
		})
	}
}

func TestResolvePodmanDiscoveryOwnershipAndPorts(t *testing.T) {
	root := t.TempDir()
	explicit := listenSocket(t, filepath.Join(root, "explicit.sock"))
	environment := listenSocket(t, filepath.Join(root, "environment.sock"))
	xdg := listenSocket(t, filepath.Join(root, "runtime", "podman", "podman.sock"))
	uid := UID(os.Getuid())

	tests := []struct {
		name     string
		config   Config
		env      Env
		platform Platform
		want     string
		wantXDG  string
	}{
		{"explicit", Config{Enable: true, SocketPath: pointer(explicit)}, Env{"CONTAINER_HOST": "unix:///ignored.sock"}, "linux", explicit, "/run/user/" + strconv.Itoa(int(uid))},
		{"environment", Config{Enable: true}, Env{"CONTAINER_HOST": "unix://" + environment}, "linux", environment, "/run/user/" + strconv.Itoa(int(uid))},
		{"XDG runtime", Config{Enable: true}, Env{"XDG_RUNTIME_DIR": filepath.Join(root, "runtime")}, "linux", xdg, filepath.Join(root, "runtime")},
	}
	if os.Geteuid() == 0 {
		fallbackRoot := filepath.Join("/run/user", "424242")
		if _, err := os.Lstat(fallbackRoot); os.IsNotExist(err) {
			fallback := listenSocket(t, filepath.Join(fallbackRoot, "podman", "podman.sock"))
			if err := os.Chown(fallback, 424242, -1); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(fallbackRoot) })
			tests = append(tests, struct {
				name     string
				config   Config
				env      Env
				platform Platform
				want     string
				wantXDG  string
			}{"fallback", Config{Enable: true}, Env{}, "linux", fallback, "/run/user/424242"})
		} else if err != nil {
			t.Fatal(err)
		} else {
			t.Log("fallback discovery skipped because /run/user/424242 already exists")
		}
	} else {
		t.Log("fallback discovery requires a temporary /run/user entry")
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidateUID := uid
			if test.name == "fallback" {
				candidateUID = 424242
			}
			got, err := ResolvePodman(test.config, test.env, Home(root), candidateUID, test.platform)
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != test.want || got.Endpoint != "unix://"+test.want || got.XDGRuntimeDir != test.wantXDG {
				t.Fatalf("socket = %#v", got)
			}
		})
	}

	if _, err := ResolvePodman(Config{Enable: true, SocketPath: pointer(explicit)}, Env{}, Home(root), uid+1, "linux"); err == nil {
		t.Fatal("accepted Podman socket not owned by invoking user")
	}
}

func TestResolvePodmanRejectsInvalidAndDisabledConfigurations(t *testing.T) {
	root := t.TempDir()
	for _, endpoint := range []string{
		"tcp://127.0.0.1:8080", "ssh://host", "http://host", "npipe:////./pipe/podman",
		"unix:/relative.sock", "unix://host/socket", "unix:///tmp/socket?query", "unix:///tmp/%2fescape",
	} {
		t.Run(strings.ReplaceAll(endpoint, "/", "_"), func(t *testing.T) {
			_, err := ResolvePodman(Config{Enable: true}, Env{"CONTAINER_HOST": endpoint}, Home(root), UID(os.Getuid()), "darwin")
			if err == nil {
				t.Fatalf("accepted endpoint %q", endpoint)
			}
		})
	}
	if _, err := ResolvePodman(Config{Enable: true}, Env{}, Home(root), UID(os.Getuid()), "darwin"); err == nil {
		t.Fatal("Darwin Podman discovery succeeded without a socket")
	}
	if _, err := ResolveDocker(Config{Enable: false, SocketPath: pointer("relative")}, Env{"DOCKER_HOST": "tcp://host"}, Home(root)); err != nil {
		t.Fatalf("disabled Docker was not inert: %v", err)
	}
	if _, err := ResolvePodman(Config{Enable: false, SocketPath: pointer("relative")}, Env{"CONTAINER_HOST": "tcp://host"}, Home(root), UID(os.Getuid()), "unsupported"); err != nil {
		t.Fatalf("disabled Podman was not inert: %v", err)
	}
	for _, config := range []Config{{HostPorts: []uint16{1}}, {Enable: true, HostPorts: []uint16{0}}} {
		if _, err := ResolvePodman(config, Env{}, Home(root), UID(os.Getuid()), "linux"); err == nil {
			t.Fatalf("accepted invalid port configuration %#v", config)
		}
	}
	if got, want := CombinePorts([]uint16{8080, 443, 8080}, []uint16{443, 3000}), []uint16{443, 3000, 8080}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CombinePorts() = %#v, want %#v", got, want)
	}
	if got := CombinePorts(nil, nil); len(got) != 0 {
		t.Fatalf("CombinePorts(nil, nil) = %#v", got)
	}
}

func listenSocket(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return path
}

func pointer(value string) *string { return &value }

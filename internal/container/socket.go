// Package container resolves optional local container daemon sockets.
package container

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Config is the container-related manifest configuration used at launch.
type Config struct {
	Enable     bool
	SocketPath *string
	HostPorts  []uint16
}

// Env is the inherited environment available for socket discovery.
type Env map[string]string

// Home is the invoking user's home directory.
type Home string

// UID identifies the invoking user for rootless Podman validation.
type UID uint32

// Platform identifies the target Fence platform.
type Platform string

// Socket is a validated, canonical local Unix socket capability.
type Socket struct {
	Path          string
	Endpoint      string
	XDGRuntimeDir string
}

// ResolveDocker selects and validates the configured local Docker socket.
func ResolveDocker(config Config, env Env, home Home) (Socket, error) {
	return resolveDocker(config, env, home, []string{"/run/docker.sock", "/var/run/docker.sock"})
}

func resolveDocker(config Config, env Env, home Home, defaults []string) (Socket, error) {
	if err := validateConfig("Docker", config); err != nil {
		return Socket{}, err
	}
	if !config.Enable {
		return Socket{}, nil
	}
	if config.SocketPath != nil {
		return resolveSocket("Docker", *config.SocketPath, nil)
	}
	if endpoint, ok := env["DOCKER_HOST"]; ok {
		path, err := unixEndpoint("DOCKER_HOST", endpoint)
		if err != nil {
			return Socket{}, err
		}
		return resolveSocket("Docker", path, nil)
	}

	candidates := make([]string, 0, len(defaults)+2)
	if runtime, ok := env["XDG_RUNTIME_DIR"]; ok && runtime != "" {
		candidates = append(candidates, filepath.Join(runtime, "docker.sock"))
	}
	if home != "" {
		candidates = append(candidates, filepath.Join(string(home), ".docker", "run", "docker.sock"))
	}
	candidates = append(candidates, defaults...)
	return resolveDiscovered("Docker", candidates, nil)
}

// ResolvePodman selects and validates the configured local Podman socket.
func ResolvePodman(config Config, env Env, home Home, uid UID, platform Platform) (Socket, error) {
	return resolvePodman(config, env, home, uid, platform, "/run/user")
}

func resolvePodman(config Config, env Env, home Home, uid UID, platform Platform, runtimeRoot string) (Socket, error) {
	if err := validateConfig("Podman", config); err != nil {
		return Socket{}, err
	}
	if !config.Enable {
		return Socket{}, nil
	}
	if platform != "linux" && platform != "darwin" {
		return Socket{}, errors.New("container: platform must be linux or darwin")
	}

	defaultRuntime := filepath.Join(runtimeRoot, strconv.Itoa(int(uid)))
	runtime, runtimeSet := env["XDG_RUNTIME_DIR"]
	exportedRuntime := ""
	if platform == "linux" && !runtimeSet {
		runtime = defaultRuntime
		exportedRuntime = defaultRuntime
	}
	ownership := func(info os.FileInfo) error {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != uint32(uid) {
			return errors.New("container: Podman socket must be owned by the invoking user")
		}
		return nil
	}
	resolve := func(path string) (Socket, error) {
		socket, err := resolveSocket("Podman", path, ownership)
		if err != nil {
			return Socket{}, err
		}
		if exportedRuntime != "" {
			socket.XDGRuntimeDir = exportedRuntime
		} else if runtimeSet && runtime != "" {
			socket.XDGRuntimeDir = runtime
		}
		return socket, nil
	}
	if config.SocketPath != nil {
		return resolve(*config.SocketPath)
	}
	if endpoint, ok := env["CONTAINER_HOST"]; ok {
		path, err := unixEndpoint("CONTAINER_HOST", endpoint)
		if err != nil {
			return Socket{}, err
		}
		return resolve(path)
	}

	candidates := make([]string, 0, 2)
	if runtime != "" {
		candidates = append(candidates, filepath.Join(runtime, "podman", "podman.sock"))
	}
	candidates = append(candidates, filepath.Join(defaultRuntime, "podman", "podman.sock"))
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Socket{}, fmt.Errorf("container: inspect Podman socket: %w", err)
		}
		return resolve(candidate)
	}
	return Socket{}, errors.New("container: Podman socket was not found")
}

// CombinePorts returns sorted, deduplicated Docker and Podman host ports.
func CombinePorts(docker, podman []uint16) []uint16 {
	seen := make(map[uint16]struct{}, len(docker)+len(podman))
	for _, ports := range [][]uint16{docker, podman} {
		for _, port := range ports {
			seen[port] = struct{}{}
		}
	}
	result := make([]uint16, 0, len(seen))
	for port := range seen {
		result = append(result, port)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func validateConfig(name string, config Config) error {
	if !config.Enable {
		if len(config.HostPorts) != 0 {
			return fmt.Errorf("container: %s host ports require the integration to be enabled", name)
		}
		return nil
	}
	for _, port := range config.HostPorts {
		if port == 0 {
			return fmt.Errorf("container: %s host port must be between 1 and 65535", name)
		}
	}
	return nil
}

func resolveDiscovered(name string, candidates []string, ownership func(os.FileInfo) error) (Socket, error) {
	for _, candidate := range candidates {
		if _, err := os.Lstat(candidate); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Socket{}, fmt.Errorf("container: inspect %s socket: %w", name, err)
		}
		return resolveSocket(name, candidate, ownership)
	}
	return Socket{}, fmt.Errorf("container: %s socket was not found", name)
}

func resolveSocket(name, path string, ownership func(os.FileInfo) error) (Socket, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) {
		return Socket{}, fmt.Errorf("container: %s socket path must be absolute", name)
	}
	if _, err := os.Lstat(path); err != nil {
		return Socket{}, fmt.Errorf("container: inspect %s socket: %w", name, err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Socket{}, fmt.Errorf("container: resolve %s socket", name)
	}
	info, err := os.Stat(resolved)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return Socket{}, fmt.Errorf("container: %s socket must be an existing Unix socket", name)
	}
	if ownership != nil {
		if err := ownership(info); err != nil {
			return Socket{}, err
		}
	}
	return Socket{Path: resolved, Endpoint: "unix://" + resolved}, nil
}

func unixEndpoint(name, endpoint string) (string, error) {
	if !strings.HasPrefix(endpoint, "unix:///") || strings.ContainsAny(endpoint, "?#%") {
		return "", fmt.Errorf("container: %s must be unix:///absolute/path", name)
	}
	path := strings.TrimPrefix(endpoint, "unix://")
	clean := filepath.Clean(path)
	if path == "" || strings.IndexByte(path, 0) >= 0 || !filepath.IsAbs(path) || path != clean {
		return "", fmt.Errorf("container: %s must be unix:///absolute/path", name)
	}
	return path, nil
}

// Package launch orchestrates one launcher invocation.
package launch

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/rochecompaan/den/internal/claude"
	"github.com/rochecompaan/den/internal/configdir"
	"github.com/rochecompaan/den/internal/container"
	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/fence"
	"github.com/rochecompaan/den/internal/manifest"
	"github.com/rochecompaan/den/internal/repowolf"
)

type environmentBuilder func([]string, environment.Controlled) []string

type lifecycleRunner func(context.Context, manifest.Manifest, []string, repowolf.Config, configdir.Selection, func() error, []string, container.Socket, container.Socket, io.Writer) int

// Run executes one validated launcher manifest.
func Run(ctx context.Context, launcherManifest manifest.Manifest, arguments []string) int {
	return runWithLifecycle(ctx, launcherManifest, arguments, os.LookupEnv, os.Lstat, os.Environ, environment.Build, os.Stderr, runFence)
}

// run keeps validation tests isolated from process execution. Production uses
// Run, which injects the mandatory Fence lifecycle above.
func run(ctx context.Context, launcherManifest manifest.Manifest, arguments []string, lookup func(string) (string, bool), lstat func(string) (fs.FileInfo, error), environ func() []string, build environmentBuilder, stderr io.Writer) int {
	return runWithLifecycle(ctx, launcherManifest, arguments, lookup, lstat, environ, build, stderr, func(context.Context, manifest.Manifest, []string, repowolf.Config, configdir.Selection, func() error, []string, container.Socket, container.Socket, io.Writer) int {
		return 0
	})
}

func runWithLifecycle(
	ctx context.Context,
	launcherManifest manifest.Manifest,
	arguments []string,
	lookup func(string) (string, bool),
	lstat func(string) (fs.FileInfo, error),
	environ func() []string,
	build environmentBuilder,
	stderr io.Writer,
	lifecycle lifecycleRunner,
) (exitCode int) {
	config, err := repowolf.LoadEnv(lookup, lstat)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if launcherManifest.Agent.Name == "claude" {
		if err := claude.ValidateArguments(arguments); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if launcherManifest.Platform == "darwin" {
			if err := claude.ValidateDarwinArguments(arguments); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		}
	}
	home, _ := lookup("HOME")
	var inherited *string
	if launcherManifest.Agent.ConfigEnvironment != "" {
		if value, ok := lookup(launcherManifest.Agent.ConfigEnvironment); ok {
			inherited = &value
		}
	}
	selection, err := configdir.Select(
		launcherManifest.ExplicitConfigDir,
		inherited,
		home,
		launcherManifest.ProtectedPathPatterns,
		configdir.Dependencies{ACLProbe: launcherManifest.ACLProbe},
	)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() {
		if err := selection.Rollback(); err != nil {
			fmt.Fprintln(stderr, "configuration directory rollback failed")
			exitCode = 1
		}
	}()
	var revalidateDarwinSettings func() error
	if launcherManifest.Agent.Name == "claude" && launcherManifest.Platform == "darwin" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(stderr, "cannot determine working directory for Claude settings validation")
			return 1
		}
		configDirectory := selection.CanonicalPath
		if selection.Mode == configdir.Default {
			configDirectory = filepath.Join(home, ".claude")
		}
		scopes := claude.DarwinScopes(configDirectory, workingDirectory)
		if err := claude.ValidateDarwinSettings(scopes); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		revalidateDarwinSettings = func() error { return claude.RevalidateDarwinSettings(scopes) }
	}
	containerEnv := containerEnvironment(lookup)
	dockerSocket, err := resolveDocker(launcherManifest.Docker, containerEnv, home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	podmanSocket, err := resolvePodman(launcherManifest.Podman, containerEnv, home, launcherManifest.Platform)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if launcherManifest.Platform == "linux" {
		if err := fence.Preflight(ctx, launcherManifest.FenceExecutable); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	host := environ()
	if launcherManifest.Agent.Name == "claude" && launcherManifest.Platform == "darwin" {
		host = claude.ScrubDarwinEnvironment(host)
	}
	childEnvironment := build(host, environment.Controlled{
		Endpoint:      config.Endpoint,
		Token:         config.Token,
		CAFile:        config.CAFile,
		ClientDir:     launcherManifest.RepoWolfClientDir,
		PathEntries:   launcherManifest.PathEntries,
		DockerHost:    dockerSocket.Endpoint,
		ContainerHost: podmanSocket.Endpoint,
		XDGRuntimeDir: podmanSocket.XDGRuntimeDir,
	})
	if selection.Mode == configdir.Custom {
		childEnvironment = setEnvironment(childEnvironment, launcherManifest.Agent.ConfigEnvironment, selection.CanonicalPath)
	}
	return lifecycle(ctx, launcherManifest, arguments, config, selection, revalidateDarwinSettings, childEnvironment, dockerSocket, podmanSocket, stderr)
}

func resolveDocker(config manifest.ContainerConfig, env container.Env, home string) (container.Socket, error) {
	if !config.Enable && len(config.HostPorts) == 0 {
		return container.Socket{}, nil
	}
	return container.ResolveDocker(container.Config{Enable: config.Enable, SocketPath: config.SocketPath, HostPorts: config.HostPorts}, env, container.Home(home))
}

func resolvePodman(config manifest.ContainerConfig, env container.Env, home, platform string) (container.Socket, error) {
	if !config.Enable && len(config.HostPorts) == 0 {
		return container.Socket{}, nil
	}
	return container.ResolvePodman(container.Config{Enable: config.Enable, SocketPath: config.SocketPath, HostPorts: config.HostPorts}, env, container.Home(home), container.UID(os.Getuid()), container.Platform(platform))
}

func containerEnvironment(lookup func(string) (string, bool)) container.Env {
	values := make(container.Env, 3)
	for _, name := range []string{"DOCKER_HOST", "CONTAINER_HOST", "XDG_RUNTIME_DIR"} {
		if value, ok := lookup(name); ok {
			values[name] = value
		}
	}
	return values
}

// fenceTemporaryEnvironment replaces inherited temporary-directory state with
// the owner-validated per-launch scratch directory.
func fenceTemporaryEnvironment(host []string, scratch string) []string {
	result := make([]string, 0, len(host)+2)
	for _, entry := range host {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name == "TMPDIR" || name == "DEN_FENCE_TMPDIR" || name == "DEN_FENCE_POLICY_FILE" {
			continue
		}
		result = append(result, entry)
	}
	return append(result, "TMPDIR="+scratch, "DEN_FENCE_TMPDIR="+scratch)
}

func setEnvironment(values []string, name, value string) []string {
	if name == "" {
		return values
	}
	result := make([]string, 0, len(values)+1)
	for _, entry := range values {
		key, _, ok := strings.Cut(entry, "=")
		if ok && key == name {
			continue
		}
		result = append(result, entry)
	}
	return append(result, name+"="+value)
}

// Package launch orchestrates one launcher invocation.
package launch

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/rochecompaan/den/internal/arguments"
	"github.com/rochecompaan/den/internal/claude"
	"github.com/rochecompaan/den/internal/configdir"
	"github.com/rochecompaan/den/internal/container"
	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/fence"
	"github.com/rochecompaan/den/internal/manifest"
	"github.com/rochecompaan/den/internal/repowolf"
)

type environmentBuilder func([]string, environment.Controlled) []string
type accountHomeResolver func() (string, error)

type lifecycleRunner func(context.Context, manifest.Manifest, []string, repowolf.Config, []*configdir.Handle, StateInputs, func() error, []string, container.Socket, container.Socket, io.Writer) int

// Run executes one validated launcher manifest.
func Run(ctx context.Context, launcherManifest manifest.Manifest, arguments []string) int {
	return runWithLifecycleAndHome(ctx, launcherManifest, arguments, os.LookupEnv, os.Lstat, os.Environ, environment.Build, os.Stderr, runFence, invokingAccountHome)
}

// run keeps validation tests isolated from process execution. Production uses
// Run, which injects the mandatory Fence lifecycle above.
func run(ctx context.Context, launcherManifest manifest.Manifest, arguments []string, lookup func(string) (string, bool), lstat func(string) (fs.FileInfo, error), environ func() []string, build environmentBuilder, stderr io.Writer) int {
	home, _ := lookup("HOME")
	return runWithLifecycleAndHome(ctx, launcherManifest, arguments, lookup, lstat, environ, build, stderr, func(context.Context, manifest.Manifest, []string, repowolf.Config, []*configdir.Handle, StateInputs, func() error, []string, container.Socket, container.Socket, io.Writer) int {
		return 0
	}, func() (string, error) { return home, nil })
}

func runWithLifecycle(
	ctx context.Context,
	launcherManifest manifest.Manifest,
	userArguments []string,
	lookup func(string) (string, bool),
	lstat func(string) (fs.FileInfo, error),
	environ func() []string,
	build environmentBuilder,
	stderr io.Writer,
	lifecycle lifecycleRunner,
) (exitCode int) {
	return runWithLifecycleAndHome(ctx, launcherManifest, userArguments, lookup, lstat, environ, build, stderr, lifecycle, invokingAccountHome)
}

func runWithLifecycleAndHome(
	ctx context.Context,
	launcherManifest manifest.Manifest,
	userArguments []string,
	lookup func(string) (string, bool),
	lstat func(string) (fs.FileInfo, error),
	environ func() []string,
	build environmentBuilder,
	stderr io.Writer,
	lifecycle lifecycleRunner,
	resolveAccountHome accountHomeResolver,
) (exitCode int) {
	if launcherManifest.Agent.ArgumentPolicy != "" {
		if err := arguments.Validate(launcherManifest.Agent.ArgumentPolicy, launcherManifest.Agent.ReservedFlags, launcherManifest.Agent.ReservedCommands, userArguments); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if launcherManifest.Agent.Name == "claude" {
		if err := claude.ValidateArguments(userArguments); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if launcherManifest.Agent.Name == "claude" && launcherManifest.Platform == "darwin" {
		if err := claude.ValidateDarwinArguments(userArguments); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	config, err := repowolf.LoadEnv(lookup, lstat)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	home, _ := lookup("HOME")
	accountHome, err := resolveAccountHome()
	if err != nil || accountHome == "" || !filepath.IsAbs(accountHome) || filepath.Clean(accountHome) != accountHome {
		fmt.Fprintln(stderr, "invoking account home is unavailable")
		return 1
	}
	inherited := make(map[string]string, len(launcherManifest.StateBindings))
	for _, binding := range launcherManifest.StateBindings {
		if value, ok := lookup(binding.InheritedEnvironment); ok {
			inherited[binding.InheritedEnvironment] = value
		}
	}
	plan, err := configdir.PlanBindings(launcherManifest.StateBindings, inherited, home)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	handles, err := plan.Open(launcherManifest.Platform, configdir.ACLValidator{
		ACLProbe: launcherManifest.ACLProbe, ProtectedHomes: []string{accountHome, home}, ProtectedPathPatterns: launcherManifest.ProtectedPathPatterns,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer func() {
		if err := closeStateHandles(handles); err != nil {
			fmt.Fprintln(stderr, "configuration directory rollback failed")
			exitCode = 1
		}
	}()
	state := StateInputsFrom(handles)
	var revalidateDarwinSettings func() error
	if launcherManifest.Agent.Name == "claude" && launcherManifest.Platform == "darwin" {
		workingDirectory, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(stderr, "cannot determine working directory for Claude settings validation")
			return 1
		}
		configDirectory := filepath.Join(home, ".claude")
		for _, handle := range handles {
			if handle.CanonicalPath != "" && handle.Exports()[0].Name == "CLAUDE_CONFIG_DIR" {
				configDirectory = handle.CanonicalPath
				break
			}
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
	host = environment.Scrub(host, launcherManifest.Agent.Environment.Scrub)
	inputs := buildChildInputs(build(host, environment.Controlled{
		Endpoint:      config.Endpoint,
		Token:         config.Token,
		CAFile:        config.CAFile,
		ClientDir:     launcherManifest.RepoWolfClientDir,
		PathEntries:   launcherManifest.PathEntries,
		DockerHost:    dockerSocket.Endpoint,
		ContainerHost: podmanSocket.Endpoint,
		XDGRuntimeDir: podmanSocket.XDGRuntimeDir,
	}), launcherManifest.Agent, state, userArguments)
	return lifecycle(ctx, launcherManifest, inputs.Arguments, config, handles, state, revalidateDarwinSettings, inputs.Environment, dockerSocket, podmanSocket, stderr)
}

func invokingAccountHome() (string, error) {
	account, err := user.Current()
	if err != nil || account == nil {
		return "", err
	}
	return account.HomeDir, nil
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
	return environment.Overwrite(values, name, value)
}

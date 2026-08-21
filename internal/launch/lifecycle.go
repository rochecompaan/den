package launch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rochecompaan/den/internal/configdir"
	"github.com/rochecompaan/den/internal/container"
	"github.com/rochecompaan/den/internal/manifest"
	"github.com/rochecompaan/den/internal/policy"
	"github.com/rochecompaan/den/internal/process"
	"github.com/rochecompaan/den/internal/repowolf"
	"github.com/rochecompaan/den/internal/tempdir"
)

const staleTemporaryDirectoryAge = 24 * time.Hour

type staleRemover func(string, int, time.Duration) error
type temporaryPair func(string) (string, string, func() error, error)

func runFence(
	ctx context.Context,
	launcherManifest manifest.Manifest,
	arguments []string,
	config repowolf.Config,
	selection configdir.Selection,
	darwinRevalidate func() error,
	environment []string,
	docker, podman container.Socket,
	stderr io.Writer,
) int {
	return runFenceWithTemporary(ctx, launcherManifest, arguments, config, selection, darwinRevalidate, environment, docker, podman, stderr, tempdir.RemoveStale, tempdir.NewPair)
}

func runFenceWithTemporary(
	ctx context.Context,
	launcherManifest manifest.Manifest,
	arguments []string,
	config repowolf.Config,
	selection configdir.Selection,
	darwinRevalidate func() error,
	environment []string,
	docker, podman container.Socket,
	stderr io.Writer,
	removeStale staleRemover,
	newPair temporaryPair,
) int {
	if err := removeStale(launcherManifest.ScratchRoot, os.Getuid(), staleTemporaryDirectoryAge); err != nil {
		fmt.Fprintln(stderr, "temporary directory validation failed")
		return 1
	}
	policyDir, scratchDir, cleanup, err := newPair(launcherManifest.ScratchRoot)
	if err != nil {
		fmt.Fprintln(stderr, "temporary directory creation failed")
		return 1
	}
	defer func() { _ = cleanup() }()

	policyFile := filepath.Join(policyDir, "policy.json")
	contents, err := generatePolicy(launcherManifest, config, selection, scratchDir, policyFile, docker, podman)
	if err != nil {
		fmt.Fprintln(stderr, "Fence policy generation failed")
		return 1
	}
	if err := writeReadOnlyPolicy(policyFile, contents); err != nil {
		fmt.Fprintln(stderr, "Fence policy write failed")
		return 1
	}
	if err := selection.Revalidate(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if darwinRevalidate != nil {
		if err := darwinRevalidate(); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}

	environment = fenceTemporaryEnvironment(environment, scratchDir)
	environment = append(environment, "DEN_FENCE_POLICY_FILE="+policyFile)
	argumentsForFence := []string{"--settings", policyFile, "--expose-host-path", config.CAFile, "--", launcherManifest.Agent.Executable}
	argumentsForFence = append(argumentsForFence, launcherManifest.Agent.MandatoryArgs...)
	argumentsForFence = append(argumentsForFence, arguments...)
	return process.Run(process.Command{
		Path: launcherManifest.FenceExecutable, Args: argumentsForFence, Env: environment,
		Started: selection.Commit,
	}, process.IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, process.Signals{})
}

func generatePolicy(
	launcherManifest manifest.Manifest,
	config repowolf.Config,
	selection configdir.Selection,
	scratchDir, policyFile string,
	docker, podman container.Socket,
) ([]byte, error) {
	base, err := os.ReadFile(launcherManifest.BasePolicy)
	if err != nil {
		return nil, err
	}
	closures, err := closurePaths(launcherManifest.ClosurePathsFile)
	if err != nil {
		return nil, err
	}
	worktree, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	sockets := make([]string, 0, 2)
	if docker.Path != "" {
		sockets = append(sockets, docker.Path)
	}
	if podman.Path != "" {
		sockets = append(sockets, podman.Path)
	}
	return policy.Generate(policy.Base(base), policy.Dynamic{
		Platform: launcherManifest.Platform, RepoWolfHostname: config.Hostname, CAFile: config.CAFile,
		ClosurePaths: closures, Worktree: worktree, ScratchDir: scratchDir,
		StatePaths: selection.WritablePaths, DefaultStatePaths: selection.DeniedDefaultPaths,
		CustomMode: selection.Mode == configdir.Custom, UnixSockets: sockets,
		HostPorts:  container.CombinePorts(launcherManifest.Docker.HostPorts, launcherManifest.Podman.HostPorts),
		PolicyFile: policyFile,
	})
}

func closurePaths(path string) ([]string, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(string(contents), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	if len(paths) == 0 {
		return nil, errors.New("no closure paths")
	}
	return paths, nil
}

func writeReadOnlyPolicy(path string, contents []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(contents); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Chmod(path, 0o400)
}

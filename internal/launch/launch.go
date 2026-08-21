// Package launch orchestrates one launcher invocation.
package launch

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/rochecompaan/den/internal/environment"
	"github.com/rochecompaan/den/internal/manifest"
	"github.com/rochecompaan/den/internal/repowolf"
)

type environmentBuilder func([]string, environment.Controlled) []string

// Run executes one validated launcher manifest.
func Run(ctx context.Context, launcherManifest manifest.Manifest, arguments []string) int {
	return run(ctx, launcherManifest, arguments, os.LookupEnv, os.Lstat, os.Environ, environment.Build, os.Stderr)
}

func run(
	_ context.Context,
	launcherManifest manifest.Manifest,
	_ []string,
	lookup func(string) (string, bool),
	lstat func(string) (fs.FileInfo, error),
	environ func() []string,
	build environmentBuilder,
	stderr io.Writer,
) int {
	config, err := repowolf.LoadEnv(lookup, lstat)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	_ = build(environ(), environment.Controlled{
		Endpoint:    config.Endpoint,
		Token:       config.Token,
		CAFile:      config.CAFile,
		ClientDir:   launcherManifest.RepoWolfClientDir,
		PathEntries: launcherManifest.PathEntries,
	})
	return 0
}

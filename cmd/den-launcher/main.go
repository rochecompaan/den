package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rochecompaan/den/internal/launch"
	"github.com/rochecompaan/den/internal/manifest"
)

type manifestLoader func(string) (manifest.Manifest, error)
type launcher func(context.Context, manifest.Manifest, []string) int

func main() {
	os.Exit(run(context.Background(), os.Args[1:], manifest.Load, launch.Run, os.Stderr))
}

func run(ctx context.Context, arguments []string, load manifestLoader, launch launcher, stderr io.Writer) int {
	if len(arguments) < 3 || arguments[0] != "--manifest" || !filepath.IsAbs(arguments[1]) || arguments[2] != "--" {
		fmt.Fprintln(stderr, "usage: den-launcher --manifest ABSOLUTE_PATH -- USER_ARGS...")
		return 2
	}

	launcherManifest, err := load(arguments[1])
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return launch(ctx, launcherManifest, arguments[3:])
}

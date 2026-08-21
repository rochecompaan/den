package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rochecompaan/den/internal/launch"
	"github.com/rochecompaan/den/internal/manifest"
)

func main() {
	arguments := os.Args[1:]
	if len(arguments) < 3 || arguments[0] != "--manifest" || !filepath.IsAbs(arguments[1]) || arguments[2] != "--" {
		fmt.Fprintln(os.Stderr, "usage: den-launcher --manifest ABSOLUTE_PATH -- USER_ARGS...")
		os.Exit(2)
	}

	launcherManifest, err := manifest.Load(arguments[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(launch.Run(context.Background(), launcherManifest, arguments[3:]))
}

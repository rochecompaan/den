package main

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rochecompaan/den/internal/manifest"
)

func TestRunLoadsManifestForwardsArgumentsAndReturnsLaunchExitCode(t *testing.T) {
	manifestPath := filepath.Join(t.TempDir(), "manifest.json")
	wantManifest := manifest.Manifest{Version: 1}
	userArguments := []string{"--model", "test model", ""}
	var loadedPath string
	var receivedManifest manifest.Manifest
	var receivedArguments []string
	var stderr bytes.Buffer

	got := run(
		context.Background(),
		[]string{"--manifest", manifestPath, "--", "--model", "test model", ""},
		func(path string) (manifest.Manifest, error) {
			loadedPath = path
			return wantManifest, nil
		},
		func(_ context.Context, loaded manifest.Manifest, arguments []string) int {
			receivedManifest = loaded
			receivedArguments = arguments
			return 23
		},
		&stderr,
	)

	if got != 23 {
		t.Fatalf("run() = %d, want 23", got)
	}
	if loadedPath != manifestPath {
		t.Fatalf("loaded path = %q, want %q", loadedPath, manifestPath)
	}
	if !reflect.DeepEqual(receivedManifest, wantManifest) {
		t.Fatalf("received manifest = %#v, want %#v", receivedManifest, wantManifest)
	}
	if !reflect.DeepEqual(receivedArguments, userArguments) {
		t.Fatalf("received arguments = %#v, want %#v", receivedArguments, userArguments)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunRejectsInvalidArgumentLayouts(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
	}{
		{"empty", nil},
		{"missing manifest path", []string{"--manifest"}},
		{"relative manifest path", []string{"--manifest", "manifest.json", "--"}},
		{"missing separator", []string{"--manifest", "/manifest.json"}},
		{"invalid separator", []string{"--manifest", "/manifest.json", "not-a-separator"}},
		{"wrong flag", []string{"--config", "/manifest.json", "--"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			got := run(
				context.Background(),
				test.arguments,
				func(string) (manifest.Manifest, error) {
					t.Fatal("manifest loader was called")
					return manifest.Manifest{}, nil
				},
				func(context.Context, manifest.Manifest, []string) int {
					t.Fatal("launcher was called")
					return 0
				},
				&stderr,
			)
			if got != 2 {
				t.Fatalf("run() = %d, want 2", got)
			}
			if stderr.String() != "usage: den-launcher --manifest ABSOLUTE_PATH -- USER_ARGS...\n" {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestRunReturnsLoadError(t *testing.T) {
	var stderr bytes.Buffer
	got := run(
		context.Background(),
		[]string{"--manifest", "/manifest.json", "--"},
		func(string) (manifest.Manifest, error) {
			return manifest.Manifest{}, errors.New("manifest is invalid")
		},
		func(context.Context, manifest.Manifest, []string) int {
			t.Fatal("launcher was called")
			return 0
		},
		&stderr,
	)

	if got != 2 {
		t.Fatalf("run() = %d, want 2", got)
	}
	if stderr.String() != "manifest is invalid\n" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

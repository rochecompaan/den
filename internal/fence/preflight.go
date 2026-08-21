// Package fence verifies runtime capabilities of the pinned Fence binary.
package fence

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var tableColumns = regexp.MustCompile(`\s{2,}`)

// Preflight requires the pinned Linux Fence probe to report an available
// network namespace. Probe diagnostics are not copied into the error.
func Preflight(ctx context.Context, fenceExecutable string) error {
	if !filepath.IsAbs(fenceExecutable) || filepath.Clean(fenceExecutable) != fenceExecutable {
		return correctiveError()
	}
	output, err := exec.CommandContext(ctx, fenceExecutable, "--linux-features").Output()
	if err != nil {
		return correctiveError()
	}
	if err := parseLinuxFeatures(string(output)); err != nil {
		return correctiveError()
	}
	return nil
}

func parseLinuxFeatures(output string) error {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "Network namespace") {
			continue
		}
		columns := tableColumns.Split(trimmed, -1)
		if len(columns) != 4 || columns[0] != "Network namespace" || columns[1] != "direct network isolation" || columns[2] != "ok" || columns[3] == "" {
			return correctiveError()
		}
		count++
	}
	if count != 1 {
		return correctiveError()
	}
	return nil
}

func correctiveError() error {
	return errors.New("Fence network namespace is unavailable; enable unprivileged user namespaces and bubblewrap network isolation, then retry")
}

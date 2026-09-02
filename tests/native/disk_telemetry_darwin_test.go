//go:build native && darwin

package native

import (
	"fmt"
	"os"
	"syscall"
)

type darwinTelemetryPath struct {
	label string
	path  string
}

func reportDarwinDiskTelemetry(phase string, paths ...darwinTelemetryPath) {
	if os.Getenv("DEN_CI_DISK_TELEMETRY") != "1" {
		return
	}
	for _, candidate := range paths {
		var statistics syscall.Statfs_t
		if err := syscall.Statfs(candidate.path, &statistics); err != nil {
			fmt.Fprintf(os.Stderr, "darwin disk telemetry phase=%s path=%s unavailable=%v\n", phase, candidate.label, err)
			continue
		}
		availableKiB := statistics.Bavail * uint64(statistics.Bsize) / 1024
		freeKiB := statistics.Bfree * uint64(statistics.Bsize) / 1024
		totalKiB := statistics.Blocks * uint64(statistics.Bsize) / 1024
		fmt.Fprintf(os.Stderr,
			"darwin disk telemetry phase=%s path=%s fsid=%d:%d total-kib=%d free-kib=%d available-kib=%d\n",
			phase, candidate.label, statistics.Fsid.Val[0], statistics.Fsid.Val[1], totalKiB, freeKiB, availableKiB)
	}
}

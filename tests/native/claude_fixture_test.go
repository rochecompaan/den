//go:build native

package native

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestScratchIsolationWitnessRejectsForbiddenAccess(t *testing.T) {
	own := t.TempDir()
	claudeTmp := filepath.Join(own, "claude-501")
	if err := os.Mkdir(claudeTmp, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, "marker"), []byte("own"), 0o600); err != nil {
		t.Fatal(err)
	}
	missingPeer := filepath.Join(t.TempDir(), "missing-peer")
	readablePeer := t.TempDir()
	if err := os.WriteFile(filepath.Join(readablePeer, "marker"), []byte("peer"), 0o600); err != nil {
		t.Fatal(err)
	}
	deniedProbe := filepath.Join("/dev/null", "probe")

	tests := []struct {
		name         string
		peer         string
		tmpProbe     string
		privateProbe string
		wantSuccess  bool
	}{
		{name: "denied peer and probes", peer: missingPeer, tmpProbe: deniedProbe, privateProbe: deniedProbe, wantSuccess: true},
		{name: "readable peer", peer: readablePeer, tmpProbe: deniedProbe, privateProbe: deniedProbe},
		{name: "writable tmp probe", peer: missingPeer, tmpProbe: filepath.Join(t.TempDir(), "probe"), privateProbe: deniedProbe},
		{name: "writable private tmp probe", peer: missingPeer, tmpProbe: deniedProbe, privateProbe: filepath.Join(t.TempDir(), "probe")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("sh", "-c", "set -eu"+scratchIsolationWitnessCommand(own, test.peer))
			command.Env = []string{
				"DEN_FENCE_TMPDIR=" + own,
				"DEN_NATIVE_PRIVATE_TMP_PROBE=" + test.privateProbe,
				"DEN_NATIVE_TMP_PROBE=" + test.tmpProbe,
				"PATH=" + os.Getenv("PATH"),
				"TMPDIR=" + claudeTmp,
			}
			output, err := command.CombinedOutput()
			if (err == nil) != test.wantSuccess {
				t.Fatalf("scratch isolation witness success = %v, want %v\n%s", err == nil, test.wantSuccess, output)
			}
		})
	}
}

func TestScratchBarrierStopsWhenPeerResultHasNoPath(t *testing.T) {
	provider := newClaudeProvider()
	waiting := provider.registerScratch("first")
	failed := provider.registerScratch("first")
	waiting.results = []string{"scratch:/waiting"}

	type response struct {
		content map[string]any
		stop    string
	}
	done := make(chan response, 1)
	waiterStarted := make(chan struct{})
	go func() {
		provider.mu.Lock()
		close(waiterStarted)
		content, stop := provider.nextContentLocked(waiting)
		provider.mu.Unlock()
		done <- response{content: content, stop: stop}
	}()

	<-waiterStarted
	provider.mu.Lock()
	failed.results = []string{"Exit code 1"}
	provider.ready.Broadcast()
	provider.mu.Unlock()

	select {
	case result := <-done:
		if result.stop != "end_turn" || result.content["text"] != "fixture scratch barrier failed" {
			t.Fatalf("barrier response = %#v, %q", result.content, result.stop)
		}
	case <-time.After(100 * time.Millisecond):
		provider.mu.Lock()
		failed.results = []string{"scratch:/failed"}
		provider.ready.Broadcast()
		provider.mu.Unlock()
		t.Fatal("scratch barrier kept waiting after a peer returned no scratch path")
	}
}

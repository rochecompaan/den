//go:build native

package native

import (
	"testing"
	"time"
)

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

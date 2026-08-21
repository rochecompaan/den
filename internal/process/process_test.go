package process

import (
	"bytes"
	"io"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestRunPreservesStreamsArgumentsAndExitStatus(t *testing.T) {
	if os.Getenv("PROCESS_HELPER") == "1" {
		body, _ := io.ReadAll(os.Stdin)
		_, _ = os.Stdout.Write(append([]byte("stdout:"), body...))
		_, _ = os.Stderr.Write([]byte("stderr:ok"))
		os.Exit(23)
	}
	var stdout, stderr bytes.Buffer
	code := Run(Command{
		Path: os.Args[0],
		Args: []string{"-test.run=TestRunPreservesStreamsArgumentsAndExitStatus"},
		Env:  append(os.Environ(), "PROCESS_HELPER=1"),
	}, IO{Stdin: bytes.NewBufferString("input"), Stdout: &stdout, Stderr: &stderr}, Signals{Notify: signal.Notify, Stop: signal.Stop})
	if code != 23 {
		t.Fatalf("Run() = %d, want 23", code)
	}
	if stdout.String() != "stdout:input" || stderr.String() != "stderr:ok" {
		t.Fatalf("streams = %q, %q", stdout.String(), stderr.String())
	}
}

func TestRunReturns128PlusSignal(t *testing.T) {
	code := Run(Command{Path: "/bin/sh", Args: []string{"-c", "kill -TERM $$"}, Env: os.Environ()}, IO{}, Signals{})
	if code != 128+int(syscall.SIGTERM) {
		t.Fatalf("Run() = %d, want %d", code, 128+int(syscall.SIGTERM))
	}
}

func TestRunForwardsTerminatingSignals(t *testing.T) {
	for _, forwarded := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT} {
		t.Run(forwarded.String(), func(t *testing.T) {
			started := make(chan struct{})
			go func() {
				<-started
				time.Sleep(50 * time.Millisecond)
				_ = syscall.Kill(os.Getpid(), forwarded)
			}()
			code := Run(Command{
				Path:    "/bin/sh",
				Args:    []string{"-c", "trap 'exit 0' INT TERM HUP QUIT; while :; do sleep 1; done"},
				Env:     os.Environ(),
				Started: func() { close(started) },
			}, IO{}, Signals{Notify: signal.Notify, Stop: signal.Stop})
			if code != 0 {
				t.Fatalf("Run() = %d, want 0", code)
			}
		})
	}
}

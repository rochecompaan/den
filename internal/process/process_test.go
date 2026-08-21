package process

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"syscall"
	"testing"
	"time"
)

func TestRunPreservesStreamsArgumentsAndExitStatus(t *testing.T) {
	if os.Getenv("PROCESS_HELPER") == "1" {
		if os.Getenv("PROCESS_EXPECT_EMPTY_ARGUMENT") == "1" {
			if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "" {
				os.Exit(1)
			}
			_, _ = os.Stdout.Write([]byte("empty-argument-ok"))
			os.Exit(0)
		}
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

func TestRunPreservesEmptyStringArgument(t *testing.T) {
	var stdout bytes.Buffer
	code := Run(Command{
		Path: os.Args[0], Args: []string{"-test.run=TestRunPreservesStreamsArgumentsAndExitStatus", ""},
		Env: append(os.Environ(), "PROCESS_HELPER=1", "PROCESS_EXPECT_EMPTY_ARGUMENT=1"),
	}, IO{Stdout: &stdout}, Signals{})
	if code != 0 || stdout.String() != "empty-argument-ok" {
		t.Fatalf("Run() = %d, output = %q; empty argument was not preserved", code, stdout.String())
	}
}

func TestRunLeavesResizeAndJobControlSignalsUnregistered(t *testing.T) {
	var registered []os.Signal
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatal(err)
	}
	code := Run(Command{Path: truePath, Env: os.Environ()}, IO{}, Signals{
		Notify: func(_ chan<- os.Signal, signals ...os.Signal) { registered = append(registered, signals...) },
		Stop:   func(chan<- os.Signal) {},
	})
	if code != 0 {
		t.Fatalf("Run() = %d", code)
	}
	want := []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered signals = %#v, want %#v; SIGWINCH/SIGTSTP/SIGCONT must remain terminal-managed", registered, want)
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

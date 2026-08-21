package process

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"strings"
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

func TestRunPreservesEmptyArguments(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatal(err)
	}
	if code := Run(Command{Path: truePath, Env: os.Environ()}, IO{}, Signals{}); code != 0 {
		t.Fatalf("Run() = %d, want 0", code)
	}
}

func TestRunPreservesForegroundPTYProcessGroup(t *testing.T) {
	script, err := exec.LookPath("script")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	command := "test -t 0; pgrp=$(ps -o pgrp= -p $$ | tr -d ' '); tpgid=$(ps -o tpgid= -p $$ | tr -d ' '); test \"$pgrp\" = \"$tpgid\"; printf pty-ok"
	code := Run(Command{Path: script, Args: []string{"-qfec", command, "/dev/null"}, Env: os.Environ()}, IO{Stdout: &stdout, Stderr: &stdout}, Signals{})
	if code != 0 || !strings.Contains(stdout.String(), "pty-ok") {
		t.Fatalf("Run() = %d, PTY output = %q", code, stdout.String())
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

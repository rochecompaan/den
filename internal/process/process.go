// Package process supervises Fence while preserving terminal process behavior.
package process

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Command describes the already-validated Fence command.
type Command struct {
	Path    string
	Args    []string
	Env     []string
	Dir     string
	Started func()
}

// IO connects the launcher's standard streams directly to Fence.
type IO struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Signals supplies process signal registration. Zero values use os/signal.
type Signals struct {
	Notify func(chan<- os.Signal, ...os.Signal)
	Stop   func(chan<- os.Signal)
}

// Run starts Fence and returns its exit status. It does not create a process
// group, so the terminal's foreground group and job-control behavior survive.
func Run(command Command, streams IO, signals Signals) int {
	notify := signals.Notify
	if notify == nil {
		notify = signal.Notify
	}
	stop := signals.Stop
	if stop == nil {
		stop = signal.Stop
	}
	forwarded := make(chan os.Signal, 1)
	notify(forwarded, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)

	cmd := exec.Command(command.Path, command.Args...)
	cmd.Env = command.Env
	cmd.Dir = command.Dir
	cmd.Stdin = streams.Stdin
	cmd.Stdout = streams.Stdout
	cmd.Stderr = streams.Stderr
	if err := cmd.Start(); err != nil {
		stop(forwarded)
		return 1
	}
	if command.Started != nil {
		command.Started()
	}
	done := make(chan struct{})
	halt := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case received := <-forwarded:
				_ = cmd.Process.Signal(received)
			case <-halt:
				return
			}
		}
	}()

	err := cmd.Wait()
	stop(forwarded)
	close(halt)
	<-done
	return exitCode(err)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitError, ok := err.(*exec.ExitError); ok {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
			if status.Signaled() {
				return 128 + int(status.Signal())
			}
			return status.ExitStatus()
		}
	}
	return 1
}

// Command process-harness exercises process.Run from an already foreground PTY.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/rochecompaan/den/internal/process"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	if err := requireForegroundPTY(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "pty":
		os.Exit(run("test -t 0; pgrp=$(ps -o pgrp= -p $$ | tr -d ' '); tpgid=$(ps -o tpgid= -p $$ | tr -d ' '); test \"$pgrp\" = \"$tpgid\"; printf 'pty-ok\\n'"))
	case "job-control":
		if err := os.WriteFile(os.Getenv("DEN_PROCESS_PID_FILE"), []byte(strconv.Itoa(syscall.Getpgrp())), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(run("trap 'printf W >> \"$DEN_PROCESS_SIGNAL_FILE\"' WINCH; trap 'printf T >> \"$DEN_PROCESS_SIGNAL_FILE\"' TSTP; trap 'printf C >> \"$DEN_PROCESS_SIGNAL_FILE\"; exit 0' CONT; printf R > \"$DEN_PROCESS_READY_FILE\"; while :; do sleep 1; done"))
	default:
		os.Exit(2)
	}
}

func requireForegroundPTY() error {
	if !terminalGroupMatches(os.Getpid()) {
		return fmt.Errorf("process harness is not in the foreground PTY group")
	}
	return nil
}

func run(script string) int {
	shell, err := exec.LookPath("sh")
	if err != nil {
		return 1
	}
	return process.Run(process.Command{Path: shell, Args: []string{"-c", script}, Env: os.Environ()}, process.IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}, process.Signals{})
}

func terminalGroupMatches(pid int) bool {
	output, err := exec.Command("ps", "-o", "tpgid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	group, err := strconv.Atoi(strings.TrimSpace(string(output)))
	return err == nil && group == syscall.Getpgrp()
}

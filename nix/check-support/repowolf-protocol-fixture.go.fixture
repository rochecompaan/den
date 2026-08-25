//go:build repowolf_native_fixture

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"sync"

	repowolfv1 "github.com/rochecompaan/repowolf/gen/repowolf/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type fixture struct {
	repowolfv1.UnimplementedGitHubServiceServer
	repowolfv1.UnimplementedGitServiceServer
	remote string
	log    string
	mu     sync.Mutex
}

func (f *fixture) record(operation string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.OpenFile(f.log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintln(file, operation)
}

func (f *fixture) Execute(context.Context, *repowolfv1.GitHubRequest) (*repowolfv1.GitHubResponse, error) {
	f.record("gh")
	return nil, status.Error(codes.PermissionDenied, "fixture recorded request")
}

func (f *fixture) UploadPack(stream grpc.BidiStreamingServer[repowolfv1.GitFrame, repowolfv1.GitFrame]) error {
	f.record("git-upload-pack")
	return f.relay(stream, "git-upload-pack")
}

func (f *fixture) ReceivePack(stream grpc.BidiStreamingServer[repowolfv1.GitFrame, repowolfv1.GitFrame]) error {
	f.record("git-receive-pack")
	return f.relay(stream, "git-receive-pack")
}

func (f *fixture) relay(stream grpc.BidiStreamingServer[repowolfv1.GitFrame, repowolfv1.GitFrame], program string) error {
	first, err := stream.Recv()
	if err != nil || first.GetOpen() == nil {
		return status.Error(codes.InvalidArgument, "open frame required")
	}
	command := exec.CommandContext(stream.Context(), program, f.remote)
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	inputDone := make(chan error, 1)
	go func() {
		defer stdin.Close()
		for {
			frame, receiveErr := stream.Recv()
			if receiveErr != nil {
				if receiveErr == io.EOF {
					inputDone <- nil
				} else {
					inputDone <- receiveErr
				}
				return
			}
			if data := frame.GetData(); data != nil {
				if _, writeErr := stdin.Write(data.Data); writeErr != nil {
					inputDone <- writeErr
					return
				}
			}
		}
	}()
	buffer := make([]byte, 64<<10)
	for {
		count, readErr := stdout.Read(buffer)
		if count > 0 {
			frame := &repowolfv1.GitFrame{Payload: &repowolfv1.GitFrame_Data{Data: &repowolfv1.GitData{Data: append([]byte(nil), buffer[:count]...)}}}
			if err := stream.Send(frame); err != nil {
				_ = command.Process.Kill()
				return err
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = command.Process.Kill()
				return readErr
			}
			break
		}
	}
	if err := command.Wait(); err != nil {
		return err
	}
	if err := <-inputDone; err != nil {
		return err
	}
	return stream.Send(&repowolfv1.GitFrame{Payload: &repowolfv1.GitFrame_Terminal{Terminal: &repowolfv1.GitTerminal{
		Category: repowolfv1.GitTerminalCategory_GIT_TERMINAL_CATEGORY_COMPLETED,
	}}})
}

func main() {
	listen := flag.String("listen", "", "loopback listen address")
	certificate := flag.String("certificate", "", "TLS certificate")
	key := flag.String("key", "", "TLS key")
	remote := flag.String("remote", "", "bare Git fixture")
	log := flag.String("log", "", "operation log")
	flag.Parse()
	if *listen == "" || *certificate == "" || *key == "" || *remote == "" || *log == "" {
		fmt.Fprintln(os.Stderr, "all fixture flags are required")
		os.Exit(2)
	}
	pair, err := tls.LoadX509KeyPair(*certificate, *key)
	if err != nil {
		panic(err)
	}
	listener, err := net.Listen("tcp4", *listen)
	if err != nil {
		panic(err)
	}
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13})))
	service := &fixture{remote: *remote, log: *log}
	repowolfv1.RegisterGitHubServiceServer(server, service)
	repowolfv1.RegisterGitServiceServer(server, service)
	if err := server.Serve(listener); err != nil {
		panic(err)
	}
}

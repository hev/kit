//go:build !windows

package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"testing"
	"time"
)

func TestServeSIGINTShutdown(t *testing.T) {
	if os.Getenv("HEV_SERVE_SIGNAL_HELPER") == "1" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			os.Exit(2)
		}
		fmt.Println("ready")
		if serveUntilCanceled(ctx, ln, http.NewServeMux()) != nil {
			os.Exit(3)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestServeSIGINTShutdown$")
	cmd.Env = append(os.Environ(), "HEV_SERVE_SIGNAL_HELPER=1")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	ready := make(chan bool, 1)
	go func() { ready <- bufio.NewScanner(pipe).Scan() }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("no readiness")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("startup timeout")
	}
	if err = cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("SIGINT did not stop server")
	}
}

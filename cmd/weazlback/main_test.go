package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

func TestBackendIgnoresTerminalHangup(t *testing.T) {
	if os.Getenv("WEAZLBACK_HANGUP_HELPER") == "1" {
		ignoreTerminalHangup()
		fmt.Println("ready")
		term := make(chan os.Signal, 1)
		signalNotifyTERM(term)
		<-term
		return
	}
	command := exec.Command(os.Args[0], "-test.run=TestBackendIgnoresTerminalHangup")
	command.Env = append(os.Environ(), "WEAZLBACK_HANGUP_HELPER=1")
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if !bufio.NewScanner(stdout).Scan() {
		t.Fatal("helper did not become ready")
	}
	if err := command.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := command.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("backend died after SIGHUP: %v", err)
	}
	_ = command.Process.Signal(syscall.SIGTERM)
	if err := command.Wait(); err != nil {
		t.Fatalf("helper did not exit cleanly: %v", err)
	}
}

func signalNotifyTERM(channel chan<- os.Signal) {
	// Isolated for the subprocess helper so the parent test's signal handling is
	// never modified.
	signal.Notify(channel, syscall.SIGTERM)
}

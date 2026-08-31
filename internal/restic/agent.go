package restic

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

type keyAgent struct {
	Path     string
	listener net.Listener
	once     sync.Once
}

func startAgent(privateKey []byte) (*keyAgent, error) {
	if len(privateKey) == 0 {
		return nil, nil
	}
	parsed, err := ssh.ParseRawPrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("parse SSH private key: %w", err)
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	path := filepath.Join(dir, fmt.Sprintf("weazlback-agent-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		os.Remove(path)
		return nil, err
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: parsed, Comment: "weazlback-ephemeral"}); err != nil {
		listener.Close()
		os.Remove(path)
		return nil, err
	}
	server := &keyAgent{Path: path, listener: listener}
	go server.serve(keyring)
	return server, nil
}

func (a *keyAgent) serve(keyring agent.Agent) {
	for {
		connection, err := a.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer connection.Close()
			_ = agent.ServeAgent(keyring, connection)
		}()
	}
}

func (a *keyAgent) Close() error {
	if a == nil {
		return nil
	}
	var result error
	a.once.Do(func() {
		if err := a.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			result = err
		}
		if err := os.Remove(a.Path); err != nil && !errors.Is(err, os.ErrNotExist) && result == nil {
			result = err
		}
	})
	return result
}

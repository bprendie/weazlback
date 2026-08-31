package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/bprendie/weazlback/internal/restic"
	"github.com/bprendie/weazlback/internal/vault"
)

type agentRequest struct {
	Action string `json:"action"`
}

type agentResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func widgetAgent(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(io.LimitReader(stdin, 64*1024))
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	pass := []byte(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
	if len(pass) == 0 {
		return errors.New("Vault passphrase is required")
	}
	cfg, destination, v, err := loadRuntimeWithPassphrase("", pass)
	zero(pass)
	if err != nil {
		return err
	}
	defer v.Lock()
	path, err := agentSocketPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Remove(path)
	listener, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	defer func() { listener.Close(); os.Remove(path) }()
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Vault Open")
	if flusher, ok := stdout.(interface{ Sync() error }); ok {
		_ = flusher.Sync()
	}
	agentCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _, _ = io.Copy(io.Discard, reader); cancel(); listener.Close() }()
	var operation sync.Mutex
	for {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			if agentCtx.Err() != nil {
				return nil
			}
			return acceptErr
		}
		go serveAgentRequest(agentCtx, connection, &operation, cfg, destination, v)
	}
}

func serveAgentRequest(ctx context.Context, connection net.Conn, operation *sync.Mutex, cfg config.Config, destination *config.Destination, v *vault.File) {
	defer connection.Close()
	var request agentRequest
	if err := json.NewDecoder(connection).Decode(&request); err != nil {
		_ = json.NewEncoder(connection).Encode(agentResponse{Error: "invalid agent request"})
		return
	}
	if request.Action == "status" {
		_ = json.NewEncoder(connection).Encode(agentResponse{OK: true})
		return
	}
	if request.Action == "check" {
		operation.Lock()
		defer operation.Unlock()
		repo, err := repositoryFrom(v, *destination)
		if err == nil {
			err = restic.NewService(io.Discard).Check(ctx, repo, false)
		}
		response := agentResponse{OK: err == nil}
		if err != nil {
			response.Error = err.Error()
		}
		_ = json.NewEncoder(connection).Encode(response)
		return
	}
	operation.Lock()
	defer operation.Unlock()
	profiles := []string{"core", "home"}
	if request.Action == "heavy" {
		profiles = []string{"heavy"}
	} else if request.Action != "backup" {
		_ = json.NewEncoder(connection).Encode(agentResponse{Error: "unsupported agent action"})
		return
	}
	err := runUnlockedWidgetBackup(ctx, cfg, destination, v, profiles, io.Discard)
	response := agentResponse{OK: err == nil}
	if err != nil {
		response.Error = err.Error()
	}
	_ = json.NewEncoder(connection).Encode(response)
}

func requestVaultAgent(ctx context.Context, action string) error {
	path, err := agentSocketPath()
	if err != nil {
		return err
	}
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		return errors.New("Vault is locked; open the Weazlback widget")
	}
	defer connection.Close()
	if err := json.NewEncoder(connection).Encode(agentRequest{Action: action}); err != nil {
		return err
	}
	var response agentResponse
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	return nil
}

func agentSocketPath() (string, error) {
	if root := os.Getenv("WEAZLBACK_HOME"); root != "" {
		return filepath.Join(root, "run", "agent.sock"), nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "weazlback", "agent.sock"), nil
}

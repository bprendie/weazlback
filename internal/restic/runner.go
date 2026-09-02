package restic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

type Repository struct {
	Location           string
	Password           []byte
	SSHKey             []byte
	KnownHosts         string
	Elevated           bool
	GraphicalElevation bool
	Connections        int
	UploadLimitKiB     int
}

type Runner struct {
	Binary string
	Stderr io.Writer
}

func New(stderr io.Writer) Runner { return Runner{Binary: "restic", Stderr: stderr} }

func (r Runner) Run(ctx context.Context, repo Repository, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	err := r.run(ctx, repo, &stdout, nil, args...)
	if r.shouldRecoverStaleLock(args, err) {
		if unlockErr := r.run(ctx, repo, io.Discard, nil, "unlock"); unlockErr == nil {
			stdout.Reset()
			err = r.run(ctx, repo, &stdout, nil, args...)
		}
	}
	return stdout.Bytes(), err
}

func (r Runner) RunWithNewPassword(ctx context.Context, repo Repository, newPassword []byte, args ...string) ([]byte, error) {
	var stdout bytes.Buffer
	err := r.run(ctx, repo, &stdout, newPassword, args...)
	return stdout.Bytes(), err
}

func (r Runner) Stream(ctx context.Context, repo Repository, event func([]byte), args ...string) error {
	err := r.run(ctx, repo, &lineWriter{event: event}, nil, args...)
	if r.shouldRecoverStaleLock(args, err) {
		if unlockErr := r.run(ctx, repo, io.Discard, nil, "unlock"); unlockErr == nil {
			err = r.run(ctx, repo, &lineWriter{event: event}, nil, args...)
		}
	}
	return err
}

func (Runner) shouldRecoverStaleLock(args []string, err error) bool {
	return len(args) > 0 && args[0] != "unlock" && repositoryLocked(err)
}

func (r Runner) run(ctx context.Context, repo Repository, stdout io.Writer, newPassword []byte, args ...string) error {
	if len(repo.Password) == 0 {
		return errors.New("repository password is empty")
	}
	passwordRead, passwordWrite, err := os.Pipe()
	if err != nil {
		return err
	}
	defer passwordRead.Close()
	var newRead, newWrite *os.File
	if len(newPassword) > 0 {
		newRead, newWrite, err = os.Pipe()
		if err != nil {
			passwordWrite.Close()
			return err
		}
		defer newRead.Close()
		defer newWrite.Close()
	}
	agent, err := startAgent(repo.SSHKey)
	if err != nil {
		passwordWrite.Close()
		return err
	}
	if agent != nil {
		defer agent.Close()
	}
	cmdArgs := []string{"-r", repo.Location}
	connections := repo.Connections
	if connections == 0 {
		connections = 4
	}
	if connections > 0 {
		backend := "local"
		if strings.HasPrefix(repo.Location, "sftp:") {
			backend = "sftp"
		}
		cmdArgs = append(cmdArgs, "-o", fmt.Sprintf("%s.connections=%d", backend, connections))
	}
	if repo.UploadLimitKiB > 0 {
		cmdArgs = append(cmdArgs, "--limit-upload", fmt.Sprint(repo.UploadLimitKiB))
	}
	if agent != nil {
		sshArgs := "-F /dev/null -oBatchMode=yes -oIdentitiesOnly=no"
		if repo.KnownHosts != "" {
			if strings.ContainsAny(repo.KnownHosts, " \t\r\n") {
				return errors.New("known_hosts path must not contain whitespace")
			}
			sshArgs += " -oStrictHostKeyChecking=yes -oUserKnownHostsFile=" + repo.KnownHosts
		}
		cmdArgs = append(cmdArgs, "-o", "sftp.args="+sshArgs)
	}
	cmdArgs = append(cmdArgs, args...)
	passwordPath := "/proc/self/fd/3"
	var cmd *exec.Cmd
	if repo.Elevated && repo.GraphicalElevation {
		passwordPath = fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), passwordRead.Fd())
		pkexecArgs := []string{"env", "RESTIC_PASSWORD_FILE=" + passwordPath}
		if agent != nil {
			pkexecArgs = append(pkexecArgs, "SSH_AUTH_SOCK="+agent.Path)
		}
		pkexecArgs = append(pkexecArgs, r.Binary)
		cmd = exec.CommandContext(ctx, "pkexec", append(pkexecArgs, cmdArgs...)...)
		cmd.Stdin = os.Stdin
	} else if repo.Elevated {
		passwordPath = fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), passwordRead.Fd())
		sudoArgs := []string{"-n", "--preserve-env=SSH_AUTH_SOCK", "env", "RESTIC_PASSWORD_FILE=" + passwordPath, r.Binary}
		cmd = exec.CommandContext(ctx, "sudo", append(sudoArgs, cmdArgs...)...)
		cmd.Stdin = os.Stdin
	} else {
		cmd = exec.CommandContext(ctx, r.Binary, cmdArgs...)
		cmd.ExtraFiles = []*os.File{passwordRead}
		cmd.Env = append(os.Environ(), "RESTIC_PASSWORD_FILE="+passwordPath)
		if newRead != nil {
			cmd.ExtraFiles = append(cmd.ExtraFiles, newRead)
			cmd.Env = append(cmd.Env, "RESTIC_NEW_PASSWORD_FILE=/proc/self/fd/4")
		}
	}
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	if agent != nil {
		cmd.Env = append(cmd.Env, "SSH_AUTH_SOCK="+agent.Path)
	}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, io.MultiWriter(&stderr, writerOrDiscard(r.Stderr))
	if err := cmd.Start(); err != nil {
		passwordWrite.Close()
		return err
	}
	_, passErr := passwordWrite.Write(append(append([]byte{}, repo.Password...), '\n'))
	passwordWrite.Close()
	var newPassErr error
	if newWrite != nil {
		_, newPassErr = newWrite.Write(append(append([]byte{}, newPassword...), '\n'))
		_ = newWrite.Close()
	}
	if passErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return passErr
	}
	if newPassErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return newPassErr
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("restic: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (r Runner) JSON(ctx context.Context, repo Repository, out any, args ...string) error {
	b, err := r.Run(ctx, repo, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

func shellWord(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

type lineWriter struct {
	event func([]byte)
	buf   []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		index := bytes.IndexByte(w.buf, '\n')
		if index < 0 {
			break
		}
		line := append([]byte(nil), w.buf[:index]...)
		w.buf = w.buf[index+1:]
		if len(line) > 0 && w.event != nil {
			w.event(line)
		}
	}
	return len(p), nil
}

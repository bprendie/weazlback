package packagecapsule

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type ExecRunner struct {
	Context context.Context
	Quiet   bool
}

func (r ExecRunner) context() context.Context {
	if r.Context != nil {
		return r.Context
	}
	return context.Background()
}

func (r ExecRunner) Output(name string, args ...string) (string, error) {
	command := exec.CommandContext(r.context(), name, args...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s: %w: %s", name, err, stderr.String())
	}
	return stdout.String(), nil
}

func (r ExecRunner) Run(name string, args ...string) error {
	command := exec.CommandContext(r.context(), name, args...)
	return r.run(command, name)
}

func (r ExecRunner) RunDir(dir, name string, args ...string) error {
	command := exec.CommandContext(r.context(), name, args...)
	command.Dir = dir
	return r.run(command, name)
}

func (r ExecRunner) run(command *exec.Cmd, name string) error {
	if !r.Quiet {
		command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
		return command.Run()
	}
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(output.String())
		if len(message) > 2048 {
			message = message[len(message)-2048:]
		}
		if message != "" {
			return fmt.Errorf("%s: %w: %s", name, err, message)
		}
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

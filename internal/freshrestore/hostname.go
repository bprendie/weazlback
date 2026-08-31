package freshrestore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

var hostnamePattern = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)

func ResolveHostname(mode, original string) (string, error) {
	current, _ := os.Hostname()
	switch strings.TrimSpace(mode) {
	case "", "original":
		mode = original
	case "current":
		mode = current
	}
	if len(mode) > 253 || !hostnamePattern.MatchString(mode) {
		return "", errors.New("hostname must be a valid DNS-style hostname")
	}
	return mode, nil
}

func ApplyHostname(hostname string) error {
	current, _ := os.Hostname()
	if hostname == current {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command("sudo", "-n", executable, "--internal-set-hostname", hostname)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func SetHostnamePrivileged(hostname string) error {
	if os.Geteuid() != 0 {
		return errors.New("hostname helper must run as root")
	}
	if _, err := ResolveHostname(hostname, hostname); err != nil {
		return err
	}
	if err := writeHostnameFiles(hostname, "/etc/hostname", "/etc/hosts"); err != nil {
		return err
	}
	return unix.Sethostname([]byte(hostname))
}

func writeHostnameFiles(hostname, hostnamePath, hostsPath string) error {
	if err := atomicSystemFile(hostnamePath, []byte(hostname+"\n")); err != nil {
		return err
	}
	b, err := os.ReadFile(hostsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	found := false
	for i, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "127.0.1.1" {
			lines[i], found = "127.0.1.1\t"+hostname, true
			break
		}
	}
	if !found {
		lines = append(lines, "127.0.1.1\t"+hostname)
	}
	return atomicSystemFile(hostsPath, []byte(strings.Join(lines, "\n")+"\n"))
}

func atomicSystemFile(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".weazlback-hostname-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o644); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return os.Rename(name, path)
}

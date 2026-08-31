package nuke

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/bprendie/weazlback/internal/config"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func DeleteRepository(destination config.Destination, privateKey []byte, removeLocalDirectory bool) error {
	if destination.Kind == "ssh" {
		return deleteSFTP(destination, privateKey)
	}
	return deleteLocal(destination.Repository, removeLocalDirectory)
}

func deleteLocal(path string, removeDirectory bool) error {
	path = filepath.Clean(path)
	home, _ := os.UserHomeDir()
	if !filepath.IsAbs(path) || path == "/" || path == home || len(strings.Split(path, string(os.PathSeparator))) < 3 {
		return errors.New("refusing to delete an unsafe repository path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("configured repository is not a real directory")
	}
	if removeDirectory {
		return os.RemoveAll(path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func deleteSFTP(destination config.Destination, privateKey []byte) error {
	user, host, root, err := parseSFTP(destination.Repository)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		return err
	}
	hostKey, err := knownhosts.New(destination.SSHKnownHosts)
	if err != nil {
		return err
	}
	address := host
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "22")
	}
	connection, err := ssh.Dial("tcp", address, &ssh.ClientConfig{User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: hostKey})
	if err != nil {
		return err
	}
	defer connection.Close()
	client, err := sftp.NewClient(connection)
	if err != nil {
		return err
	}
	defer client.Close()
	entries, err := client.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := removeRemote(client, filepath.Join(root, entry.Name()), entry); err != nil {
			return err
		}
	}
	return nil
}

func removeRemote(client *sftp.Client, path string, info fs.FileInfo) error {
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		entries, err := client.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := removeRemote(client, filepath.Join(path, entry.Name()), entry); err != nil {
				return err
			}
		}
		return client.RemoveDirectory(path)
	}
	return client.Remove(path)
}

func parseSFTP(repository string) (string, string, string, error) {
	value := strings.TrimPrefix(repository, "sftp:")
	at := strings.IndexByte(value, '@')
	colon := strings.IndexByte(value, ':')
	if at <= 0 || colon <= at+1 || colon == len(value)-1 {
		return "", "", "", fmt.Errorf("unsupported SFTP repository locator")
	}
	return value[:at], value[at+1 : colon], value[colon+1:], nil
}

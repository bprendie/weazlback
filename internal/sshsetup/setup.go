package sshsetup

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Target struct {
	Host     string
	Port     int
	User     string
	Password string
}

type Result struct {
	PrivateKey    []byte
	AuthorizedKey string
	KnownHostLine string
	Fingerprint   string
	Repository    string
}

func Probe(ctx context.Context, host string, port int) (string, string, error) {
	if port == 0 {
		port = 22
	}
	var captured ssh.PublicKey
	config := &ssh.ClientConfig{User: "weazlback-probe", Timeout: 8 * time.Second,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error { captured = key; return nil }}
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return "", "", err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(8 * time.Second))
	_, _, _, _ = ssh.NewClientConn(connection, net.JoinHostPort(host, strconv.Itoa(port)), config)
	if captured == nil {
		return "", "", errors.New("server did not present an SSH host key")
	}
	address := host
	if port != 22 {
		address = fmt.Sprintf("[%s]:%d", host, port)
	}
	return ssh.FingerprintSHA256(captured), knownhosts.Line([]string{address}, captured), nil
}

func Bootstrap(ctx context.Context, target Target, expectedFingerprint, repositoryID string) (Result, error) {
	if target.Port == 0 {
		target.Port = 22
	}
	if target.Host == "" || target.User == "" || target.Password == "" {
		return Result{}, errors.New("hostname, username, and password are required")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Result{}, err
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		return Result{}, err
	}
	block, err := ssh.MarshalPrivateKey(private, "weazlback")
	if err != nil {
		return Result{}, err
	}
	privatePEM := pem.EncodeToMemory(block)
	authorized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	marker := "weazlback:" + safeID(repositoryID)
	forced := `restrict,command="/usr/lib/openssh/sftp-server -d /srv/weazlback/repositories" ` + authorized + ` ` + marker

	var hostLine string
	config := &ssh.ClientConfig{User: target.User, Auth: []ssh.AuthMethod{ssh.Password(target.Password)}, Timeout: 10 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			fingerprint := ssh.FingerprintSHA256(key)
			if fingerprint != expectedFingerprint {
				return fmt.Errorf("SSH host key changed: got %s", fingerprint)
			}
			hostLine = knownhosts.Line([]string{target.Host}, key)
			return nil
		}}
	client, err := ssh.Dial("tcp", net.JoinHostPort(target.Host, strconv.Itoa(target.Port)), config)
	if err != nil {
		return Result{}, err
	}
	defer client.Close()
	repositoryID = safeID(repositoryID)
	if repositoryID == "" {
		return Result{}, errors.New("invalid repository ID")
	}
	script := bootstrapScript(repositoryID, base64.StdEncoding.EncodeToString([]byte(forced+"\n")))
	session, err := client.NewSession()
	if err != nil {
		return Result{}, err
	}
	defer session.Close()
	session.Stdin = strings.NewReader(target.Password + "\n")
	var output bytes.Buffer
	session.Stdout, session.Stderr = &output, &output
	if err := session.Run("sudo -S -p '' sh -c " + shellQuote(script)); err != nil {
		return Result{}, fmt.Errorf("remote bootstrap: %w: %s", err, strings.TrimSpace(output.String()))
	}
	repository := fmt.Sprintf("sftp:weazlback@%s:/srv/weazlback/repositories/%s", target.Host, repositoryID)
	return Result{PrivateKey: privatePEM, AuthorizedKey: forced, KnownHostLine: hostLine,
		Fingerprint: expectedFingerprint, Repository: repository}, nil
}

func bootstrapScript(repositoryID, encodedKey string) string {
	marker := "weazlback:" + repositoryID
	return "set -eu; " +
		"id -u weazlback >/dev/null 2>&1 || useradd --system --create-home --home-dir /var/lib/weazlback --shell /bin/sh weazlback; " +
		"passwd -d weazlback >/dev/null; " +
		"install -d -m 700 -o weazlback -g weazlback /var/lib/weazlback/.ssh; " +
		"install -d -m 700 -o weazlback -g weazlback /srv/weazlback/repositories/" + repositoryID + "; " +
		"touch /var/lib/weazlback/.ssh/authorized_keys; " +
		"tmp=$(mktemp /var/lib/weazlback/.ssh/.authorized_keys.XXXXXX); " +
		"grep -vF " + shellQuote(marker) + " /var/lib/weazlback/.ssh/authorized_keys > \"$tmp\" || true; " +
		"printf %s " + shellQuote(encodedKey) + " | base64 -d >> \"$tmp\"; " +
		"mv \"$tmp\" /var/lib/weazlback/.ssh/authorized_keys; " +
		"chown weazlback:weazlback /var/lib/weazlback/.ssh/authorized_keys; chmod 600 /var/lib/weazlback/.ssh/authorized_keys"
}

func safeID(value string) string {
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return ""
		}
	}
	return value
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }

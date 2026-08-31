package restic

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

const UploadProbeBytes int64 = 100 << 20

type UploadProbe struct {
	Bytes   int64
	Elapsed time.Duration
	MiBPerS float64
}

func ProbeSFTPUpload(ctx context.Context, repo Repository) (UploadProbe, error) {
	return ProbeSFTPUploadWithProgress(ctx, repo, nil)
}

func ProbeSFTPUploadWithProgress(ctx context.Context, repo Repository, progress func(int64, int64, time.Duration)) (UploadProbe, error) {
	user, host, root, err := parseSFTPLocation(repo.Location)
	if err != nil {
		return UploadProbe{}, err
	}
	if len(repo.SSHKey) == 0 || repo.KnownHosts == "" {
		return UploadProbe{}, errors.New("bandwidth probe requires the vaulted SSH key and pinned host key")
	}
	signer, err := ssh.ParsePrivateKey(repo.SSHKey)
	if err != nil {
		return UploadProbe{}, fmt.Errorf("parse SSH private key: %w", err)
	}
	hostKey, err := knownhosts.New(repo.KnownHosts)
	if err != nil {
		return UploadProbe{}, err
	}
	address := host
	if _, _, splitErr := net.SplitHostPort(address); splitErr != nil {
		address = net.JoinHostPort(address, "22")
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return UploadProbe{}, err
	}
	connection, channels, requests, err := ssh.NewClientConn(raw, address, &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)}, HostKeyCallback: hostKey, Timeout: 10 * time.Second,
	})
	if err != nil {
		raw.Close()
		return UploadProbe{}, err
	}
	sshClient := ssh.NewClient(connection, channels, requests)
	defer sshClient.Close()
	probeDone := make(chan struct{})
	defer close(probeDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = sshClient.Close()
		case <-probeDone:
		}
	}()
	concurrentRequests := repo.Connections
	if concurrentRequests < 1 {
		concurrentRequests = 4
	}
	client, err := sftp.NewClient(sshClient, sftp.UseConcurrentWrites(true), sftp.MaxConcurrentRequestsPerFile(concurrentRequests))
	if err != nil {
		return UploadProbe{}, err
	}
	defer client.Close()
	nameBytes := make([]byte, 12)
	if _, err := rand.Read(nameBytes); err != nil {
		return UploadProbe{}, err
	}
	remote := path.Join(root, fmt.Sprintf(".weazlback-bandwidth-%x.tmp", nameBytes))
	file, err := client.OpenFile(remote, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return UploadProbe{}, err
	}
	defer client.Remove(remote)
	started := time.Now()
	reader := io.Reader(rand.Reader)
	if progress != nil {
		reader = &uploadProgressReader{reader: rand.Reader, total: UploadProbeBytes, started: started, progress: progress}
	}
	written, copyErr := io.Copy(file, io.LimitReader(reader, UploadProbeBytes))
	closeErr := file.Close()
	elapsed := time.Since(started)
	if copyErr != nil {
		return UploadProbe{}, copyErr
	}
	if closeErr != nil {
		return UploadProbe{}, closeErr
	}
	return UploadProbe{Bytes: written, Elapsed: elapsed, MiBPerS: float64(written) / (1 << 20) / elapsed.Seconds()}, nil
}

type uploadProgressReader struct {
	reader   io.Reader
	total    int64
	written  int64
	started  time.Time
	progress func(int64, int64, time.Duration)
}

func (w *uploadProgressReader) Read(value []byte) (int, error) {
	n, err := w.reader.Read(value)
	w.written += int64(n)
	w.progress(w.written, w.total, time.Since(w.started))
	return n, err
}

func parseSFTPLocation(repository string) (string, string, string, error) {
	if !strings.HasPrefix(repository, "sftp:") {
		return "", "", "", errors.New("bandwidth probe requires an SFTP repository")
	}
	value := strings.TrimPrefix(repository, "sftp:")
	at := strings.IndexByte(value, '@')
	colon := strings.IndexByte(value, ':')
	if at <= 0 || colon <= at+1 || colon == len(value)-1 {
		return "", "", "", errors.New("unsupported SFTP repository locator")
	}
	return value[:at], value[at+1 : colon], value[colon+1:], nil
}

func RecommendedUploadMiB(measured float64) int {
	if measured <= 0 {
		return 0
	}
	value := int(measured * 0.79)
	if value < 1 {
		return 1
	}
	return value
}

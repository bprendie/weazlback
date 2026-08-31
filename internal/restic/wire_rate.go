package restic

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type WireCounter func() (uint64, error)

func SFTPHost(repository string) string {
	if !strings.HasPrefix(repository, "sftp:") {
		return ""
	}
	authority := strings.TrimPrefix(repository, "sftp:")
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}
	if strings.HasPrefix(authority, "[") {
		if end := strings.Index(authority, "]"); end > 1 {
			return authority[1:end]
		}
		return ""
	}
	if colon := strings.Index(authority, ":"); colon >= 0 {
		authority = authority[:colon]
	}
	return strings.TrimSpace(authority)
}

func NewWireCounter(repository string) WireCounter {
	host := SFTPHost(repository)
	if host == "" {
		return nil
	}
	addresses, err := net.LookupIP(host)
	if err != nil || len(addresses) == 0 {
		return nil
	}
	address := addresses[0].String()
	for _, candidate := range addresses {
		if candidate.To4() != nil {
			address = candidate.String()
			break
		}
	}
	output, err := exec.Command("ip", "route", "get", address).Output()
	if err != nil {
		return nil
	}
	fields, device := strings.Fields(string(output)), ""
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "dev" {
			device = fields[i+1]
			break
		}
	}
	if device == "" || strings.ContainsAny(device, "/\x00") {
		return nil
	}
	path := filepath.Join("/sys/class/net", device, "statistics/tx_bytes")
	return func() (uint64, error) {
		value, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		return strconv.ParseUint(strings.TrimSpace(string(value)), 10, 64)
	}
}

func SampleWireRate(ctx context.Context, counter WireCounter, report func(float64)) {
	if counter == nil {
		return
	}
	previous, err := counter()
	if err != nil {
		return
	}
	previousAt := time.Now()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			current, readErr := counter()
			if readErr != nil {
				return
			}
			if current >= previous {
				report(float64(current-previous) / now.Sub(previousAt).Seconds())
			}
			previous, previousAt = current, now
		}
	}
}

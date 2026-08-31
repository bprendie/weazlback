package recovery

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func VerifyMediaDirectory(directory string) error {
	checksumPath := filepath.Join(directory, "SHA256SUMS")
	file, err := os.Open(checksumPath)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	verified := 0
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != 64 || fields[1] != filepath.Base(fields[1]) {
			return fmt.Errorf("invalid recovery-media checksum entry")
		}
		body, err := os.ReadFile(filepath.Join(directory, fields[1]))
		if err != nil {
			return fmt.Errorf("verify %s: %w", fields[1], err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != fields[0] {
			return fmt.Errorf("recovery-media checksum failed: %s", fields[1])
		}
		verified++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if verified < 4 {
		return fmt.Errorf("recovery media checksum set is incomplete")
	}
	return nil
}

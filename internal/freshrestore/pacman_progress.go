package freshrestore

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var pacmanItemPattern = regexp.MustCompile(`\(\s*([0-9]+)\s*/\s*([0-9]+)\s*\)\s+(?:installing|upgrading|reinstalling|downgrading)\s+([^\s]+)`)

func runPacmanProgress(ctx context.Context, command string, args []string, phase, lane string, items []string, callback func(RestoreProgress)) error {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	read, write, err := os.Pipe()
	if err != nil {
		return err
	}
	defer read.Close()
	cmd.Stdout, cmd.Stderr = write, write
	if err := cmd.Start(); err != nil {
		write.Close()
		return err
	}
	_ = write.Close()

	var output boundedOutput
	scanner := bufio.NewScanner(io.TeeReader(read, &output))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	scanner.Split(splitPacmanRecords)
	for scanner.Scan() {
		completed, total, current, ok := parsePacmanItem(scanner.Text())
		if !ok {
			continue
		}
		if len(items) > 0 && total < len(items) {
			total = len(items)
		}
		emitProgress(callback, RestoreProgress{Phase: phase, Lane: lane, Current: current, Completed: completed, Total: total})
	}
	waitErr := cmd.Wait()
	if scanErr := scanner.Err(); scanErr != nil && waitErr == nil {
		return scanErr
	}
	if waitErr != nil {
		return fmt.Errorf("%w: %s", waitErr, strings.TrimSpace(output.String()))
	}
	return nil
}

func parsePacmanItem(line string) (completed, total int, current string, ok bool) {
	plain := stripTerminalControl(line)
	match := pacmanItemPattern.FindStringSubmatch(plain)
	if len(match) != 4 {
		return 0, 0, "", false
	}
	completed, firstErr := strconv.Atoi(match[1])
	total, secondErr := strconv.Atoi(match[2])
	return completed, total, match[3], firstErr == nil && secondErr == nil && total > 0
}

func splitPacmanRecords(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexAny(data, "\r\n"); index >= 0 {
		advance = index + 1
		for advance < len(data) && (data[advance] == '\r' || data[advance] == '\n') {
			advance++
		}
		return advance, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

func stripTerminalControl(value string) string {
	var out strings.Builder
	escape := 0
	for _, r := range value {
		if r == '\x1b' {
			escape = 1
			continue
		}
		if escape == 1 {
			if r == '[' {
				escape = 2
			} else {
				escape = 0
			}
			continue
		}
		if escape == 2 {
			if r >= '@' && r <= '~' {
				escape = 0
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(value []byte) (int, error) {
	const limit = 64 * 1024
	b.data = append(b.data, value...)
	if len(b.data) > limit {
		b.data = append([]byte(nil), b.data[len(b.data)-limit:]...)
	}
	return len(value), nil
}

func (b *boundedOutput) String() string { return string(b.data) }

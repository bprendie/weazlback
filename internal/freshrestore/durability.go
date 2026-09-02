package freshrestore

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

func syncPathFilesystems(paths []string) error {
	seen := map[uint64]bool{}
	for _, path := range paths {
		probe := path
		for {
			if _, err := os.Stat(probe); err == nil {
				break
			}
			parent := filepath.Dir(probe)
			if parent == probe {
				return fmt.Errorf("cannot resolve filesystem for %s", path)
			}
			probe = parent
		}
		file, err := os.Open(probe)
		if err != nil {
			return err
		}
		var stat syscall.Stat_t
		err = syscall.Fstat(int(file.Fd()), &stat)
		if err == nil && !seen[uint64(stat.Dev)] {
			err = unix.Syncfs(int(file.Fd()))
			seen[uint64(stat.Dev)] = err == nil
		}
		_ = file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

package benchmark

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

type fileProof struct {
	Path     string
	Mode     os.FileMode
	Size     int64
	Hash     [sha256.Size]byte
	Link     string
	Hardlink string
	ModTime  int64
	UID      uint32
	GID      uint32
	Xattrs   [sha256.Size]byte
}

func proveTree(root string) ([]fileProof, error) {
	var proofs []fileProof
	inodes := map[uint64]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		proof := fileProof{Path: rel, Mode: info.Mode(), Size: info.Size(), ModTime: info.ModTime().UnixNano()}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			proof.UID, proof.GID = stat.Uid, stat.Gid
			if stat.Nlink > 1 && info.Mode().IsRegular() {
				if first, exists := inodes[stat.Ino]; exists {
					proof.Hardlink = first
				} else {
					inodes[stat.Ino], proof.Hardlink = rel, rel
				}
			}
		}
		if info.Mode()&os.ModeSymlink != 0 {
			proof.Link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		} else if info.Mode().IsRegular() {
			file, openErr := os.Open(path)
			if openErr != nil {
				return openErr
			}
			hash := sha256.New()
			_, copyErr := io.Copy(hash, file)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			copy(proof.Hash[:], hash.Sum(nil))
		}
		proof.Xattrs = xattrDigest(path)
		proofs = append(proofs, proof)
		return nil
	})
	sort.Slice(proofs, func(i, j int) bool { return proofs[i].Path < proofs[j].Path })
	return proofs, err
}

func xattrDigest(path string) [sha256.Size]byte {
	var digest [sha256.Size]byte
	size, err := unix.Llistxattr(path, nil)
	if err != nil || size == 0 {
		return digest
	}
	buffer := make([]byte, size)
	read, err := unix.Llistxattr(path, buffer)
	if err != nil {
		return digest
	}
	names := strings.Split(strings.TrimRight(string(buffer[:read]), "\x00"), "\x00")
	sort.Strings(names)
	hash := sha256.New()
	for _, name := range names {
		valueSize, getErr := unix.Lgetxattr(path, name, nil)
		if getErr != nil {
			continue
		}
		value := make([]byte, valueSize)
		if _, getErr = unix.Lgetxattr(path, name, value); getErr != nil {
			continue
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(value)
	}
	copy(digest[:], hash.Sum(nil))
	return digest
}

func verifyTrees(source, restored string) error {
	want, err := proveTree(source)
	if err != nil {
		return err
	}
	got, err := proveTree(restored)
	if err != nil {
		return err
	}
	if len(got) != len(want) {
		return fmt.Errorf("restored file count %d, want %d", len(got), len(want))
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("restored file mismatch at %s", want[i].Path)
		}
	}
	return nil
}

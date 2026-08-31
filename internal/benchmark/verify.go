package benchmark

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

type fileProof struct {
	Path string
	Mode os.FileMode
	Size int64
	Hash [sha256.Size]byte
}

func proveTree(root string) ([]fileProof, error) {
	var proofs []fileProof
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
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
		file, err := os.Open(path)
		if err != nil {
			return err
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
		var digest [sha256.Size]byte
		copy(digest[:], hash.Sum(nil))
		proofs = append(proofs, fileProof{Path: rel, Mode: info.Mode().Perm(), Size: info.Size(), Hash: digest})
		return nil
	})
	sort.Slice(proofs, func(i, j int) bool { return proofs[i].Path < proofs[j].Path })
	return proofs, err
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

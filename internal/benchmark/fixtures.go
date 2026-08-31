package benchmark

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var fixtureNames = []string{"tiny", "mixed", "raw", "qcow2"}

func createFixture(name, root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	switch name {
	case "tiny":
		return createTiny(root)
	case "mixed":
		return createMixed(root)
	case "raw":
		return createRaw(root)
	case "qcow2":
		return createQCOW2(root)
	default:
		return fmt.Errorf("unknown fixture %q", name)
	}
}

func createTiny(root string) error {
	for i := range 5000 {
		dir := filepath.Join(root, fmt.Sprintf("config-%02d", i%32))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		data := []byte(fmt.Sprintf("fixture=%d\nvalue=%x\n", i, sha256.Sum256([]byte(fmt.Sprint(i)))))
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("item-%05d.conf", i)), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func createMixed(root string) error {
	sizes := []int64{4 << 10, 64 << 10, 1 << 20, 8 << 20}
	for i := range 96 {
		size := sizes[i%len(sizes)]
		path := filepath.Join(root, fmt.Sprintf("group-%02d", i%8), fmt.Sprintf("blob-%03d", i))
		if err := writePattern(path, size, byte(i)); err != nil {
			return err
		}
	}
	return nil
}

func createRaw(root string) error {
	path := filepath.Join(root, "sparse.raw")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := file.Truncate(4 << 30); err != nil {
		return err
	}
	for _, offset := range []int64{0, 1 << 30, 2 << 30, 3 << 30} {
		if err := randomWriteAt(file, offset, 64<<20); err != nil {
			return err
		}
	}
	return file.Sync()
}

func createQCOW2(root string) error {
	path, err := exec.LookPath("qemu-img")
	if err != nil {
		return errorsForTool("qemu-img")
	}
	disk := filepath.Join(root, "disk.qcow2")
	raw := filepath.Join(root, "seed.raw")
	if err := writePattern(raw, 256<<20, 0); err != nil {
		return err
	}
	defer os.Remove(raw)
	if out, err := exec.Command(path, "convert", "-f", "raw", "-O", "qcow2", raw, disk).CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command(path, "resize", disk, "4G").CombinedOutput(); err != nil {
		return fmt.Errorf("qemu-img resize: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func mutateFixture(name, root string) error {
	switch name {
	case "tiny":
		return os.WriteFile(filepath.Join(root, "config-00", "item-00000.conf"), []byte("changed=true\n"), 0o600)
	case "mixed":
		return writePattern(filepath.Join(root, "group-00", "blob-000"), 4<<10, 0xff)
	case "raw":
		file, err := os.OpenFile(filepath.Join(root, "sparse.raw"), os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		return randomWriteAt(file, (2<<30)+(128<<20), 8<<20)
	case "qcow2":
		randomPath := filepath.Join(root, "mutation.bin")
		if err := writePattern(randomPath, 8<<20, 0); err != nil {
			return err
		}
		defer os.Remove(randomPath)
		return qemuWriteFromFile(filepath.Join(root, "disk.qcow2"), randomPath)
	default:
		return fmt.Errorf("unknown fixture %q", name)
	}
}

func qemuWrite(disk, operation string) error {
	path, err := exec.LookPath("qemu-io")
	if err != nil {
		return errorsForTool("qemu-io")
	}
	out, err := exec.Command(path, "-f", "qcow2", "-c", operation, disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-io: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writePattern(path string, size int64, seed byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_ = seed
	_, err = io.CopyN(file, rand.Reader, size)
	return err
}

func randomWriteAt(file *os.File, offset, size int64) error {
	section := io.NewOffsetWriter(file, offset)
	_, err := io.CopyN(section, rand.Reader, size)
	return err
}

func qemuWriteFromFile(disk, source string) error {
	path, err := exec.LookPath("qemu-io")
	if err != nil {
		return errorsForTool("qemu-io")
	}
	operation := "write -s " + source + " 128M 8M"
	out, err := exec.Command(path, "-f", "qcow2", "-c", operation, disk).CombinedOutput()
	if err != nil {
		return fmt.Errorf("qemu-io: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func errorsForTool(name string) error { return fmt.Errorf("required tool %s is not installed", name) }

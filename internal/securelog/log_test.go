package securelog

import (
	"bytes"
	"os"
	"testing"
)

type xor byte

func (x xor) Encrypt(value []byte) ([]byte, error) {
	result := append([]byte(nil), value...)
	for i := range result {
		result[i] ^= byte(x)
	}
	return result, nil
}

func TestLogsAreEncryptedPrivateAndRetained(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	for i := 0; i < 55; i++ {
		path, err := Write(xor(0x42), "restore", ID()+string(rune('a'+i)), []byte("/home/bob/private-key.txt"))
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(path)
		if bytes.Contains(raw, []byte("private-key")) {
			t.Fatal("encrypted restore log leaked a filename")
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%o", info.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(os.Getenv("WEAZLBACK_HOME") + "/logs")
	if len(entries) != 50 {
		t.Fatalf("retained=%d", len(entries))
	}
}

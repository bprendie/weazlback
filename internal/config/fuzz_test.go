package config

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzConfigLoad(f *testing.F) {
	f.Add([]byte(`{"schema_version":1,"profiles":[],"destinations":[]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = Load(path)
	})
}

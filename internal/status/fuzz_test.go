package status

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzStatusLoad(f *testing.F) {
	f.Add([]byte(`{"schema_version":2,"state":"healthy"}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{"progress":{"percent":1e309}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		path := filepath.Join(t.TempDir(), "status.json")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _ = (Store{Path: path}).Load()
	})
}

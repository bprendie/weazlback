package browserrepair

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func FuzzSecureRootCannotEscapeHome(f *testing.F) {
	for _, seed := range []string{"../outside", "../../etc", ".config/chromium", ".", "/tmp/outside"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		home := t.TempDir()
		candidate := value
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(home, candidate)
		}
		cleanHome, _ := filepath.Abs(home)
		cleanCandidate, _ := filepath.Abs(candidate)
		rel, _ := filepath.Rel(cleanHome, cleanCandidate)
		escaped := rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
		if escaped && secureRoot(Options{Home: home, UID: os.Getuid(), Lstat: os.Lstat}, candidate) == nil {
			t.Fatalf("accepted escape %q", value)
		}
	})
}

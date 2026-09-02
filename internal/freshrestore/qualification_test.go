package freshrestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryParsersRespectAvailableAndCgroup(t *testing.T) {
	dir := t.TempDir()
	mem := filepath.Join(dir, "meminfo")
	limit := filepath.Join(dir, "memory.max")
	if err := os.WriteFile(mem, []byte("MemTotal: 9000 kB\nMemAvailable: 8000 kB\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := memAvailable(mem); got != 8000<<10 {
		t.Fatalf("available=%d", got)
	}
	if err := os.WriteFile(limit, []byte("4194304\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cgroupLimit(limit); got != 4194304 {
		t.Fatalf("limit=%d", got)
	}
	if err := os.WriteFile(limit, []byte("max\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cgroupLimit(limit); got != 0 {
		t.Fatalf("unlimited=%d", got)
	}
}

func TestQualificationRejectsUnsafeBudgetAndTransport(t *testing.T) {
	q := QualifyTurbo(t.TempDir(), "tape", TurboPolicy{MemoryPercent: 90})
	if q.Eligible || len(q.HardFailures) < 2 {
		t.Fatalf("qualification=%+v", q)
	}
}

func TestQualificationSoftFindingRequiresBreakGlass(t *testing.T) {
	dir := t.TempDir()
	q := QualifyTurbo(dir, "local", TurboPolicy{MemoryPercent: 70})
	if q.TargetFilesystem != "btrfs" && q.Eligible {
		t.Fatalf("soft finding qualified without override: %+v", q)
	}
	q = QualifyTurbo(dir, "local", TurboPolicy{MemoryPercent: 70, BreakGlass: true})
	if len(q.HardFailures) == 0 && !q.Eligible {
		t.Fatalf("break glass did not accept soft findings: %+v", q)
	}
}

func TestQualificationRejectsLowSpace(t *testing.T) {
	q := Qualification{Eligible: true, FreeBytes: 1 << 30}
	requireRestoreSpace(&q, 2<<30)
	if q.Eligible || len(q.HardFailures) != 1 {
		t.Fatalf("space qualification=%+v", q)
	}
}

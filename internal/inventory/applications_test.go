package inventory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplicationManifestIsPrivateAndVersioned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "applications.json")
	m := ApplicationManifest{SchemaVersion: ApplicationSchemaVersion, CapturedAt: time.Now(), Hostname: "test",
		Packages: PackageInventory{OfficialInstalled: []InstalledPackage{{Name: "restic", Version: "1"}}}}
	if err := WriteApplications(path, m); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	var decoded ApplicationManifest
	b, _ := os.ReadFile(path)
	if json.Unmarshal(b, &decoded) != nil || decoded.SchemaVersion != ApplicationSchemaVersion {
		t.Fatal("manifest did not round trip")
	}
}

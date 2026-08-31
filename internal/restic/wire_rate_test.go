package restic

import "testing"

func TestSFTPHost(t *testing.T) {
	tests := map[string]string{
		"sftp:weazlback@backup.example.test:/srv/repo":     "backup.example.test",
		"sftp:user@[2001:db8::1]:/repo":                   "2001:db8::1",
		"/mnt/weazlback":                                  "",
	}
	for repository, want := range tests {
		if got := SFTPHost(repository); got != want {
			t.Errorf("SFTPHost(%q)=%q, want %q", repository, got, want)
		}
	}
}

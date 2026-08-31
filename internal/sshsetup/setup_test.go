package sshsetup

import (
	"strings"
	"testing"
)

func TestBootstrapScriptIsRestrictedAndUnlocksAccount(t *testing.T) {
	script := bootstrapScript("machine-1234", "a2V5Cg==")
	for _, expected := range []string{"passwd -d weazlback", "chmod 600", "/srv/weazlback/repositories/machine-1234",
		"grep -vF 'weazlback:machine-1234'", "mv \"$tmp\""} {
		if !strings.Contains(script, expected) {
			t.Errorf("script missing %q", expected)
		}
	}
	if strings.Contains(script, "base64 -d > /var/lib/weazlback/.ssh/authorized_keys") {
		t.Fatal("bootstrap truncates unrelated authorized keys")
	}
}

func TestSafeID(t *testing.T) {
	if safeID("workstation-1234") == "" {
		t.Fatal("safe ID rejected")
	}
	for _, invalid := range []string{"../root", "has space", "UPPER"} {
		if safeID(invalid) != "" {
			t.Errorf("unsafe ID %q accepted", invalid)
		}
	}
}

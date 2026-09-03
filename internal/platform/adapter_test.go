package platform

import (
	"path/filepath"
	"testing"
)

func TestClaimsAreSpecificAndNeverClaimWholeConfigOrHome(t *testing.T) {
	home := "/home/me"
	for _, identity := range []Identity{{ID: "omarchy", Family: "arch", PackageFamily: "pacman", Desktop: "omarchy-shell"}, {ID: "ubuntu", Family: "debian", Desktop: "gnome"}, {ID: "fedora", Family: "fedora", Desktop: "plasma"}} {
		claims := For(identity).CoreClaims(home)
		if len(claims) == 0 {
			t.Fatalf("%+v has no claims", identity)
		}
		for _, claim := range claims {
			if claim.Path == home || claim.Path == filepath.Join(home, ".config") {
				t.Fatalf("overbroad claim: %+v", claim)
			}
		}
	}
}

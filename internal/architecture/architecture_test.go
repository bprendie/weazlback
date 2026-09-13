package architecture

import "testing"

func TestArchitectureAliases(t *testing.T) {
	for _, pair := range [][2]string{{"arm64", "aarch64"}, {"amd64", "x86_64"}, {"any", "arm64"}} {
		if !Compatible(pair[0], pair[1]) {
			t.Fatal(pair)
		}
	}
	if Compatible("x86_64", "aarch64") || Compatible("", "aarch64") {
		t.Fatal("wrong/unknown architecture accepted")
	}
}

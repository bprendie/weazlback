package architecture

import (
	"debug/elf"
	"fmt"
	"runtime"
)

func Go(value string) string {
	switch value {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return value
}
func Package(value string) string {
	switch Go(value) {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return value
}
func Native() string { return Package(runtime.GOARCH) }
func Compatible(artifact, target string) bool {
	return artifact == "any" || (artifact != "" && Go(artifact) == Go(target))
}
func ValidateELF(path, arch string) error {
	f, err := elf.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	want := map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64}[Go(arch)]
	if want == elf.EM_NONE || f.Machine != want || f.Class != elf.ELFCLASS64 {
		return fmt.Errorf("%s: expected Linux %s ELF", path, arch)
	}
	return nil
}

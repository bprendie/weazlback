package apprestore

import (
	"context"
	"os/exec"
	"strings"
	"sync"

	"github.com/bprendie/weazlback/internal/inventory"
)

type SystemResolver struct{ Context context.Context }

type MapResolver map[string]string

func (r MapResolver) Available(name string, source Source) (string, bool) {
	value, ok := r[packageKey(name, source)]
	return value, ok
}

func ResolveManifest(ctx context.Context, manifest inventory.ApplicationManifest, workers int) MapResolver {
	if workers < 1 {
		workers = 4
	}
	jobs := make(chan Package)
	result := MapResolver{}
	var mutex sync.Mutex
	var group sync.WaitGroup
	resolver := SystemResolver{Context: ctx}
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for pkg := range jobs {
				if version, ok := resolver.Available(pkg.Name, pkg.Source); ok {
					mutex.Lock()
					result[packageKey(pkg.Name, pkg.Source)] = version
					mutex.Unlock()
				}
			}
		}()
	}
	for _, pkg := range desiredPackages(manifest) {
		jobs <- pkg
	}
	close(jobs)
	group.Wait()
	return result
}

func (r SystemResolver) Available(name string, source Source) (string, bool) {
	ctx := r.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var command *exec.Cmd
	switch source {
	case Official:
		command = exec.CommandContext(ctx, "pacman", "-Si", name)
	case AUR:
		helper := "paru"
		if _, err := exec.LookPath(helper); err != nil {
			helper = "yay"
		}
		command = exec.CommandContext(ctx, helper, "-Si", name)
	case Flatpak:
		command = exec.CommandContext(ctx, "flatpak", "remote-info", "flathub", name)
	default:
		return "", false
	}
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.SplitN(line, ":", 2)
		if len(fields) == 2 && strings.EqualFold(strings.TrimSpace(fields[0]), "version") {
			return strings.TrimSpace(fields[1]), true
		}
	}
	return "", true
}

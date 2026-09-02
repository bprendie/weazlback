package freshrestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bprendie/weazlback/internal/packagecapsule"
	"github.com/bprendie/weazlback/internal/restic"
)

func (r *Restore) reconcilePackageCapsule(ctx context.Context) (installed, fallback, failures []string) {
	if r.Plan.PackageCapsule == nil || len(r.Plan.PackageDelta.Local) == 0 {
		return nil, nil, nil
	}
	files, err := r.stagePackageCapsule(ctx)
	if err != nil {
		r.Plan.CapsuleFallbackReason = "Package Capsule extraction or verification failed"
		fallback = r.fallbackAllCapsulePackages()
		return nil, fallback, []string{"Package Capsule verification: " + err.Error()}
	}
	r.Plan.CapsuleArtifactFiles = files
	return r.installVerifiedCapsule(ctx, files)
}

func (r *Restore) installVerifiedCapsule(ctx context.Context, files map[string]string) (installed, fallback, failures []string) {
	items := r.Plan.PackageDelta.Local
	if len(items) == 0 {
		return nil, nil, nil
	}
	emitProgress(r.Options.Progress, RestoreProgress{Phase: "package capsule", Lane: "offline transaction", Current: items[0].Name, Total: len(items)})
	args := []string{"-n", "pacman", "-U", "--needed", "--noconfirm", "--"}
	for _, item := range items {
		path := files[item.Name]
		if path == "" {
			r.Plan.CapsuleFallbackReason = "Package Capsule artifact ledger was incomplete"
			fallback = r.fallbackAllCapsulePackages()
			return nil, fallback, []string{"Package Capsule ledger is missing the verified path for " + item.Name}
		}
		args = append(args, path)
	}
	var installErr error
	if r.Options.Progress != nil {
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Name)
		}
		installErr = runPacmanProgress(ctx, "sudo", args, "package capsule", "offline transaction", names, r.Options.Progress)
	} else {
		installErr = visible(ctx, "sudo", args...)
	}
	if installErr != nil {
		r.Plan.CapsuleFallbackReason = "Pacman rejected the coordinated local transaction"
		fallback = r.fallbackAllCapsulePackages()
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "package capsule", Lane: "online fallback", Current: "Pacman rejected coordinated local transaction", Failed: len(items), Total: len(items)})
		return nil, fallback, nil
	}
	for index, item := range items {
		installed = append(installed, item.Name)
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "package capsule", Lane: "offline transaction", Current: item.Name, Completed: index + 1, Total: len(items)})
	}
	return installed, nil, nil
}

func (r *Restore) stagePackageCapsule(ctx context.Context) (map[string]string, error) {
	target := r.PackageStageDir
	if target == "" {
		target = r.StageDir + "-packages"
	}
	_ = os.RemoveAll(target)
	if err := os.MkdirAll(target, 0o700); err != nil {
		return nil, err
	}
	root := filepath.Dir(r.Plan.PackageManifestPath)
	started := time.Now()
	err := r.service.RestoreWithProgress(ctx, r.Session.Repository, r.Plan.PackageSnapshot.ID, target, []string{root}, func(value restic.RestoreProgress) {
		total, completed := int(value.TotalFiles), int(value.FilesRestored)
		if value.MessageType == "summary" && total > 0 {
			completed = total
		}
		elapsed, rate := time.Since(started).Seconds(), 0.0
		if elapsed > 0 {
			rate = float64(value.BytesRestored) / elapsed
		}
		emitProgress(r.Options.Progress, RestoreProgress{Phase: "package capsule", Lane: "extract + verify", Current: "authenticated artifacts",
			Completed: completed, Total: total, BytesDone: value.BytesRestored, BytesTotal: value.TotalBytes, BytesPerSecond: rate})
	})
	if err != nil {
		return nil, err
	}
	stagedRoot := stagedPath(target, root)
	manifest, err := packagecapsule.Load(stagedRoot)
	if err != nil {
		return nil, err
	}
	if !manifest.CapturedAt.Equal(r.Plan.PackageCapsule.CapturedAt) || manifest.MachineID != r.Plan.PackageCapsule.MachineID {
		return nil, fmt.Errorf("restored Package Capsule does not match the planned manifest")
	}
	return packagecapsule.VerifyArtifacts(stagedRoot, manifest, packagecapsule.ExecRunner{Context: ctx, Quiet: true})
}

func (r *Restore) fallbackAllCapsulePackages() []string {
	var names []string
	for _, item := range r.Plan.PackageDelta.Local {
		names = append(names, item.Name)
		if item.Source == "official" {
			r.Plan.Official = appendUniqueStrings(r.Plan.Official, item.Name)
		} else {
			r.Plan.AUR = appendUniqueStrings(r.Plan.AUR, item.Name)
		}
	}
	return names
}

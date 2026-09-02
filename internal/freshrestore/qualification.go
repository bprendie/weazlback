package freshrestore

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const defaultTurboMemoryPercent = 70

type mountRecord struct{ mount, device, filesystem string }

func QualifyTurbo(targetHome, transport string, policy TurboPolicy) Qualification {
	return QualifyTurboSource(targetHome, "", transport, policy)
}

func QualifyTurboSource(targetHome, sourcePath, transport string, policy TurboPolicy) Qualification {
	if policy.MemoryPercent == 0 {
		policy.MemoryPercent = defaultTurboMemoryPercent
	}
	q := Qualification{SourceTransport: transport}
	if transport == "local" && sourcePath != "" {
		if source, err := resolveMount(sourcePath); err != nil {
			q.HardFailures = append(q.HardFailures, "cannot resolve local repository source mount")
		} else {
			q.SourceMount, q.SourceDevice, q.SourceFilesystem = source.mount, source.device, source.filesystem
			q.SourceReadAheadKiB = readAheadKiB(source.device)
		}
	}
	if policy.MemoryPercent < 10 || policy.MemoryPercent > 70 {
		q.HardFailures = append(q.HardFailures, "memory budget must be between 10% and 70%")
	}
	available, limit := memoryEnvelope()
	base := available
	if limit > 0 && limit < base {
		base = limit
	}
	q.Budget = ResourceBudget{MemoryAvailable: available, MemoryLimit: limit, MemoryBudget: base * uint64(policy.MemoryPercent) / 100}
	if q.Budget.MemoryBudget < 512<<20 {
		q.SoftFindings = append(q.SoftFindings, "less than 512 MiB available for Turbo queues")
	}
	mount, err := resolveMount(targetHome)
	if err != nil {
		q.HardFailures = append(q.HardFailures, err.Error())
	} else {
		q.TargetMount, q.TargetDevice, q.TargetFilesystem = mount.mount, mount.device, mount.filesystem
		q.TargetReadAheadKiB = readAheadKiB(mount.device)
		q.PreservesOwnership, q.PreservesXattrs, q.PreservesACLs, q.PreservesSparse = metadataCapabilities(mount.filesystem)
		var stat syscall.Statfs_t
		if err := syscall.Statfs(mount.mount, &stat); err != nil {
			q.HardFailures = append(q.HardFailures, "cannot determine target free space")
		} else {
			q.FreeBytes = stat.Bavail * uint64(stat.Bsize)
		}
		if mount.filesystem != "btrfs" {
			q.SoftFindings = append(q.SoftFindings, "Btrfs fast landing unavailable; Turbo must use Standard placement")
		}
		switch mount.filesystem {
		case "vfat", "exfat", "ntfs", "fuseblk":
			q.HardFailures = append(q.HardFailures, "target filesystem cannot preserve required Linux ownership and metadata")
		}
	}
	if transport != "local" && transport != "ssh" {
		q.HardFailures = append(q.HardFailures, "unsupported repository transport")
	}
	q.BreakGlassApplied = policy.BreakGlass && len(q.HardFailures) == 0 && len(q.SoftFindings) > 0
	q.Eligible = len(q.HardFailures) == 0 && (len(q.SoftFindings) == 0 || policy.BreakGlass)
	return q
}

func metadataCapabilities(filesystem string) (bool, bool, bool, bool) {
	switch filesystem {
	case "btrfs", "ext4", "xfs", "f2fs", "zfs":
		return true, true, true, true
	default:
		return false, false, false, false
	}
}

func readAheadKiB(device string) int {
	if device == "" || !strings.HasPrefix(device, "/dev/") {
		return 0
	}
	out, err := exec.Command("blockdev", "--getra", device).Output()
	if err != nil {
		return 0
	}
	sectors, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return sectors / 2
}

func requireRestoreSpace(q *Qualification, required uint64) {
	if required == 0 || q.FreeBytes == 0 {
		return
	}
	margin := required/10 + 512<<20
	if q.FreeBytes < required+margin {
		q.HardFailures = append(q.HardFailures, "insufficient target space including recovery safety margin")
		q.Eligible = false
	}
}

func memoryEnvelope() (uint64, uint64) {
	available := memAvailable("/proc/meminfo")
	limit := cgroupLimit("/sys/fs/cgroup/memory.max")
	current := cgroupLimit("/sys/fs/cgroup/memory.current")
	if limit > current && limit-current < available {
		available = limit - current
	}
	return available, limit
}

func memAvailable(path string) uint64 {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) >= 2 && fields[0] == "MemAvailable:" {
			value, _ := strconv.ParseUint(fields[1], 10, 64)
			return value << 10
		}
	}
	return 0
}

func cgroupLimit(path string) uint64 {
	data, err := os.ReadFile(path)
	if err != nil || strings.TrimSpace(string(data)) == "max" {
		return 0
	}
	value, _ := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	return value
}

func resolveMount(path string) (mountRecord, error) {
	clean, err := filepath.Abs(path)
	if err != nil {
		return mountRecord{}, err
	}
	for {
		if _, err := os.Stat(clean); err == nil {
			break
		}
		parent := filepath.Dir(clean)
		if parent == clean {
			return mountRecord{}, errors.New("target path has no existing ancestor")
		}
		clean = parent
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return mountRecord{}, err
	}
	best := mountRecord{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Split(line, " - ")
		if len(parts) != 2 {
			continue
		}
		left, right := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(left) < 5 || len(right) < 2 {
			continue
		}
		mount := strings.ReplaceAll(left[4], `\040`, " ")
		if (clean == mount || strings.HasPrefix(clean, mount+string(os.PathSeparator))) && len(mount) >= len(best.mount) {
			best = mountRecord{mount: mount, filesystem: right[0], device: strings.Split(right[1], "[")[0]}
		}
	}
	if best.mount == "" {
		return mountRecord{}, fmt.Errorf("target mount is ambiguous for %s", path)
	}
	return best, nil
}

package restic

import (
	"sort"
	"strings"
)

func SnapshotHealth(tags []string) string {
	for _, tag := range tags {
		switch tag {
		case "failed":
			return "failed"
		case "incomplete":
			return "incomplete"
		case "unverified":
			return "unverified"
		}
	}
	return "healthy"
}

type Identity struct {
	ID       string
	Name     string
	Hostname string
	Legacy   bool
	Points   int
}

func MachineID(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "machine:") {
			return strings.TrimPrefix(tag, "machine:")
		}
	}
	return ""
}

func Profile(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, "profile:") {
			return strings.TrimPrefix(tag, "profile:")
		}
	}
	return ""
}

func IdentityID(snapshot Snapshot) string {
	id := MachineID(snapshot.Tags)
	if id == "" {
		return "legacy:" + snapshot.Hostname
	}
	return id
}

func GroupIdentities(snapshots []Snapshot, currentID, currentName string) []Identity {
	snapshots = append([]Snapshot(nil), snapshots...)
	sort.SliceStable(snapshots, func(i, j int) bool { return snapshots[i].Time.After(snapshots[j].Time) })
	groups := map[string]*Identity{}
	for _, snapshot := range snapshots {
		id := IdentityID(snapshot)
		legacy := MachineID(snapshot.Tags) == ""
		group := groups[id]
		if group == nil {
			name := snapshot.Hostname
			if id == currentID && currentName != "" {
				name = currentName
			}
			group = &Identity{ID: id, Name: name, Hostname: snapshot.Hostname, Legacy: legacy}
			groups[id] = group
		}
		group.Points++
	}
	result := make([]Identity, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID == currentID {
			return true
		}
		if result[j].ID == currentID {
			return false
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func FilterIdentity(snapshots []Snapshot, id string) []Snapshot {
	if id == "" {
		return append([]Snapshot(nil), snapshots...)
	}
	result := make([]Snapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.HasPrefix(id, "legacy:") {
			if MachineID(snapshot.Tags) == "" && "legacy:"+snapshot.Hostname == id {
				result = append(result, snapshot)
			}
		} else if MachineID(snapshot.Tags) == id {
			result = append(result, snapshot)
		}
	}
	return result
}

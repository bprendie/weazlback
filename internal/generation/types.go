package generation

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/bprendie/weazlback/internal/restic"
)

const (
	TagPrefix    = "generation:"
	TagComplete  = "generation-complete"
	TagFailed    = "generation-failed"
	TagAbandoned = "generation-abandoned"
)

var RequiredProfiles = []string{"packages", "core", "home", "heavy"}

type Generation struct {
	ID        string                     `json:"id"`
	StartedAt time.Time                  `json:"started_at"`
	EndedAt   time.Time                  `json:"ended_at,omitempty"`
	MachineID string                     `json:"machine_id"`
	Members   map[string]restic.Snapshot `json:"members"`
	Complete  bool                       `json:"complete"`
	Failed    bool                       `json:"failed"`
	Abandoned bool                       `json:"abandoned"`
}

func NewID(now time.Time) (string, error) {
	random := make([]byte, 6)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return now.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(random), nil
}

func ID(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, TagPrefix) {
			return strings.TrimPrefix(tag, TagPrefix)
		}
	}
	return ""
}

func Has(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func Catalog(snapshots []restic.Snapshot) []Generation {
	groups := map[string]*Generation{}
	for _, snapshot := range snapshots {
		id := ID(snapshot.Tags)
		if id == "" {
			continue
		}
		g := groups[id]
		if g == nil {
			g = &Generation{ID: id, StartedAt: snapshot.Time, MachineID: restic.MachineID(snapshot.Tags), Members: map[string]restic.Snapshot{}}
			groups[id] = g
		}
		profile := restic.Profile(snapshot.Tags)
		if current, ok := g.Members[profile]; !ok || snapshot.Time.After(current.Time) {
			g.Members[profile] = snapshot
		}
		if snapshot.Time.Before(g.StartedAt) {
			g.StartedAt = snapshot.Time
		}
		if snapshot.Time.After(g.EndedAt) {
			g.EndedAt = snapshot.Time
		}
		g.Complete = g.Complete || Has(snapshot.Tags, TagComplete)
		g.Failed = g.Failed || Has(snapshot.Tags, TagFailed)
		g.Abandoned = g.Abandoned || Has(snapshot.Tags, TagAbandoned)
	}
	result := make([]Generation, 0, len(groups))
	for _, g := range groups {
		g.Complete = g.Complete && HasAll(*g, RequiredProfiles)
		result = append(result, *g)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EndedAt.After(result[j].EndedAt) })
	return result
}

func HasAll(g Generation, profiles []string) bool {
	for _, profile := range profiles {
		if _, ok := g.Members[profile]; !ok {
			return false
		}
	}
	return true
}

func LatestComplete(generations []Generation, machineID string) (Generation, bool) {
	for _, g := range generations {
		if g.Complete && !g.Failed && !g.Abandoned && (machineID == "" || g.MachineID == machineID) {
			return g, true
		}
	}
	return Generation{}, false
}

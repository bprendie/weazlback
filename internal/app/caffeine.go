package app

import (
	"encoding/json"
	"os/exec"
	"sync"
)

type caffeineLease struct {
	changed  bool
	fallback *exec.Cmd
	once     sync.Once
}

func acquireCaffeine() *caffeineLease {
	lease := &caffeineLease{}
	output, err := exec.Command("omarchy-shell", "idle", "status").Output()
	if err == nil {
		var state struct {
			StayAwake bool `json:"stayAwake"`
		}
		if json.Unmarshal(output, &state) == nil {
			if state.StayAwake {
				return lease
			}
			if exec.Command("omarchy-shell", "idle", "disable").Run() == nil {
				lease.changed = true
				return lease
			}
		}
	}
	command := exec.Command("systemd-inhibit", "--what=sleep", "--mode=block",
		"--who=Weazlback", "--why=Encrypted backup is active", "sleep", "infinity")
	if command.Start() == nil {
		lease.fallback = command
	}
	return lease
}

func (l *caffeineLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.changed {
			_ = exec.Command("omarchy-shell", "idle", "enable").Run()
		}
		if l.fallback != nil && l.fallback.Process != nil {
			_ = l.fallback.Process.Kill()
			_ = l.fallback.Wait()
		}
	})
}

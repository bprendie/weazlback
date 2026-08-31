package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestVaultAcceptsWeakPassphraseAndUnlocks(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := New()
	m.vaultInput.SetValue("x")
	created, _ := m.updateVault(tea.KeyMsg{Type: tea.KeyEnter})
	m = created.(Model)
	if m.vaultStage != "confirm" {
		t.Fatalf("stage=%q", m.vaultStage)
	}
	m.vaultInput.SetValue("x")
	unlocked, _ := m.updateVault(tea.KeyMsg{Type: tea.KeyEnter})
	m = unlocked.(Model)
	defer m.Close()
	if m.vaultStage != "" || !m.vault.Unlocked() {
		t.Fatal("vault did not unlock")
	}
}

func TestDestinationStartsWithTransportChoice(t *testing.T) {
	t.Setenv("WEAZLBACK_HOME", t.TempDir())
	m := New()
	next, _ := m.startDestination()
	m = next.(Model)
	if m.destinationStage != "choose" {
		t.Fatalf("stage=%q", m.destinationStage)
	}
	next, _ = m.updateDestination(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = next.(Model)
	if m.destinationStage != "local" {
		t.Fatalf("stage=%q", m.destinationStage)
	}
}

func TestRecoveryScreenExposesPrepareUSBImmediately(t *testing.T) {
	m := Model{styles: newStyles(), mode: modeRecovery}
	view := m.recoveryScreen()
	if !strings.Contains(view, "Prepare USB") {
		t.Fatalf("prepare USB action is hidden: %q", view)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.recoveryStage != "form" || !m.recoveryPrepare {
		t.Fatalf("enter did not start USB preparation: stage=%q prepare=%v", m.recoveryStage, m.recoveryPrepare)
	}
}

func TestDetachedRendererCannotBackpressureBackupProgress(t *testing.T) {
	events := make(chan tea.Msg, 1)
	sendLatestOperationEvent(events, operationProgressMsg{})
	sendLatestOperationEvent(events, operationProgressMsg{})
	sendLatestOperationEvent(events, operationDoneMsg{})
	message := <-events
	if _, ok := message.(operationDoneMsg); !ok {
		t.Fatalf("latest event is %T, want completion", message)
	}
}

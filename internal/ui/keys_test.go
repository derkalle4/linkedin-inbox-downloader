package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func drainActionBus() {
	for {
		select {
		case <-ActionBus:
		default:
			return
		}
	}
}

func TestBackupQDoesNotQuit(t *testing.T) {
	drainActionBus()
	m := New(false, "/tmp")
	m.Screen = ScreenBackup
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := next.(Model)
	if got.Quit {
		t.Fatal("q during backup should return to the inbox, not quit")
	}
	if got.BackupCurrent != "Cancelling…" {
		t.Fatalf("status=%q", got.BackupCurrent)
	}
	if cmd == nil {
		t.Fatal("expected a cancel command")
	}
	cmd()
	select {
	case msg := <-ActionBus:
		if _, ok := msg.(BackupCancelMsg); !ok {
			t.Fatalf("got %T, want BackupCancelMsg", msg)
		}
	default:
		t.Fatal("expected BackupCancelMsg on ActionBus")
	}
}

func TestBackupCtrlCStillQuits(t *testing.T) {
	m := New(false, "/tmp")
	m.Screen = ScreenBackup
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	got := next.(Model)
	if !got.Quit {
		t.Fatal("ctrl+c during backup should still quit")
	}
}

func TestInboxQStillQuits(t *testing.T) {
	m := New(false, "/tmp")
	m.Screen = ScreenInbox
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	got := next.(Model)
	if !got.Quit {
		t.Fatal("q on the inbox should still quit")
	}
}

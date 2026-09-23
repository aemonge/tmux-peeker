package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
)

// modelWithSelectedPane builds a picker with "work" expanded into window 1,
// itself expanded, with pane 1 selected.
func modelWithSelectedPane() Model {
	m := NewModel()
	m.sessions = []tmux.Session{{Name: "work", WindowCount: 1}}
	m.tree.setSessionExpanded("work", true)
	m.tree.windowsCache["work"] = []tmux.Window{{Index: 1, Name: "editor"}}
	m.tree.setWindowExpanded("work", 1, true)
	m.tree.panesCache[paneCacheKey{session: "work", window: 1}] = []tmux.Pane{{Index: 0}, {Index: 1}}
	m.applyFilter()
	m.cursor = m.findItemIndex(itemPane, "work", 1, 1)
	return m
}

// recordingRunner records Run calls so kill dispatch can be asserted.
type recordingRunner struct{ runs []string }

func (r *recordingRunner) Output(name string, args ...string) ([]byte, error) {
	return []byte("stub\n"), nil
}

func (r *recordingRunner) Run(name string, args ...string) error {
	r.runs = append(r.runs, name+" "+strings.Join(args, " "))
	return nil
}

func TestKillBindingOpensConfirmForEveryKind(t *testing.T) {
	t.Run("session row", func(t *testing.T) {
		m := modelWithSelectedWindow()
		m.cursor = 0
		m = updateModel(t, m, runeKey("x"))
		if m.mode != modeConfirmKill {
			t.Fatalf("session kill mode = %v, want modeConfirmKill", m.mode)
		}
		if got := m.confirmKillMod.target.label(); got != "source" {
			t.Errorf("session kill target = %q, want source", got)
		}
	})

	t.Run("window row", func(t *testing.T) {
		m := modelWithSelectedWindow()
		m = updateModel(t, m, runeKey("x"))
		if m.mode != modeConfirmKill {
			t.Fatalf("window kill mode = %v, want modeConfirmKill", m.mode)
		}
		if got := m.confirmKillMod.target.label(); got != "source:1" {
			t.Errorf("window kill target = %q, want source:1", got)
		}
	})

	t.Run("pane row", func(t *testing.T) {
		m := modelWithSelectedPane()
		m = updateModel(t, m, runeKey("x"))
		if m.mode != modeConfirmKill {
			t.Fatalf("pane kill mode = %v, want modeConfirmKill", m.mode)
		}
		if got := m.confirmKillMod.target.label(); got != "work:1.1" {
			t.Errorf("pane kill target = %q, want work:1.1", got)
		}
	})
}

func TestConfirmKillDispatchesWindowKill(t *testing.T) {
	recorder := &recordingRunner{}
	tmux.SetRunner(recorder)
	defer tmux.SetRunner(execTmuxRunner{})

	m := modelWithSelectedWindow()
	m.tree.expandedWindow["source"] = map[int]bool{1: true}
	m.tree.panesCache[paneCacheKey{session: "source", window: 1}] = []tmux.Pane{{Index: 0}}
	m = updateModel(t, m, runeKey("x"))

	next, cmd := m.Update(runeKey("y"))
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	msg := cmd()
	killed, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("confirm produced %T, want killedMsg", msg)
	}
	if killed.err != nil || killed.cancelled {
		t.Fatalf("killedMsg = %+v, want confirmed without error", killed)
	}
	want := "tmux kill-window -t source:1"
	if len(recorder.runs) != 1 || recorder.runs[0] != want {
		t.Errorf("run calls = %q, want [%q]", recorder.runs, want)
	}

	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() model type = %T, want ui.Model", next)
	}
	next, cmd = got.Update(killed)
	got = next.(Model)
	if got.mode != modeList {
		t.Fatalf("after killedMsg mode = %v, want modeList", got.mode)
	}
	if _, cached := got.tree.windowsCache["source"]; cached {
		t.Error("killed window's session kept stale windows cache")
	}
	if _, cached := got.tree.expandedWindow["source"]; cached {
		t.Error("killed window's session kept stale window expansions")
	}
	if _, cached := got.tree.panesCache[paneCacheKey{session: "source", window: 1}]; cached {
		t.Error("killed window's session kept stale panes cache")
	}
	sawWindows, sawSessions := false, false
	for _, c := range batchCmds(t, cmd) {
		switch c.(type) {
		case windowsLoadedMsg:
			sawWindows = true
		case sessionsLoadedMsg:
			sawSessions = true
		}
	}
	if !sawWindows || !sawSessions {
		t.Errorf("window kill refreshed windows=%v sessions=%v, want both", sawWindows, sawSessions)
	}
}

func TestConfirmKillDispatchesPaneKill(t *testing.T) {
	recorder := &recordingRunner{}
	tmux.SetRunner(recorder)
	defer tmux.SetRunner(execTmuxRunner{})

	m := modelWithSelectedPane()
	m = updateModel(t, m, runeKey("x"))

	next, cmd := m.Update(runeKey("y"))
	msg := cmd()
	killed, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("confirm produced %T, want killedMsg", msg)
	}
	want := "tmux kill-pane -t work:1.1"
	if len(recorder.runs) != 1 || recorder.runs[0] != want {
		t.Errorf("run calls = %q, want [%q]", recorder.runs, want)
	}

	got := next.(Model)
	next, cmd = got.Update(killed)
	got = next.(Model)
	if got.mode != modeList {
		t.Fatalf("after killedMsg mode = %v, want modeList", got.mode)
	}
	if _, cached := got.tree.panesCache[paneCacheKey{session: "work", window: 1}]; cached {
		t.Error("killed pane's window kept stale panes cache")
	}
	sawPanes, sawWindows := false, false
	for _, c := range batchCmds(t, cmd) {
		switch c.(type) {
		case panesLoadedMsg:
			sawPanes = true
		case windowsLoadedMsg:
			sawWindows = true
		}
	}
	if !sawPanes || !sawWindows {
		t.Errorf("pane kill refreshed panes=%v windows=%v, want both", sawPanes, sawWindows)
	}
}

func TestCancelKillReturnsToListWithoutCommands(t *testing.T) {
	recorder := &recordingRunner{}
	tmux.SetRunner(recorder)
	defer tmux.SetRunner(execTmuxRunner{})

	m := modelWithSelectedPane()
	m = updateModel(t, m, runeKey("x"))

	next, cmd := m.Update(runeKey("n"))
	if cmd == nil {
		t.Fatal("cancel returned no command")
	}
	msg := cmd()
	killed, ok := msg.(killedMsg)
	if !ok {
		t.Fatalf("cancel produced %T, want killedMsg", msg)
	}
	if !killed.cancelled {
		t.Fatalf("cancel killedMsg = %+v, want cancelled", killed)
	}
	if len(recorder.runs) != 0 {
		t.Errorf("cancel executed tmux: %q", recorder.runs)
	}

	got := next.(Model)
	next, cmd = got.Update(killed)
	got = next.(Model)
	if got.mode != modeList {
		t.Fatalf("after cancelled kill mode = %v, want modeList", got.mode)
	}
	if cmd != nil {
		t.Errorf("cancelled kill returned refresh command %T", cmd())
	}
	if _, cached := got.tree.panesCache[paneCacheKey{session: "work", window: 1}]; !cached {
		t.Error("cancelled kill invalidated the panes cache")
	}
}

func TestKilledSessionRefreshesSessionList(t *testing.T) {
	m := modelWithSelectedWindow()
	next, cmd := m.Update(killedMsg{target: killTarget{kind: itemSession, session: "source"}})
	got := next.(Model)
	if got.mode != modeList {
		t.Fatalf("after session killedMsg mode = %v, want modeList", got.mode)
	}
	if _, ok := cmd().(sessionsLoadedMsg); !ok {
		t.Fatalf("session kill produced %T, want sessionsLoadedMsg", cmd())
	}
}

func TestKilledSessionErrorSurfacesToPicker(t *testing.T) {
	m := modelWithSelectedWindow()
	next, _ := m.Update(killedMsg{
		target: killTarget{kind: itemSession, session: "gone"},
		err:    errKillFailedForTest,
	})
	got := next.(Model)
	if got.mode != modeList {
		t.Fatalf("after failed killedMsg mode = %v, want modeList", got.mode)
	}
	if got.err != errKillFailedForTest {
		t.Errorf("model error = %v, want the kill failure", got.err)
	}
}

func TestWindowsLoadErrorFallsBackToSessionsRefresh(t *testing.T) {
	m := modelWithSelectedPane()
	next, cmd := m.Update(windowsLoadedMsg{sessionName: "work", err: errKillFailedForTest})
	got := next.(Model)
	if cmd == nil {
		t.Fatal("failed windows load returned no command")
	}
	if _, ok := cmd().(sessionsLoadedMsg); !ok {
		t.Fatalf("failed windows load produced %T, want sessionsLoadedMsg", cmd())
	}
	if got.mode != modeList {
		t.Fatalf("after failed windows load mode = %v, want modeList", got.mode)
	}
}

var errKillFailedForTest = errors.New("kill failed")

// batchCmds invokes cmd and returns the messages it produced, flattening a
// tea.BatchMsg into its members.
func batchCmds(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		msgs := make([]tea.Msg, 0, len(batch))
		for _, c := range batch {
			msgs = append(msgs, c())
		}
		return msgs
	}
	return []tea.Msg{msg}
}

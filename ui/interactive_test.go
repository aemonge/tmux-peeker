package ui

import (
	"testing"

	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
)

func interactiveTestModel() Model {
	m := NewModel()
	m.sessions = []tmux.Session{{Name: "work"}}
	m.tree.setSessionExpanded("work", true)
	m.tree.windowsCache["work"] = []tmux.Window{
		{Index: 0, Name: "editor", ActiveCommand: "nvim"},
		{Index: 1, Name: "server", ActiveCommand: "go run"},
		{Index: 2, Name: "logs", ActiveCommand: "tail"},
	}
	m.applyFilter()
	m.width = 100
	m.height = 40
	return m
}

func enterInteractive(t *testing.T, m Model) Model {
	t.Helper()
	next := updateModel(t, m, runeKey("i"))
	if next.mode != modeInteractive {
		t.Fatalf("mode = %v, want modeInteractive", next.mode)
	}
	return next
}

func TestInteractiveEntryResolvesSessionFromEachRowKind(t *testing.T) {
	t.Run("session row", func(t *testing.T) {
		m := enterInteractive(t, interactiveTestModel())
		if m.interactiveMod.session != "work" {
			t.Fatalf("session = %q, want work", m.interactiveMod.session)
		}
		if !m.interactiveMod.loaded {
			t.Fatal("cached windows did not prime the interactive model")
		}
		if len(m.interactiveMod.windows) != 3 {
			t.Fatalf("windows = %d, want 3 from cache", len(m.interactiveMod.windows))
		}
	})

	t.Run("window row uses parent session", func(t *testing.T) {
		m := interactiveTestModel()
		m.cursor = 1 // first window row under "work"
		m = enterInteractive(t, m)
		if m.interactiveMod.session != "work" {
			t.Fatalf("session = %q, want work", m.interactiveMod.session)
		}
	})

	t.Run("pane row uses parent session", func(t *testing.T) {
		m := interactiveTestModel()
		m.tree.setWindowExpanded("work", 0, true)
		m.tree.panesCache[paneCacheKey{session: "work", window: 0}] = []tmux.Pane{{Index: 0}}
		m.applyFilter()
		m.cursor = 4 // first pane row
		m = enterInteractive(t, m)
		if m.interactiveMod.session != "work" {
			t.Fatalf("session = %q, want work", m.interactiveMod.session)
		}
	})
}

func TestInteractiveEntryIssuesWindowLoadForUncachedSession(t *testing.T) {
	m := NewModel()
	m.sessions = []tmux.Session{{Name: "fresh"}}
	m.applyFilter()
	m.width, m.height = 100, 40

	next, cmd := m.Update(runeKey("i"))
	got := next.(Model)
	if got.mode != modeInteractive {
		t.Fatalf("mode = %v, want modeInteractive", got.mode)
	}
	if got.interactiveMod.loaded {
		t.Fatal("uncached session marked loaded at entry")
	}
	if cmd == nil {
		t.Fatal("entry returned nil cmd, want loadWindows")
	}
}

func TestInteractiveExitKeysReturnToList(t *testing.T) {
	for _, key := range []string{"i", "esc", "q"} {
		t.Run("exit via "+key, func(t *testing.T) {
			m := enterInteractive(t, interactiveTestModel())
			m = updateModel(t, m, runeKey(key))
			if m.mode != modeList {
				t.Fatalf("mode after %q = %v, want modeList", key, m.mode)
			}
		})
	}
}

func TestInteractiveNavigationMovesCursorAndScrolls(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	// 12 windows on a 40-row screen overflow: 2 indicator rows reserved,
	// leaving 38 rows = 3 bands of header+content.
	m.interactiveMod.setWindows(tenPlusWindows(12), 40)

	if got := m.interactiveMod.cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0", got)
	}
	for range 11 {
		m = updateModel(t, m, runeKey("j"))
	}
	if got := m.interactiveMod.cursor; got != 11 {
		t.Fatalf("cursor = %d, want 11", got)
	}
	if got := m.interactiveMod.offset; got != 9 {
		t.Fatalf("offset = %d, want 9 (cursor visible in last band)", got)
	}

	m = updateModel(t, m, runeKey("g"))
	if m.interactiveMod.cursor != 0 || m.interactiveMod.offset != 0 {
		t.Fatalf("after g cursor/offset = %d/%d, want 0/0", m.interactiveMod.cursor, m.interactiveMod.offset)
	}
	m = updateModel(t, m, runeKey("G"))
	if m.interactiveMod.cursor != 11 || m.interactiveMod.offset != 9 {
		t.Fatalf("after G cursor/offset = %d/%d, want 11/9", m.interactiveMod.cursor, m.interactiveMod.offset)
	}
	m = updateModel(t, m, runeKey("k"))
	if m.interactiveMod.cursor != 10 || m.interactiveMod.offset != 9 {
		t.Fatalf("after k cursor/offset = %d/%d, want 10/9 (still visible)", m.interactiveMod.cursor, m.interactiveMod.offset)
	}
	for range 2 {
		m = updateModel(t, m, runeKey("k"))
	}
	if m.interactiveMod.cursor != 8 || m.interactiveMod.offset != 8 {
		t.Fatalf("after kk cursor/offset = %d/%d, want 8/8 (scrolled up)", m.interactiveMod.cursor, m.interactiveMod.offset)
	}
}

func TestInteractiveVisibleBandsRespectMinimumContentRows(t *testing.T) {
	tests := []struct {
		name        string
		windowCount int
		height      int
		want        int
	}{
		{"single window", 1, 40, 1},
		{"three windows fit", 3, 40, 3},
		{"four fit without indicators", 4, 44, 4},
		{"five overflow reserves indicators", 5, 44, 3},
		{"overflow minimum height", 9, 13, 1},
		{"tiny terminal still shows one", 3, 11, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := interactiveVisibleBands(tt.windowCount, tt.height); got != tt.want {
				t.Fatalf("interactiveVisibleBands(%d, %d) = %d, want %d", tt.windowCount, tt.height, got, tt.want)
			}
		})
	}
}

func TestInteractiveWindowLoadPreservesCursorByWindowIndex(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.interactiveMod.cursor = 2
	m.interactiveMod.offset = 0

	// Window 1 was killed outside; indexes shift down.
	m = updateModel(t, m, windowsLoadedMsg{
		sessionName: "work",
		windows: []tmux.Window{
			{Index: 0, Name: "editor"},
			{Index: 2, Name: "logs"},
		},
	})
	if m.interactiveMod.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (same window index 2)", m.interactiveMod.cursor)
	}
}

func TestInteractiveEnterAttachesToHighlightedWindow(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, runeKey("j")) // highlight window 1

	next, cmd := m.Update(runeKey("enter"))
	got := next.(Model)
	if got.attachTarget.session != "work" || got.attachTarget.window != 1 || got.attachTarget.pane != -1 {
		t.Fatalf("attachTarget = %+v, want work:1 pane -1", got.attachTarget)
	}
	if cmd == nil {
		t.Fatal("attach returned nil cmd, want tea.Quit")
	}
	if !isQuit(cmd()) {
		t.Fatal("attach cmd did not quit")
	}
}

func TestInteractiveCaptureMsgUpdatesVisibleBand(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, interactiveCaptureMsg{session: "work", window: 1, content: "hello"})
	if got := m.interactiveMod.captures[1]; got != "hello" {
		t.Fatalf("captures[1] = %q, want hello", got)
	}

	// Captures for other sessions or after exit are ignored.
	m.mode = modeList
	m = updateModel(t, m, interactiveCaptureMsg{session: "work", window: 2, content: "stale"})
	if _, exists := m.interactiveMod.captures[2]; exists {
		t.Fatal("capture stored while not in interactive mode")
	}
}

func TestInteractiveExitsWhenSessionDies(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "other"}}})
	if m.mode != modeList {
		t.Fatalf("mode after session death = %v, want modeList", m.mode)
	}
}

func TestInteractiveSessionSurvivesRefresh(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, sessionsLoadedMsg{sessions: []tmux.Session{{Name: "work"}, {Name: "other"}}})
	if m.mode != modeInteractive {
		t.Fatalf("mode after refresh = %v, want modeInteractive", m.mode)
	}
}

func TestInteractiveCaptureCmdsCoverVisibleBandsOnly(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.interactiveMod.setWindows(tenPlusWindows(12), 40)
	m.interactiveMod.cursor = 11
	m.interactiveMod.ensureCursorVisible(40)

	cmds := m.interactiveMod.captureCmds(40)
	if len(cmds) != 3 {
		t.Fatalf("capture cmds = %d, want 3 visible bands", len(cmds))
	}
}

func tenPlusWindows(count int) []tmux.Window {
	windows := make([]tmux.Window, count)
	for i := range windows {
		windows[i] = tmux.Window{Index: i, Name: "win"}
	}
	return windows
}

func isQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}

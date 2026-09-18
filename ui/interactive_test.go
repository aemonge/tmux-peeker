package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
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

func stripLines(rendered string) []string {
	lines := strings.Split(ansi.Strip(rendered), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

func TestRenderInteractiveEqualBands(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 40
	rendered := m.viewInteractive()

	lines := strings.Split(rendered, "\n")
	if len(lines) != 40 {
		t.Fatalf("rendered lines = %d, want 40", len(lines))
	}
	plain := stripLines(rendered)
	// 40 rows over 3 bands: 14 + 13 + 13, headers at band starts.
	for _, want := range []struct {
		line int
		text string
	}{
		{0, "▸ peeking work:0 editor — nvim"},
		{14, "▷ peeking work:1 server — go run"},
		{27, "▷ peeking work:2 logs — tail"},
	} {
		if got := plain[want.line]; !strings.HasPrefix(got, want.text) {
			t.Errorf("line %d = %q, want prefix %q", want.line, got, want.text)
		}
	}
	for _, filler := range []int{13, 26, 39} {
		if plain[filler] != "" {
			t.Errorf("content line %d = %q, want blank filler", filler, plain[filler])
		}
	}
}

func TestRenderInteractiveIndicatorsFrameOverflow(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 40
	m.interactiveMod.setWindows(tenPlusWindows(12), 40)

	rendered := m.viewInteractive()
	if len(strings.Split(rendered, "\n")) != 40 {
		t.Fatalf("rendered lines = %d, want 40", len(strings.Split(rendered, "\n")))
	}
	plain := stripLines(rendered)
	if got := plain[0]; got != "" {
		t.Errorf("top indicator at offset 0 = %q, want spacer", got)
	}
	if got := plain[39]; !strings.Contains(got, "↓ 9 windows below") {
		t.Errorf("bottom indicator = %q, want ↓ 9 windows below", got)
	}

	m.interactiveMod.cursor = 11
	m.interactiveMod.ensureCursorVisible(40)
	plain = stripLines(m.viewInteractive())
	if got := plain[0]; !strings.Contains(got, "↑ 9 windows above") {
		t.Errorf("top indicator = %q, want ↑ 9 windows above", got)
	}
	if got := plain[39]; got != "" {
		t.Errorf("bottom indicator at last = %q, want spacer", got)
	}
	if got := plain[1]; !strings.HasPrefix(got, "▷ peeking work:9 win") {
		t.Errorf("first band header = %q, want window 9", got)
	}
	if got := plain[27]; !strings.HasPrefix(got, "▸ peeking work:11 win") {
		t.Errorf("selected band header = %q, want window 11", got)
	}
}

func TestRenderInteractiveBottomCropsContent(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 36 // 3 bands of 12 rows each
	capture := make([]string, 20)
	for i := range capture {
		capture[i] = fmt.Sprintf("line-%02d", i)
	}
	m.interactiveMod.captures[1] = strings.Join(capture, "\n")

	plain := stripLines(m.viewInteractive())
	// Band 1 starts at row 12 (header), content rows 13..23 hold the tail of
	// the 20-line capture: lines 09..19.
	if got := plain[13]; got != "line-09" {
		t.Errorf("first content row = %q, want line-09", got)
	}
	if got := plain[23]; got != "line-19" {
		t.Errorf("last content row = %q, want line-19", got)
	}
}

func TestRenderInteractiveEveryLineMatchesWidth(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.width, m.height = 80, 40
	m.interactiveMod.setWindows(tenPlusWindows(12), 40)
	m.interactiveMod.cursor = 5
	m.interactiveMod.ensureCursorVisible(40)

	rendered := m.viewInteractive()
	for i, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != 80 {
			t.Errorf("line %d width = %d, want 80", i, got)
		}
	}
}

func TestInteractiveViewTooSmallFallsBackToError(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.width, m.height = 80, 10
	rendered := m.viewInteractive()
	if !strings.Contains(ansi.Strip(rendered), "Terminal too small") {
		t.Fatalf("small terminal fallback missing message:\n%s", rendered)
	}
	if len(strings.Split(rendered, "\n")) != 10 {
		t.Fatal("small terminal fallback does not fill exactly 10 rows")
	}
}

// stubTmuxRunner fakes tmux for ui-level tests; every Output call returns a
// single stub line.
type stubTmuxRunner struct{}

func (stubTmuxRunner) Output(name string, args ...string) ([]byte, error) {
	return []byte("stub\n"), nil
}

func (stubTmuxRunner) Run(name string, args ...string) error { return nil }

// execTmuxRunner mirrors tmux's real runner so tests restore it faithfully.
type execTmuxRunner struct{}

func (execTmuxRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

func (execTmuxRunner) Run(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func TestInteractiveTickRefreshesWindowsAndVisibleCaptures(t *testing.T) {
	tmux.SetRunner(stubTmuxRunner{})
	defer tmux.SetRunner(execTmuxRunner{})

	m := enterInteractive(t, interactiveTestModel())
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("tick returned nil cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("tick cmd produced %T, want tea.BatchMsg", cmd())
	}

	captured := map[int]bool{}
	sawWindowsLoad := false
	for _, c := range batch {
		switch value := c().(type) {
		case interactiveCaptureMsg:
			if value.session != "work" {
				t.Errorf("capture session = %q, want work", value.session)
			}
			captured[value.window] = true
		case windowsLoadedMsg:
			sawWindowsLoad = true
			if value.sessionName != "work" {
				t.Errorf("windows load session = %q, want work", value.sessionName)
			}
		}
	}
	if !sawWindowsLoad {
		t.Error("tick batch did not refresh the window list")
	}
	for _, want := range []int{0, 1, 2} {
		if !captured[want] {
			t.Errorf("tick batch did not capture visible window %d", want)
		}
	}
	if len(captured) != 3 {
		t.Errorf("captured windows = %v, want exactly the 3 visible bands", captured)
	}
}

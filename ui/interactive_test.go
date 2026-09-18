package ui

import (
	"errors"
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

func TestInteractiveNavigationMovesCursor(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.interactiveMod.setWindows(tenPlusWindows(12))

	if got := m.interactiveMod.cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0", got)
	}
	for range 11 {
		m = updateModel(t, m, runeKey("j"))
	}
	if got := m.interactiveMod.cursor; got != 11 {
		t.Fatalf("cursor = %d, want 11", got)
	}

	m = updateModel(t, m, runeKey("g"))
	if m.interactiveMod.cursor != 0 {
		t.Fatalf("after g cursor = %d, want 0", m.interactiveMod.cursor)
	}
	m = updateModel(t, m, runeKey("G"))
	if m.interactiveMod.cursor != 11 {
		t.Fatalf("after G cursor = %d, want 11", m.interactiveMod.cursor)
	}
	m = updateModel(t, m, runeKey("k"))
	if m.interactiveMod.cursor != 10 {
		t.Fatalf("after k cursor = %d, want 10", m.interactiveMod.cursor)
	}
}

func TestInteractiveNavigationCyclesAtTheEdges(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.interactiveMod.setWindows(tenPlusWindows(12))
	m.interactiveMod.cursor = 11

	m = updateModel(t, m, runeKey("j"))
	if m.interactiveMod.cursor != 0 {
		t.Fatalf("j at last cursor = %d, want 0 (cycled)", m.interactiveMod.cursor)
	}

	m = updateModel(t, m, runeKey("k"))
	if m.interactiveMod.cursor != 11 {
		t.Fatalf("k at first cursor = %d, want 11 (cycled)", m.interactiveMod.cursor)
	}
}

func TestInteractiveDeckViewDerivesVisibilityFromCursor(t *testing.T) {
	im := newInteractiveModel("work")
	im.setWindows(tenPlusWindows(12))

	tests := []struct {
		cursor  int
		visible int
		first   int
		count   int
	}{
		{cursor: 0, visible: 3, first: 10, count: 3},
		{cursor: 1, visible: 3, first: 11, count: 3},
		{cursor: 5, visible: 3, first: 3, count: 3},
		{cursor: 11, visible: 3, first: 9, count: 3},
		{cursor: 11, visible: 12, first: 0, count: 12},
	}
	for _, tt := range tests {
		im.cursor = tt.cursor
		first, count := im.deckView(tt.visible)
		if first != tt.first || count != tt.count {
			t.Errorf("deckView(cursor=%d, visible=%d) = %d/%d, want %d/%d",
				tt.cursor, tt.visible, first, count, tt.first, tt.count)
		}
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
		{"four equal bands fit", 4, 44, 4},
		{"five cap at four bands", 5, 44, 4},
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

	// Window 1 was killed outside; indexes shift down. The reload also flips
	// the active flag, but navigation intent wins over the new active window.
	m = updateModel(t, m, windowsLoadedMsg{
		sessionName: "work",
		windows: []tmux.Window{
			{Index: 0, Name: "editor", Active: true},
			{Index: 2, Name: "logs"},
		},
	})
	if m.interactiveMod.cursor != 1 {
		t.Fatalf("cursor = %d, want 1 (same window index 2)", m.interactiveMod.cursor)
	}
}

func TestInteractiveFirstLoadPreselectsActiveWindow(t *testing.T) {
	t.Run("cold entry via list refresh", func(t *testing.T) {
		m := NewModel()
		m.sessions = []tmux.Session{{Name: "fresh"}}
		m.applyFilter()
		m.width, m.height = 100, 40
		m = updateModel(t, m, runeKey("i"))

		m = updateModel(t, m, windowsLoadedMsg{
			sessionName: "fresh",
			windows: []tmux.Window{
				{Index: 0, Name: "editor"},
				{Index: 1, Name: "server", Active: true},
				{Index: 2, Name: "logs"},
			},
		})
		if m.interactiveMod.cursor != 1 {
			t.Fatalf("cursor = %d, want 1 (active window)", m.interactiveMod.cursor)
		}
	})

	t.Run("warm entry from cache", func(t *testing.T) {
		m := NewModel()
		m.sessions = []tmux.Session{{Name: "work"}}
		m.tree.setSessionExpanded("work", true)
		m.tree.windowsCache["work"] = []tmux.Window{
			{Index: 0, Name: "editor"},
			{Index: 1, Name: "server", Active: true},
		}
		m.applyFilter()
		m.width, m.height = 100, 40
		m = updateModel(t, m, runeKey("i"))
		if m.interactiveMod.cursor != 1 {
			t.Fatalf("cursor = %d, want 1 (active window from cache)", m.interactiveMod.cursor)
		}
	})

	t.Run("no active flag falls back to first", func(t *testing.T) {
		m := NewModel()
		m.sessions = []tmux.Session{{Name: "fresh"}}
		m.applyFilter()
		m.width, m.height = 100, 40
		m = updateModel(t, m, runeKey("i"))
		m = updateModel(t, m, windowsLoadedMsg{
			sessionName: "fresh",
			windows:     []tmux.Window{{Index: 3, Name: "only"}},
		})
		if m.interactiveMod.cursor != 0 {
			t.Fatalf("cursor = %d, want 0", m.interactiveMod.cursor)
		}
	})
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
	m = updateModel(t, m, windowsLoadedMsg{sessionName: "work", err: errSessionGone})
	if m.mode != modeList {
		t.Fatalf("mode after session death = %v, want modeList", m.mode)
	}
}

var errSessionGone = errors.New("can't find session: work")

func TestInteractiveSessionSurvivesRefresh(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, windowsLoadedMsg{sessionName: "work", windows: tenPlusWindows(3)})
	if m.mode != modeInteractive {
		t.Fatalf("mode after refresh = %v, want modeInteractive", m.mode)
	}
}

func TestInteractiveCaptureCmdsCoverOnScreenWindowsOnly(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.interactiveMod.setWindows(tenPlusWindows(12))
	m.interactiveMod.cursor = 11

	cmds := m.interactiveMod.captureCmds(40)
	if len(cmds) != 3 {
		t.Fatalf("capture cmds = %d, want 3 on-screen windows", len(cmds))
	}

	// Early cursor: the deck wraps to the tail, never leaving blank bands.
	m.interactiveMod.cursor = 0
	if cmds := m.interactiveMod.captureCmds(40); len(cmds) != 3 {
		t.Fatalf("capture cmds at cursor 0 = %d, want 3 (cyclic)", len(cmds))
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

func TestRenderInteractiveDeckLayout(t *testing.T) {
	useSolarizedTrueColor(t)
	m := enterInteractive(t, interactiveTestModel())
	m.width, m.height = 60, 40
	m.interactiveMod.cursor = 2
	m.interactiveMod.captures[0] = "zero"
	m.interactiveMod.captures[1] = "one"
	m.interactiveMod.captures[2] = "two"

	rendered := m.viewInteractive()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 40 {
		t.Fatalf("rendered lines = %d, want 40", len(lines))
	}

	// Equal deck bands (rows 1..13 and 14..26), one double-rule row (27),
	// slot (28..39), indicator row on top; geometry never depends on the
	// cursor.
	primary := rgb{r: 7, g: 102, b: 120} // #076678
	assertEveryVisibleCellUsesForeground(t, lines[27], primary)

	plain := stripLines(rendered)
	if strings.Contains(strings.Join(plain, "\n"), "peeking") {
		t.Fatal("band titles still rendered")
	}
	if got := plain[27]; strings.Trim(got, "═") != "" {
		t.Errorf("rule row 27 = %q, want full double rule", got)
	}
	// Captures bottom-anchor inside their regions: deck windows above, the
	// selected window in the slot ending at the very bottom row.
	for _, want := range []struct {
		row  int
		text string
	}{
		{13, "zero"}, {26, "one"}, {39, "two"},
	} {
		if got := plain[want.row]; !strings.HasPrefix(got, want.text) {
			t.Errorf("row %d = %q, want capture %q", want.row, got, want.text)
		}
	}

	// Steadiness: the rule never moves while the deck rolls.
	m.interactiveMod.cursor = 1
	rolled := stripLines(m.viewInteractive())
	rolledLines := strings.Split(m.viewInteractive(), "\n")
	assertEveryVisibleCellUsesForeground(t, rolledLines[27], primary)
	if got := rolled[27]; strings.Trim(got, "═") != "" {
		t.Errorf("rule row 27 moved: %q", got)
	}
	if got := rolled[39]; !strings.HasPrefix(got, "one") {
		t.Errorf("slot after roll = %q, want capture one at the bottom", got)
	}
}

func TestRenderInteractiveIndicatorsFrameOverflow(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 40
	m.interactiveMod.setWindows(tenPlusWindows(12))
	m.interactiveMod.cursor = 11
	m.interactiveMod.captures[9] = "nine"
	m.interactiveMod.captures[10] = "ten"
	m.interactiveMod.captures[11] = "eleven"

	rendered := m.viewInteractive()
	if len(strings.Split(rendered, "\n")) != 40 {
		t.Fatalf("rendered lines = %d, want 40", len(strings.Split(rendered, "\n")))
	}
	plain := stripLines(rendered)
	if got := plain[0]; !strings.Contains(got, "↑ 9 windows above") {
		t.Errorf("top indicator = %q, want ↑ 9 windows above", got)
	}
	for _, want := range []struct {
		row  int
		text string
	}{
		{13, "nine"}, {26, "ten"}, {39, "eleven"},
	} {
		if got := plain[want.row]; !strings.HasPrefix(got, want.text) {
			t.Errorf("row %d = %q, want capture %q", want.row, got, want.text)
		}
	}

	// Early window: the deck wraps to the tail — no blank bands, no spacer.
	m.interactiveMod.cursor = 0
	m.interactiveMod.captures[0] = "first"
	plain = stripLines(m.viewInteractive())
	if got := plain[0]; !strings.Contains(got, "↑ 9 windows above") {
		t.Errorf("indicator at cursor 0 = %q, want ↑ 9 windows above (constant)", got)
	}
	if got := plain[13]; !strings.HasPrefix(got, "ten") {
		t.Errorf("cyclic deck row 13 = %q, want capture ten", got)
	}
	if got := plain[26]; !strings.HasPrefix(got, "eleven") {
		t.Errorf("cyclic deck row 26 = %q, want capture eleven", got)
	}
	if got := plain[39]; !strings.HasPrefix(got, "first") {
		t.Errorf("slot at cursor 0 = %q, want capture first", got)
	}
}

func TestRenderInteractiveBottomCropsContent(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 36 // equal bands: 12, 11, 11 content rows
	m.interactiveMod.cursor = 2
	capture := make([]string, 20)
	for i := range capture {
		capture[i] = fmt.Sprintf("line-%02d", i)
	}
	m.interactiveMod.captures[1] = strings.Join(capture, "\n")

	plain := stripLines(m.viewInteractive())
	// The lower deck band (window 1) occupies rows 13..23; an 11-row window
	// over a 20-line capture keeps the tail: lines 09..19.
	if got := plain[13]; got != "line-09" {
		t.Errorf("first deck row = %q, want line-09", got)
	}
	if got := plain[23]; got != "line-19" {
		t.Errorf("last deck row = %q, want line-19", got)
	}
}

func TestRenderInteractiveEveryLineMatchesWidth(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.width, m.height = 80, 40
	m.interactiveMod.setWindows(tenPlusWindows(12))
	m.interactiveMod.cursor = 5

	rendered := m.viewInteractive()
	for i, line := range strings.Split(rendered, "\n") {
		if got := ansi.StringWidth(line); got != 80 {
			t.Errorf("line %d width = %d, want 80", i, got)
		}
	}
}

func TestInteractiveViewTooSmallFallsBackToError(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.width, m.height = 80, 9
	rendered := m.viewInteractive()
	if !strings.Contains(ansi.Strip(rendered), "Terminal too small") {
		t.Fatalf("small terminal fallback missing message:\n%s", rendered)
	}
	if len(strings.Split(rendered, "\n")) != 9 {
		t.Fatal("small terminal fallback does not fill exactly 9 rows")
	}
}

func TestInteractiveResizeRendersWithoutStaleState(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m.height = 40
	m.interactiveMod.setWindows(tenPlusWindows(12))
	m.interactiveMod.cursor = 11

	// Grow the terminal to fit every window; visibility is derived from the
	// cursor so no stale scroll state can leak into the renderer.
	m = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 160})
	rendered := m.viewInteractive()
	if len(strings.Split(rendered, "\n")) != 160 {
		t.Fatalf("rendered lines = %d, want 160", len(strings.Split(rendered, "\n")))
	}

	m = updateModel(t, m, tea.WindowSizeMsg{Width: 120, Height: 20})
	rendered = m.viewInteractive()
	if len(strings.Split(rendered, "\n")) != 20 {
		t.Fatalf("rendered lines after shrink = %d, want 20", len(strings.Split(rendered, "\n")))
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
	m.interactiveMod.cursor = 2 // full deck: windows 0 and 1 above, 2 in the slot
	_, cmd := m.Update(interactiveTickMsg{})
	if cmd == nil {
		t.Fatal("interactive tick returned nil cmd")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("interactive tick produced %T, want tea.BatchMsg", cmd())
	}

	captured := map[int]bool{}
	sawTick := false
	for _, c := range batch {
		switch value := c().(type) {
		case interactiveCaptureMsg:
			if value.session != "work" {
				t.Errorf("capture session = %q, want work", value.session)
			}
			captured[value.window] = true
		case interactiveTickMsg:
			sawTick = true
		case windowsLoadedMsg:
			t.Error("early fast tick refreshed the window list")
		}
	}
	if !sawTick {
		t.Error("fast tick did not schedule its successor")
	}
	for _, want := range []int{0, 1, 2} {
		if !captured[want] {
			t.Errorf("fast tick did not capture visible window %d", want)
		}
	}
	if len(captured) != 3 {
		t.Errorf("captured windows = %v, want exactly the 3 visible bands", captured)
	}
}

func TestInteractiveWindowListRefreshesPeriodically(t *testing.T) {
	tmux.SetRunner(stubTmuxRunner{})
	defer tmux.SetRunner(execTmuxRunner{})

	m := enterInteractive(t, interactiveTestModel())
	for range interactiveWindowRefreshTicks - 1 {
		m = updateModel(t, m, interactiveTickMsg{})
	}
	_, cmd := m.Update(interactiveTickMsg{})
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("tick %d produced %T, want tea.BatchMsg", interactiveWindowRefreshTicks, cmd())
	}
	sawWindowsLoad := false
	for _, c := range batch {
		if _, isWindows := c().(windowsLoadedMsg); isWindows {
			sawWindowsLoad = true
		}
	}
	if !sawWindowsLoad {
		t.Fatal("periodic fast tick did not refresh the window list")
	}
}

func TestInteractiveFastTickStopsAfterExit(t *testing.T) {
	m := enterInteractive(t, interactiveTestModel())
	m = updateModel(t, m, runeKey("i")) // exit
	_, cmd := m.Update(interactiveTickMsg{})
	if cmd != nil {
		t.Fatal("stale fast tick still produced work after exit")
	}
}

func TestGlobalTickSkipsSessionsWhileInteractive(t *testing.T) {
	tmux.SetRunner(stubTmuxRunner{})
	defer tmux.SetRunner(execTmuxRunner{})

	m := enterInteractive(t, interactiveTestModel())
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("global tick returned nil cmd, want picker loop kept alive")
	}
	if _, isTick := cmd().(tickMsg); !isTick {
		t.Fatalf("global tick cmd produced %T, want only the picker loop", cmd())
	}
}

func TestInteractiveExitRefreshesPickerSessions(t *testing.T) {
	tmux.SetRunner(stubTmuxRunner{})
	defer tmux.SetRunner(execTmuxRunner{})

	m := enterInteractive(t, interactiveTestModel())
	_, cmd := m.Update(runeKey("esc"))
	if cmd == nil {
		t.Fatal("exit returned nil cmd, want loadSessions")
	}
	if _, ok := cmd().(sessionsLoadedMsg); !ok {
		t.Fatalf("exit cmd produced %T, want sessionsLoadedMsg", cmd())
	}
}

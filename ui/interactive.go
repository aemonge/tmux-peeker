package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	// heroRows is the fixed content height of the selected window's band.
	heroRows = 12
	// queueMinRows is the deck-band floor. Above it, every leftover row is
	// divided into the bands — no ceiling, no orphan blank rows at the top.
	queueMinRows = 8
	// interactiveRefreshInterval is the grid's dedicated capture cadence,
	// decoupled from the picker's slower session tick.
	interactiveRefreshInterval = 200 * time.Millisecond
	// interactiveWindowRefreshTicks refreshes the window list every N fast
	// ticks (about 2 s at the default cadence).
	interactiveWindowRefreshTicks = 10
)

// interactiveModel drives the fullscreen multi-window session view: equal
// horizontal bands, one per window in index order, navigable with the
// list-style keys.
type interactiveModel struct {
	session   string
	windows   []tmux.Window
	cursor    int            // selected window; its band is always the bottom slot
	captures  map[int]string // tmux window index -> latest capture
	loaded    bool           // windows have been fetched at least once
	tickCount int            // fast ticks since entry, drives window refresh
}

func newInteractiveModel(sessionName string) interactiveModel {
	return interactiveModel{session: sessionName, captures: make(map[int]string)}
}

// setWindows replaces the window list. The first load lands the cursor on
// the session's active window (the launch window); later reloads keep the
// cursor on the same window when it still exists and clamp otherwise.
func (im *interactiveModel) setWindows(windows []tmux.Window) {
	firstLoad := !im.loaded
	previous := -1
	if im.cursor < len(im.windows) {
		previous = im.windows[im.cursor].Index
	}
	im.windows = windows
	im.loaded = true
	im.cursor = 0
	if firstLoad {
		for i, w := range windows {
			if w.Active {
				im.cursor = i
				break
			}
		}
	} else {
		for i, w := range windows {
			if w.Index == previous {
				im.cursor = i
				break
			}
		}
	}
	if im.cursor >= len(windows) {
		im.cursor = max(0, len(windows)-1)
	}
}

// interactiveVisibleBands reports how many window bands fit in height
// rows: one fixed hero band plus as many deck bands as fit while keeping
// queueMinRows of content each. Below the hero-only minimum it reports 0.
func interactiveVisibleBands(windowCount, height int) int {
	if windowCount <= 0 || height < interactiveMinimumHeight {
		return 0
	}
	fit := 0
	for n := 1; n < windowCount; n++ {
		if (queueMinRows+1)*n+heroRows+2 > height {
			break
		}
		fit = n
	}
	return fit + 1
}

// move shifts the selection by delta windows, cycling past either end.
// Visibility is derived from the cursor on every render, so no scroll
// state needs maintaining.
func (im *interactiveModel) move(delta int) {
	if len(im.windows) == 0 {
		return
	}
	n := len(im.windows)
	im.cursor = ((im.cursor+delta)%n + n) % n
}

func (im *interactiveModel) first() {
	im.cursor = 0
}

func (im *interactiveModel) last() {
	im.cursor = max(0, len(im.windows)-1)
}

// deckView returns the window indexes currently on screen as a cyclic
// slice ending at the selected window: the deck wraps around the list, so
// early selections show the tail windows above instead of blanks.
func (im *interactiveModel) deckView(visible int) (first, count int) {
	if visible <= 0 || len(im.windows) == 0 || im.cursor >= len(im.windows) {
		return 0, 0
	}
	n := len(im.windows)
	first = ((im.cursor-(visible-1))%n + n) % n
	return first, min(visible, n)
}

// interactiveCaptureMsg carries a fresh capture for one interactive band.
type interactiveCaptureMsg struct {
	session string
	window  int
	content string
}

// captureInteractiveWindow captures the active pane of one window.
func captureInteractiveWindow(sessionName string, windowIndex int) tea.Cmd {
	key := previewKey{session: sessionName, window: windowIndex, pane: -1}
	return func() tea.Msg {
		content, err := tmux.CapturePaneTarget(key.target())
		if err != nil {
			content = ""
		}
		return interactiveCaptureMsg{session: sessionName, window: windowIndex, content: content}
	}
}

// captureCmds returns capture commands for the windows on screen, walking
// the cyclic deck slice.
func (im *interactiveModel) captureCmds(height int) []tea.Cmd {
	if !im.loaded || len(im.windows) == 0 || height <= 0 {
		return nil
	}
	visible := interactiveVisibleBands(len(im.windows), height)
	first, count := im.deckView(visible)
	n := len(im.windows)
	cmds := make([]tea.Cmd, 0, count)
	for i := 0; i < count; i++ {
		idx := (first + i) % n
		cmds = append(cmds, captureInteractiveWindow(im.session, im.windows[idx].Index))
	}
	return cmds
}

// visibleWindow returns the window at cursor, if any.
func (im *interactiveModel) visibleWindow() *tmux.Window {
	if im.cursor < 0 || im.cursor >= len(im.windows) {
		return nil
	}
	return &im.windows[im.cursor]
}

// interactiveMinimumHeight is the smallest terminal height that can show
// the indicator row, the hero header rule, and the fixed 12-row hero band.
const interactiveMinimumHeight = heroRows + 2

// interactiveLayout describes the bottom-anchored deck: deck bands sharing
// every leftover row above the hero block (header rule plus fixed heroRows
// of capture). Geometry is fully static; only content changes as the cursor
// moves.
type interactiveLayout struct {
	visible     int   // total bands on screen, including the hero
	deckRows    []int // content rows per deck band, top to bottom
	slotRows    int   // content rows for the hero band; always heroRows
	slackRows   int   // spare rows above the hero header (hero-only mode)
	hiddenAbove int   // windows hidden beyond the top indicator
}

func computeInteractiveLayout(windowCount, cursor, height int) interactiveLayout {
	visible := interactiveVisibleBands(windowCount, height)
	if visible <= 0 {
		return interactiveLayout{}
	}
	deckBands := visible - 1
	layout := interactiveLayout{
		visible:  visible,
		slotRows: heroRows,
		// The cyclic deck hides a constant count: every window not on screen.
		hiddenAbove: max(0, windowCount-visible),
	}
	if deckBands == 0 {
		layout.slackRows = height - heroRows - 2
		return layout
	}
	// Every leftover row is divided into the deck bands: with bands on
	// screen the layout consumes the height exactly, so no orphan blank
	// rows can appear at the top.
	budget := height - heroRows - deckBands - 1
	base := budget / deckBands
	leftover := budget % deckBands
	layout.deckRows = make([]int, deckBands)
	for i := range layout.deckRows {
		layout.deckRows[i] = base
		if leftover > 0 {
			layout.deckRows[i]++
			leftover--
		}
	}
	return layout
}

// renderInteractiveView paints the bottom-anchored deck: a top indicator
// row, deck bands of unselected windows behind labeled dot rules, and the
// hero block — header rule over fixed heroRows of capture reaching the
// bottom edge. Geometry is static; moving the cursor only changes which
// content each region carries.
func renderInteractiveView(m *Model) string {
	im := &m.interactiveMod
	if !im.loaded {
		return interactiveNotice("Loading…", m.width, m.height)
	}
	if len(im.windows) == 0 {
		return interactiveNotice("No windows in this session", m.width, m.height)
	}
	layout := computeInteractiveLayout(len(im.windows), im.cursor, m.height)
	if layout.visible == 0 {
		return interactiveNotice("Terminal too small for the interactive view", m.width, m.height)
	}

	blank := strings.Repeat(" ", m.width)
	lines := make([]string, 0, m.height)
	lines = append(lines, interactiveIndicatorRow(layout.hiddenAbove, "above", m.width))
	for i := 0; i < layout.slackRows; i++ {
		lines = append(lines, blank)
	}
	deckBands := len(layout.deckRows)
	n := len(im.windows)
	for b := 0; b < deckBands; b++ {
		// Cyclic fill: bands above an early selection wrap to the list tail,
		// so no band ever renders blank. Every rule is a header for the
		// band directly below it.
		windowIdx := ((im.cursor-deckBands+b)%n + n) % n
		if b > 0 {
			lines = append(lines, queueRule(m.width, windowRuleLabel(im.windows[windowIdx])))
		}
		content := im.captures[im.windows[windowIdx].Index]
		lines = append(lines, strings.Split(renderPreview(content, m.width, layout.deckRows[b]), "\n")...)
	}
	selected := im.windows[im.cursor]
	lines = append(lines, heroHeaderRule(m.width, windowRuleLabel(selected)))
	lines = append(lines, strings.Split(renderPreview(im.captures[selected.Index], m.width, layout.slotRows), "\n")...)
	return fixedBox(strings.Join(lines, "\n"), m.width, m.height)
}

// windowRuleLabel names a window the way the tree does: index:name.
func windowRuleLabel(w tmux.Window) string {
	return fmt.Sprintf("%d:%s", w.Index, w.Name)
}

// interactiveIndicatorRow renders a dim overflow indicator; a count of zero
// keeps the row as a quiet spacer so band heights stay stable.
func interactiveIndicatorRow(count int, direction string, width int) string {
	if count > 0 {
		plural := "s"
		if count == 1 {
			plural = ""
		}
		arrow := "↑"
		if direction == "below" {
			arrow = "↓"
		}
		text := fmt.Sprintf("%s %d window%s %s", arrow, count, plural, direction)
		return lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorSurface).
			Render(truncateAndCenter(text, width))
	}
	return surfaceSpaces(width)
}

// interactiveNotice fills the screen with a single centered dim message.
func interactiveNotice(message string, width, height int) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = strings.Repeat(" ", width)
	}
	lines[height/2] = padOrTruncate(centerText(message, width), width)
	return strings.Join(lines, "\n")
}

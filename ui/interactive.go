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
	// interactiveMinContentRows is the smallest number of live content rows a
	// band may show before windows overflow into indicator rows instead.
	// The selected band additionally pays two border rows from its budget.
	interactiveMinContentRows = 10
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

// interactiveVisibleBands reports how many window bands (deck plus slot)
// fit in height rows while keeping the minimum content rows per deck band:
// one indicator row, two double-rule rows, deck bands, and a 1.5x slot.
func interactiveVisibleBands(windowCount, height int) int {
	if windowCount <= 0 {
		return 0
	}
	fit := max(1, (height-8)/interactiveMinContentRows)
	return min(windowCount, fit)
}

// move shifts the selection by delta windows. Visibility is derived from
// the cursor on every render, so no scroll state needs maintaining.
func (im *interactiveModel) move(delta int) {
	if len(im.windows) == 0 {
		return
	}
	im.cursor = min(max(im.cursor+delta, 0), len(im.windows)-1)
}

func (im *interactiveModel) first() {
	im.cursor = 0
}

func (im *interactiveModel) last() {
	im.cursor = max(0, len(im.windows)-1)
}

// deckView returns the window indexes currently on screen: the deck bands
// above the slot plus the selected window itself. Blank deck fills occur
// when the cursor sits early in the list.
func (im *interactiveModel) deckView(visible int) (first, count int) {
	if visible <= 0 || len(im.windows) == 0 || im.cursor >= len(im.windows) {
		return 0, 0
	}
	first = max(0, im.cursor-(visible-1))
	return first, im.cursor - first + 1
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

// captureCmds returns capture commands for the windows on screen.
func (im *interactiveModel) captureCmds(height int) []tea.Cmd {
	if !im.loaded || len(im.windows) == 0 || height <= 0 {
		return nil
	}
	visible := interactiveVisibleBands(len(im.windows), height)
	first, count := im.deckView(visible)
	cmds := make([]tea.Cmd, 0, count)
	for i := first; i < first+count; i++ {
		cmds = append(cmds, captureInteractiveWindow(im.session, im.windows[i].Index))
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
// the indicator row, the double rule, and one minimum-size slot.
const interactiveMinimumHeight = interactiveMinContentRows + 3

// interactiveLayout describes the bottom-anchored deck: deck bands above a
// double rule and the selected window's taller slot below it. Geometry is
// fully static; only content changes as the cursor moves.
type interactiveLayout struct {
	visible     int   // total bands on screen, including the slot
	deckRows    []int // content rows per deck band, top to bottom
	slotRows    int   // content rows for the selected slot
	hiddenAbove int   // windows hidden beyond the top indicator
}

func computeInteractiveLayout(windowCount, cursor, height int) interactiveLayout {
	visible := interactiveVisibleBands(windowCount, height)
	if visible <= 0 {
		return interactiveLayout{}
	}
	deckBands := visible - 1
	layout := interactiveLayout{
		visible:     visible,
		hiddenAbove: max(0, cursor-deckBands),
	}
	available := height - 3 // top indicator row plus the two-row double rule
	if deckBands <= 0 {
		layout.slotRows = max(1, available)
		return layout
	}
	// Deck bands cost 2 size-units each, the slot 3 (about 1.5x a deck band).
	units := 2*deckBands + 3
	unit := available / units
	leftover := available % units
	layout.deckRows = make([]int, deckBands)
	for i := range layout.deckRows {
		layout.deckRows[i] = 2 * unit
		if leftover > 0 {
			layout.deckRows[i]++
			leftover--
		}
	}
	layout.slotRows = 3*unit + leftover
	return layout
}

// renderInteractiveView paints the bottom-anchored deck: a top indicator
// row, deck bands of unselected windows, a double rule, and the selected
// window's taller slot. Geometry is static; moving the cursor only changes
// which content each region carries.
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

	lines := make([]string, 0, m.height)
	lines = append(lines, interactiveIndicatorRow(layout.hiddenAbove, "above", m.width))
	deckBands := len(layout.deckRows)
	for b := 0; b < deckBands; b++ {
		windowIdx := im.cursor - deckBands + b
		content := ""
		if windowIdx >= 0 {
			content = im.captures[im.windows[windowIdx].Index]
		}
		lines = append(lines, strings.Split(renderPreview(content, m.width, layout.deckRows[b]), "\n")...)
	}
	rule := interactiveDoubleRule(m.width)
	lines = append(lines, rule, rule)
	lines = append(lines, strings.Split(renderPreview(im.captures[im.windows[im.cursor].Index], m.width, layout.slotRows), "\n")...)
	return fixedBox(strings.Join(lines, "\n"), m.width, m.height)
}

// interactiveDoubleRule renders one row of the classic double rule that
// separates the deck from the selected slot.
func interactiveDoubleRule(width int) string {
	return lipgloss.NewStyle().
		Foreground(colorPrimary).
		Render(strings.Repeat("═", width))
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

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
	cursor    int            // index into windows
	offset    int            // first visible window
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
func (im *interactiveModel) setWindows(windows []tmux.Window, height int) {
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
	im.ensureCursorVisible(height)
}

// interactiveVisibleBands reports how many window bands fit in height rows
// while keeping the minimum content rows per band and one shared separator
// row per boundary (indicator rows replace the edges when overflowing).
func interactiveVisibleBands(windowCount, height int) int {
	if windowCount <= 0 {
		return 0
	}
	fit := max(1, (height-1)/(interactiveMinContentRows+1))
	return min(windowCount, fit)
}

// move shifts the cursor by delta windows, scrolling the band window when
// the cursor crosses a visible edge.
func (im *interactiveModel) move(delta, height int) {
	if len(im.windows) == 0 {
		return
	}
	im.cursor = min(max(im.cursor+delta, 0), len(im.windows)-1)
	im.ensureCursorVisible(height)
}

func (im *interactiveModel) first() {
	im.cursor = 0
	im.offset = 0
}

func (im *interactiveModel) last(height int) {
	im.cursor = max(0, len(im.windows)-1)
	im.ensureCursorVisible(height)
}

// ensureCursorVisible scrolls offset so the cursor sits inside the visible
// bands for the given terminal height.
func (im *interactiveModel) ensureCursorVisible(height int) {
	visible := interactiveVisibleBands(len(im.windows), height)
	if im.cursor < im.offset {
		im.offset = im.cursor
	}
	if visible > 0 && im.cursor >= im.offset+visible {
		im.offset = im.cursor - visible + 1
	}
	im.offset = min(max(im.offset, 0), max(0, len(im.windows)-max(visible, 1)))
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

// captureCmds returns capture commands for the currently visible bands.
func (im *interactiveModel) captureCmds(height int) []tea.Cmd {
	if !im.loaded || len(im.windows) == 0 || height <= 0 {
		return nil
	}
	visible := interactiveVisibleBands(len(im.windows), height)
	end := min(len(im.windows), im.offset+visible)
	cmds := make([]tea.Cmd, 0, visible)
	for i := im.offset; i < end; i++ {
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

// interactiveMinimumHeight is the smallest terminal height that can show one
// band at the minimum content size.
const interactiveMinimumHeight = interactiveMinContentRows

// interactiveLayout describes how the interactive view divides the screen.
// Every band boundary carries exactly one separator row so the layout never
// changes when selection moves; overflow trades the edge rows for
// indicator rows.
type interactiveLayout struct {
	visible     int
	bandRows    []int // content rows per visible band
	hiddenAbove int
	hiddenBelow int
	overflow    bool
}

func computeInteractiveLayout(windowCount, offset, height int) interactiveLayout {
	visible := interactiveVisibleBands(windowCount, height)
	if visible <= 0 {
		return interactiveLayout{}
	}
	layout := interactiveLayout{visible: visible, overflow: windowCount > visible}
	reserved := 0
	separators := visible + 1
	if layout.overflow {
		reserved = 2
		separators = visible - 1
		layout.hiddenAbove = offset
		layout.hiddenBelow = windowCount - offset - visible
	}
	available := height - reserved - separators
	base := max(1, available/visible)
	leftover := available % visible
	layout.bandRows = make([]int, visible)
	for i := range layout.bandRows {
		layout.bandRows[i] = base
		if i < leftover {
			layout.bandRows[i]++
		}
	}
	return layout
}

// renderInteractiveView paints the fullscreen multi-window view: pure
// preview content per visible window band with no titles, divided by one
// steady separator row per boundary. Selection only recolors the selected
// band's adjacent separators, so moving the cursor never reflows anything.
func renderInteractiveView(m *Model) string {
	im := &m.interactiveMod
	if !im.loaded {
		return interactiveNotice("Loading…", m.width, m.height)
	}
	if len(im.windows) == 0 {
		return interactiveNotice("No windows in this session", m.width, m.height)
	}
	layout := computeInteractiveLayout(len(im.windows), im.offset, m.height)
	if layout.visible == 0 {
		return interactiveNotice("Terminal too small for the interactive view", m.width, m.height)
	}

	bandSelected := func(band int) bool {
		return im.offset+band == im.cursor
	}

	lines := make([]string, 0, m.height)
	if layout.overflow {
		lines = append(lines, interactiveIndicatorRow(layout.hiddenAbove, "above", m.width))
	} else {
		lines = append(lines, interactiveSeparatorRow(bandSelected(0), m.width))
	}
	for i := 0; i < layout.visible; i++ {
		content := im.captures[im.windows[im.offset+i].Index]
		lines = append(lines, strings.Split(renderPreview(content, m.width, layout.bandRows[i]), "\n")...)
		switch {
		case i == layout.visible-1 && !layout.overflow:
			lines = append(lines, interactiveSeparatorRow(bandSelected(i), m.width))
		case i < layout.visible-1:
			// Shared boundary: bright when either neighbor is selected.
			lines = append(lines, interactiveSeparatorRow(bandSelected(i) || bandSelected(i+1), m.width))
		}
	}
	if layout.overflow {
		lines = append(lines, interactiveIndicatorRow(layout.hiddenBelow, "below", m.width))
	}
	return fixedBox(strings.Join(lines, "\n"), m.width, m.height)
}

// interactiveSeparatorRow renders one band boundary. Bright separators use
// the theme's primary color; quiet ones use the theme's border color.
func interactiveSeparatorRow(bright bool, width int) string {
	color := colorBorder
	if bright {
		color = colorPrimary
	}
	return lipgloss.NewStyle().
		Foreground(color).
		Render(strings.Repeat("─", width))
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

package ui

import (
	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	// interactiveMinContentRows is the smallest number of live content rows a
	// band may show before windows overflow into indicator rows instead.
	interactiveMinContentRows = 10
	interactiveHeaderRows     = 1
)

// interactiveModel drives the fullscreen multi-window session view: equal
// horizontal bands, one per window in index order, navigable with the
// list-style keys.
type interactiveModel struct {
	session  string
	windows  []tmux.Window
	cursor   int            // index into windows
	offset   int            // first visible window
	captures map[int]string // tmux window index -> latest capture
	loaded   bool           // windows have been fetched at least once
}

func newInteractiveModel(sessionName string) interactiveModel {
	return interactiveModel{session: sessionName, captures: make(map[int]string)}
}

// setWindows replaces the window list, keeping the cursor on the same window
// when it still exists and clamping otherwise.
func (im *interactiveModel) setWindows(windows []tmux.Window, height int) {
	previous := -1
	if im.cursor < len(im.windows) {
		previous = im.windows[im.cursor].Index
	}
	im.windows = windows
	im.loaded = true
	im.cursor = 0
	for i, w := range windows {
		if w.Index == previous {
			im.cursor = i
			break
		}
	}
	if im.cursor >= len(windows) {
		im.cursor = max(0, len(windows)-1)
	}
	im.ensureCursorVisible(height)
}

// interactiveVisibleBands reports how many window bands fit in height rows.
// Overflowing sessions always reserve two indicator rows so the band count
// stays stable while scrolling.
func interactiveVisibleBands(windowCount, height int) int {
	if windowCount <= 0 {
		return 0
	}
	fit := func(reserved int) int {
		return max(1, (height-reserved)/(interactiveHeaderRows+interactiveMinContentRows))
	}
	visible := fit(0)
	if windowCount <= visible {
		return windowCount
	}
	return min(windowCount, fit(2))
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

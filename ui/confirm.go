package ui

import (
	"fmt"

	"github.com/aemonge/tmux-peeker/tmux"
	tea "github.com/charmbracelet/bubbletea"
)

// killTarget identifies the session, window, or pane a kill confirmation
// acts on. windowIndex and paneIndex are only meaningful for the matching kind.
type killTarget struct {
	kind        itemKind
	session     string
	windowIndex int
	paneIndex   int
}

// label renders the target as the tmux target string shown in the prompt.
func (t killTarget) label() string {
	switch t.kind {
	case itemWindow:
		return formatTarget(t.session, t.windowIndex, -1)
	case itemPane:
		return formatTarget(t.session, t.windowIndex, t.paneIndex)
	default:
		return t.session
	}
}

// kill issues the tmux command that destroys this target.
func (t killTarget) kill() error {
	switch t.kind {
	case itemWindow:
		return tmux.KillWindow(t.session, t.windowIndex)
	case itemPane:
		return tmux.KillPane(t.session, t.windowIndex, t.paneIndex)
	default:
		return tmux.KillSession(t.session)
	}
}

// killTargetForItem maps the selected list row to its kill target.
func killTargetForItem(it listItem) killTarget {
	switch it.kind {
	case itemWindow:
		return killTarget{kind: itemWindow, session: it.session.Name, windowIndex: it.window.Index}
	case itemPane:
		return killTarget{
			kind:        itemPane,
			session:     it.session.Name,
			windowIndex: it.window.Index,
			paneIndex:   it.pane.Index,
		}
	default:
		return killTarget{kind: itemSession, session: it.session.Name}
	}
}

type confirmKillModel struct {
	target killTarget
}

// killedMsg reports a confirmed kill's outcome, or a cancelled prompt.
type killedMsg struct {
	target    killTarget
	cancelled bool
	err       error
}

func newConfirmKillModel(target killTarget) confirmKillModel {
	return confirmKillModel{target: target}
}

func (m confirmKillModel) Update(msg tea.Msg, keyMap KeyMap) (confirmKillModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		pressed := key.String()
		switch {
		case keyMap.Matches(contextKill, "confirm", pressed):
			target := m.target
			err := target.kill()
			return m, func() tea.Msg {
				return killedMsg{target: target, err: err}
			}
		case keyMap.Matches(contextKill, "cancel", pressed):
			return m, func() tea.Msg {
				return killedMsg{cancelled: true}
			}
		}
	}
	return m, nil
}

func (m confirmKillModel) View(keyMap KeyMap) string {
	return errorStyle.Render(fmt.Sprintf("Kill %q? (%s confirm / %s cancel)",
		m.target.label(),
		keyMap.Help(contextKill, "confirm"),
		keyMap.Help(contextKill, "cancel")))
}

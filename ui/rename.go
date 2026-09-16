package ui

import (
	"github.com/aemonge/tmux-peeker/tmux"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type renameTarget struct {
	session string // owning session name
	window  int    // -1 renames the session itself; otherwise the window index
}

type renameModel struct {
	input   textinput.Model
	target  renameTarget
	oldName string
	err     error
}

type sessionRenamedMsg struct {
	oldName string
	newName string
}

type windowRenamedMsg struct {
	sessionName string
	windowIndex int
}

func newRenameModel(target renameTarget, oldName string) renameModel {
	input := textinput.New()
	input.Placeholder = oldName
	input.SetValue(oldName)
	input.Focus()
	input.CharLimit = 50
	input.Width = 40

	return renameModel{
		input:   input,
		target:  target,
		oldName: oldName,
	}
}

// newSessionRenameModel prepares the overlay for renaming a session.
func newSessionRenameModel(oldName string) renameModel {
	return newRenameModel(renameTarget{session: oldName, window: -1}, oldName)
}

// newWindowRenameModel prepares the overlay for renaming one window.
func newWindowRenameModel(sessionName string, windowIndex int, oldName string) renameModel {
	return newRenameModel(renameTarget{session: sessionName, window: windowIndex}, oldName)
}

func (m renameModel) Update(msg tea.Msg, keyMap KeyMap) (renameModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && keyMap.Matches(contextRename, "submit", key.String()) {
		newName := m.input.Value()
		if newName == "" || newName == m.oldName {
			return m, nil
		}
		if m.target.window < 0 {
			if err := tmux.RenameSession(m.target.session, newName); err != nil {
				m.err = err
				return m, nil
			}
			return m, func() tea.Msg {
				return sessionRenamedMsg{oldName: m.oldName, newName: newName}
			}
		}
		if err := tmux.RenameWindow(m.target.session, m.target.window, newName); err != nil {
			m.err = err
			return m, nil
		}
		return m, func() tea.Msg {
			return windowRenamedMsg{sessionName: m.target.session, windowIndex: m.target.window}
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m renameModel) View(keyMap KeyMap) string {
	label := "Rename Session"
	if m.target.window >= 0 {
		label = "Rename Window"
	}
	s := inputLabelStyle.Render(label) + "\n\n"
	s += inputLabelStyle.Render("Name: ") + m.input.View() + "\n\n"
	s += helpStyle.Render(keyMap.Help(contextRename, "submit") + " confirm • " +
		keyMap.Help(contextRename, "cancel") + " cancel")

	if m.err != nil {
		s += "\n" + errorStyle.Render(m.err.Error())
	}

	return s
}

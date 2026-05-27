package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	if m.currentScreen != screenMain {
		return m, nil
	}

	p := m.panelAt(msg.X, msg.Y)

	// Mouse wheel: scroll the panel under the cursor.
	if tea.MouseEvent(msg).IsWheel() {
		switch p {
		case panelLog:
			m.pane = paneRight
			m.rightPane = rightLog
			var cmd tea.Cmd
			m.logViewport, cmd = m.logViewport.Update(msg)
			if !m.logViewport.AtBottom() {
				m.logFollowTail = false
			}
			return m, cmd
		case panelEnv:
			m.pane = paneLeft
			m.leftPane = leftEnv
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m = m.moveUp()
			case tea.MouseButtonWheelDown:
				m = m.moveDown()
			}
			return m, nil
		}
		return m, nil
	}

	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	bodyY := msg.Y - m.bodyTopY()

	switch p {
	case panelExec:
		m.pane = paneLeft
		m.leftPane = leftExec
		if idx, ok := m.execIndexAtRow(bodyY); ok {
			m.execCursor = idx
			m.viewingExec = idx
			m.testCursor = 0
			m.resetLogView()
			m.refreshLog()
		}

	case panelEnv:
		m.pane = paneLeft
		m.leftPane = leftEnv
		if idx, ok := m.envIndexAtRow(m.envRelY(bodyY)); ok {
			m.envCursor = idx
		}

	case panelTests:
		m.pane = paneRight
		m.rightPane = rightTests
		if idx, ok := m.testIndexAtRow(bodyY); ok {
			m.testCursor = idx
			m.resetLogView()
			m.refreshLog()
		}

	case panelLog:
		m.pane = paneRight
		m.rightPane = rightLog
	}

	return m, nil
}

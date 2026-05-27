package tui

import tea "github.com/charmbracelet/bubbletea"

func (m Model) requestQuit() Model {
	m.quitReturnScreen = m.currentScreen
	m.quitConfirmIdx = 1 // default: No
	m.currentScreen = screenQuitConfirm
	return m
}

func (m Model) handleKeyQuitConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		return m.confirmQuit()
	case "esc", "n", "N", "left", "h":
		m.currentScreen = m.quitReturnScreen
		return m, nil
	case "y", "Y", "right", "l", "enter", " ":
		return m.confirmQuit()
	case "up", "k":
		m.quitConfirmIdx = 0
	case "down", "j":
		m.quitConfirmIdx = 1
	case "tab":
		m.quitConfirmIdx = 1 - m.quitConfirmIdx
	}
	return m, nil
}

func (m Model) confirmQuit() (tea.Model, tea.Cmd) {
	m.stopActiveExecution()
	return m, tea.Quit
}

func (m Model) viewQuitConfirm() string {
	modalW := m.modalWidth(52)

	var lines []string
	lines = append(lines, styleSectionTitle.Render("Quit HyperFleet E2E?"), "")
	if m.hasActiveExecution() {
		lines = append(lines, styleFail.Render("Running tests will be stopped."), "")
	}
	lines = append(lines, styleMuted.Render("Exit the application?"), "")
	lines = append(lines,
		m.quitConfirmOption(0, "Yes, quit"),
		m.quitConfirmOption(1, "No, stay"),
		"",
		styleFaint.Render("y/enter:yes  n/esc:no  tab:toggle"),
	)

	return m.modalScreen(modalBox(joinModalLines(lines...), modalW))
}

func (m Model) quitConfirmOption(idx int, label string) string {
	mark := styleFaint.Render("○")
	if m.quitConfirmIdx == idx {
		mark = styleAccent.Render("◉")
	}
	line := "  " + mark + "  " + label
	if m.quitConfirmIdx == idx {
		return styleSelected.Render(line)
	}
	return line
}

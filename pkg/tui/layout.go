package tui

// panelID identifies a clickable/focusable region in the main screen.
type panelID int

const (
	panelNone panelID = iota
	panelExec
	panelEnv
	panelTests
	panelLog
)

const (
	headerRows = 1
	footerRows = 1
)

func (m Model) bodyTopY() int { return headerRows }

// panelAt returns which panel contains terminal cell (x, y), or panelNone.
func (m Model) panelAt(x, y int) panelID {
	if y < m.bodyTopY() || y >= m.bodyTopY()+m.contentHeight() {
		return panelNone
	}
	bodyY := y - m.bodyTopY()

	if x < sidebarWidth {
		if bodyY < m.execListHeight() {
			return panelExec
		}
		return panelEnv
	}
	if x == sidebarWidth {
		return panelNone
	}
	if bodyY < m.testListHeight() {
		return panelTests
	}
	return panelLog
}

func (m Model) isLogFocused() bool {
	return m.pane == paneRight && m.rightPane == rightLog
}

func (m Model) execScrollStart() int {
	visible := m.execListVisibleRows()
	if len(m.executions) > 0 && m.execCursor >= visible {
		return m.execCursor - visible + 1
	}
	return 0
}

func (m Model) execListVisibleRows() int {
	visible := m.execListHeight() - 2
	if visible < 1 {
		return 1
	}
	return visible
}

func (m Model) envScrollStart() int {
	visible := m.envPaneHeight() - 1
	if visible < 1 {
		return 1
	}
	if m.envCursor >= visible {
		return m.envCursor - visible + 1
	}
	return 0
}

func (m Model) testScrollStart() int {
	visible := m.testListHeight() - 2
	if visible < 1 {
		return 1
	}
	if m.testCursor >= visible {
		return m.testCursor - visible + 1
	}
	return 0
}

// execIndexAtRow maps a row within the executions panel (0 = title) to an execution index.
func (m Model) execIndexAtRow(rowInPane int) (int, bool) {
	if rowInPane <= 0 {
		return 0, false
	}
	listRow := rowInPane - 1
	visible := m.execListVisibleRows()
	start := m.execScrollStart()
	if listRow >= 0 && listRow < visible && start+listRow < len(m.executions) {
		return start + listRow, true
	}
	return 0, false
}

func (m Model) envIndexAtRow(rowInPane int) (int, bool) {
	if rowInPane <= 0 {
		return 0, false
	}
	listRow := rowInPane - 1
	visible := m.envPaneHeight() - 1
	if visible < 1 {
		visible = 1
	}
	filtered := m.filteredEnvVars()
	start := m.envScrollStart()
	if listRow >= 0 && listRow < visible && start+listRow < len(filtered) {
		return start + listRow, true
	}
	return 0, false
}

func (m Model) testIndexAtRow(rowInPane int) (int, bool) {
	if rowInPane <= 0 || rowInPane == 1 {
		return 0, false // title + column header
	}
	if m.viewingExec < 0 || m.viewingExec >= len(m.executions) {
		return 0, false
	}
	listRow := rowInPane - 2
	visible := m.testListHeight() - 2
	if visible < 1 {
		visible = 1
	}
	tests := m.executions[m.viewingExec].Tests
	start := m.testScrollStart()
	if listRow >= 0 && listRow < visible && start+listRow < len(tests) {
		return start + listRow, true
	}
	return 0, false
}

func (m Model) envRelY(bodyY int) int {
	return bodyY - m.execListHeight() - 1
}

// envListContentWidth is the usable width for env rows (accounts for focus accent bar).
func (m Model) envListContentWidth(panelW int) int {
	if m.isEnvPanelFocused() {
		return max(1, panelW-2)
	}
	return panelW
}

// testListContentWidth is the usable width for test rows (accounts for focus accent bar).
func (m Model) testListContentWidth(panelW int) int {
	if m.isTestsPanelFocused() {
		return max(1, panelW-2)
	}
	return panelW
}

// testListColumnWidths allocates horizontal space: suite name gets the remainder.
func testListColumnWidths(contentW int) (suiteW, statusW, durW int) {
	statusW = 10 // CANCELLED
	durW = 8     // e.g. 422m05s
	const overhead = 7 // "  ", icon, and column gaps
	suiteW = contentW - overhead - statusW - durW
	if suiteW < 12 {
		suiteW = 12
	}
	return suiteW, statusW, durW
}

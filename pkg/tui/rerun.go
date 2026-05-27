package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) requestRerunConfirm(indices []int, summary string) (tea.Model, tea.Cmd) {
	m.rerunReturnScreen = m.currentScreen
	m.rerunConfirmIdx = 1 // default: No
	m.rerunExecID = ""
	m.rerunIndices = append([]int(nil), indices...)
	m.rerunSummary = summary
	if e := m.viewingExecution(); e != nil {
		m.rerunExecID = e.ID
	}
	m.currentScreen = screenRerunConfirm
	return m, nil
}

func (m Model) handleKeyRerunConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "ctrl+c":
		m = m.requestQuit()
		return m, nil
	case "esc", "n", "N", "left", "h":
		m.currentScreen = m.rerunReturnScreen
		return m, nil
	case "y", "Y", "right", "l", "enter", " ":
		if m.rerunConfirmIdx == 0 {
			return m.confirmRerun()
		}
		m.currentScreen = m.rerunReturnScreen
		return m, nil
	case "up", "k":
		m.rerunConfirmIdx = 0
	case "down", "j":
		m.rerunConfirmIdx = 1
	case "tab":
		m.rerunConfirmIdx = 1 - m.rerunConfirmIdx
	}
	return m, nil
}

func (m Model) confirmRerun() (tea.Model, tea.Cmd) {
	e := m.findExec(m.rerunExecID)
	if e == nil || len(m.rerunIndices) == 0 {
		m.currentScreen = m.rerunReturnScreen
		return m, nil
	}
	m.currentScreen = screenMain
	m = m.startRerun(e, m.rerunIndices)
	return m, nil
}

func (m Model) startRerun(source *Execution, indices []int) Model {
	if m.hasActiveExecution() {
		m.statusMsg = "Cannot re-run: a run is in progress (s to stop)"
		return m
	}

	parallelism := 1
	if len(indices) > 1 {
		parallelism = source.Filter.Parallelism
		if parallelism < 1 {
			parallelism = 1
		}
	}

	execID := fmt.Sprintf("%d", time.Now().UnixMilli())
	logDir := m.store.LogDir(execID)
	_ = os.MkdirAll(logDir, 0o750)

	var tests []*TestRun
	for _, idx := range indices {
		if idx < 0 || idx >= len(source.Tests) {
			continue
		}
		src := source.Tests[idx]
		suite := m.healSuiteFocus(src.Suite, source)
		logFile := filepath.Join(logDir, sanitizeFilename(suite.Name)+".log")
		tests = append(tests, &TestRun{
			Suite:   suite,
			Status:  StatusPending,
			LogFile: logFile,
		})
	}
	if len(tests) == 0 {
		return m
	}

	e := &Execution{
		ID:        execID,
		StartedAt: time.Now(),
		Filter:    source.Filter,
		Tests:     tests,
		Status:    StatusRunning,
		LogDir:    logDir,
	}

	m.executions = append([]*Execution{e}, m.executions...)
	m.execCursor = 0
	m.viewingExec = 0
	m.testCursor = 0
	m.resetLogView()

	if m.store != nil {
		_ = m.store.Save(e)
	}

	runIndices := make([]int, len(tests))
	for i := range tests {
		runIndices[i] = i
	}
	m.launchTestRunner(e, runIndices, parallelism, source.Filter.LabelFilter)
	m.statusMsg = "Re-running tests…"
	return m
}

func (m Model) handleRerunSingle() (tea.Model, tea.Cmd) {
	if !m.canRerunTests() {
		if m.hasActiveExecution() {
			m.statusMsg = "Cannot re-run while tests are running (s to stop)"
		}
		return m, nil
	}
	e := m.viewingExecution()
	if m.testCursor < 0 || m.testCursor >= len(e.Tests) {
		return m, nil
	}
	t := e.Tests[m.testCursor]
	summary := "Start new execution with this test?\n\n  " + truncate(t.Suite.Name, 52)
	return m.requestRerunConfirm([]int{m.testCursor}, summary)
}

func (m Model) handleRerunAllFailed() (tea.Model, tea.Cmd) {
	if !m.canRerunTests() {
		if m.hasActiveExecution() {
			m.statusMsg = "Cannot re-run while tests are running (s to stop)"
		}
		return m, nil
	}
	e := m.viewingExecution()
	var indices []int
	for i, t := range e.Tests {
		if t.Status == StatusFailed {
			indices = append(indices, i)
		}
	}
	if len(indices) == 0 {
		m.statusMsg = "No failing tests to re-run"
		return m, nil
	}
	summary := fmt.Sprintf("Start new execution with %d failing test(s)?", len(indices))
	return m.requestRerunConfirm(indices, summary)
}

func (m Model) viewRerunConfirm() string {
	modalW := m.modalWidth(52)

	var lines []string
	lines = append(lines, styleSectionTitle.Render("New execution"), "")
	for _, part := range strings.Split(m.rerunSummary, "\n") {
		lines = append(lines, styleMuted.Render(part))
	}
	lines = append(lines, "",
		m.rerunConfirmOption(0, "Yes, re-run"),
		m.rerunConfirmOption(1, "No, cancel"),
		"",
		styleFaint.Render("y/enter:yes  n/esc:no  tab:toggle"),
	)

	return m.modalScreen(modalBox(joinModalLines(lines...), modalW))
}

func (m Model) rerunConfirmOption(idx int, label string) string {
	mark := styleFaint.Render("○")
	if m.rerunConfirmIdx == idx {
		mark = styleAccent.Render("◉")
	}
	line := "  " + mark + "  " + label
	if m.rerunConfirmIdx == idx {
		return styleSelected.Render(line)
	}
	return line
}

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func renderEnvVarRow(key, value string, w int, focused bool) string {
	plain := truncate(fmt.Sprintf(" %s=%s", key, value), w)
	line := colorizeEnvPlainLine(plain)
	if focused {
		return styleFocused.Render(line)
	}
	return line
}

func colorizeEnvPlainLine(plain string) string {
	eq := strings.Index(plain, "=")
	if eq <= 1 {
		return plain
	}
	return plain[:1] +
		styleEnvKey.Render(plain[1:eq]) +
		styleFaint.Render("=") +
		styleEnvValue.Render(plain[eq+1:])
}

func (m Model) isExecPanelFocused() bool {
	return m.currentScreen == screenMain && m.pane == paneLeft && m.leftPane == leftExec
}

func (m Model) isEnvPanelFocused() bool {
	return m.currentScreen == screenMain && m.pane == paneLeft && m.leftPane == leftEnv
}

func (m Model) isTestsPanelFocused() bool {
	return m.currentScreen == screenMain && m.pane == paneRight && m.rightPane == rightTests
}

func (m Model) viewingExecution() *Execution {
	if m.viewingExec < 0 || m.viewingExec >= len(m.executions) {
		return nil
	}
	return m.executions[m.viewingExec]
}

// executionTestsFinished is true when no test is still pending or running.
func executionTestsFinished(e *Execution) bool {
	if e == nil || len(e.Tests) == 0 {
		return false
	}
	for _, t := range e.Tests {
		if t.Status == StatusPending || t.Status == StatusRunning {
			return false
		}
	}
	return true
}

func (m Model) canRerunTests() bool {
	if !m.isTestsPanelFocused() || m.hasActiveExecution() {
		return false
	}
	return executionTestsFinished(m.viewingExecution())
}

func panelSectionTitle(label string, focused bool) string {
	if focused {
		return stylePanelTitleActive.Render("▸ " + label)
	}
	return styleSectionTitle.Render(label)
}

// applyPanelFocus pads content to h lines and adds a left accent bar when focused.
func applyPanelFocus(focused bool, w, h int, content string) string {
	innerW := w
	if focused {
		innerW = w - 2
		if innerW < 1 {
			innerW = 1
		}
	}
	body := padHeight(content, h, innerW)
	if !focused {
		return body
	}
	marker := stylePanelAccent.Render("▌")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = lipgloss.JoinHorizontal(lipgloss.Top, marker, " ", line)
	}
	return strings.Join(lines, "\n")
}

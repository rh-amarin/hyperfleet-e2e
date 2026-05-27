package tui

import (
	"regexp"
	"time"
)

// activeExecution returns the execution started in this TUI session, if still running.
func (m Model) activeExecution() *Execution {
	if m.runExecID == "" {
		return nil
	}
	return m.findExec(m.runExecID)
}

// hasActiveExecution is true only while a runner goroutine is active (not for stale history).
func (m Model) hasActiveExecution() bool {
	return m.runCancel != nil
}

// stopActiveExecution cancels the in-flight test runner for the active execution.
func (m *Model) stopActiveExecution() {
	if m.runCancel != nil {
		m.runCancel()
	}
}

// finalizeExecution marks incomplete tests and sets execution status after a run ends.
func finalizeExecution(e *Execution, cancelled bool) {
	now := e.EndedAt
	if now.IsZero() {
		now = time.Now()
	}
	for _, t := range e.Tests {
		switch t.Status {
		case StatusPending, StatusRunning:
			t.Status = StatusStopped
			if t.EndAt.IsZero() {
				t.EndAt = now
			}
		}
	}

	if cancelled {
		e.Status = StatusCancelled
		return
	}

	passed := true
	for _, t := range e.Tests {
		if t.Status == StatusFailed || t.Status == StatusStopped {
			passed = false
			break
		}
	}
	if passed {
		e.Status = StatusPassed
	} else {
		e.Status = StatusFailed
	}
}

// ginkgoFocus returns the --focus regex passed to hyperfleet-e2e test.
// Tier0 catalog entries set Focus explicitly; discovered Describe names contain
// brackets that must be quoted when used as a literal focus.
func ginkgoFocus(s TestSuite) string {
	if s.Focus != "" {
		return s.Focus
	}
	return regexp.QuoteMeta(s.Name)
}

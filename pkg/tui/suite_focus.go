package tui

import (
	"os"
	"strings"
)

// healSuiteFocus restores the ginkgo --focus regex when it was lost (e.g. history reload).
func (m *Model) healSuiteFocus(s TestSuite, exec *Execution) TestSuite {
	if s.Focus != "" {
		return s
	}

	for _, c := range Tier0Catalog {
		if c.Name == s.Name {
			s.Focus = c.Focus
			if len(s.Labels) == 0 && len(c.Labels) > 0 {
				s.Labels = append([]string(nil), c.Labels...)
			}
			return s
		}
	}

	for _, known := range m.suites {
		if known.Name == s.Name {
			if known.Focus != "" {
				s.Focus = known.Focus
			}
			return s
		}
	}

	if exec != nil && exec.Filter.Focus != "" {
		s.Focus = exec.Filter.Focus
	}
	return s
}

// logIndicatesNoSpecsRan reports whether a ginkgo run matched zero specs (exits 0 anyway).
func logIndicatesNoSpecsRan(logFile string) bool {
	data, err := os.ReadFile(logFile) //nolint:gosec
	if err != nil {
		return false
	}
	lower := strings.ToLower(string(data))
	return strings.Contains(lower, "will run 0 of") ||
		strings.Contains(lower, "no specs to run") ||
		strings.Contains(lower, "found no specs")
}

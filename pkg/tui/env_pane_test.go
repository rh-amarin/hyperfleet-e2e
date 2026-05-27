package tui

import (
	"regexp"
	"strings"
	"testing"
)

func stripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`).ReplaceAllString(s, "")
}

func envPaneDataLines(t *testing.T, m Model) []string {
	t.Helper()
	out := m.viewEnvPane(sidebarWidth)
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least title + one env row, got %d lines", len(lines))
	}
	return lines[1:]
}

func TestViewEnvPaneRowAlignmentUnfocused(t *testing.T) {
	m := NewModel("", "e2e", ".", nil, nil)
	m.width = 120
	m.height = 40
	m.pane = paneLeft
	m.leftPane = leftEnv

	var wantPrefix string
	for i, line := range envPaneDataLines(t, m) {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "(none)") {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		prefixLen := len(line) - len(trimmed)
		prefix := line[:prefixLen]
		if i == 1 || wantPrefix == "" {
			wantPrefix = prefix
		}
		if prefix != wantPrefix {
			t.Fatalf("line %d prefix %q differs from first row prefix %q: %q", i, prefix, wantPrefix, line)
		}
	}
}

func TestViewEnvPaneRowAlignmentFocused(t *testing.T) {
	m := NewModel("", "e2e", ".", nil, nil)
	m.width = 120
	m.height = 40
	m.pane = paneLeft
	m.leftPane = leftEnv
	m.envCursor = 2

	var wantPrefix string
	for i, line := range envPaneDataLines(t, m) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		prefixLen := len(line) - len(trimmed)
		prefix := line[:prefixLen]
		if wantPrefix == "" {
			wantPrefix = prefix
		}
		if prefix != wantPrefix {
			t.Fatalf("line %d prefix %q differs from first row prefix %q: %q", i, prefix, wantPrefix, line)
		}
	}
}

package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMouseWheelScrollsEnvPanel(t *testing.T) {
	m := NewModel("", "e2e", ".", nil, nil)
	m.width = 120
	m.height = 40
	m.pane = paneLeft
	m.leftPane = leftEnv
	m.envCursor = 0

	// Y in env panel body (below exec list).
	bodyY := m.bodyTopY() + m.execListHeight() + 2
	x := sidebarWidth / 2

	m, _ = m.handleMouse(tea.MouseMsg{
		X:      x,
		Y:      bodyY,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if m.envCursor != 1 {
		t.Fatalf("envCursor = %d, want 1 after wheel down", m.envCursor)
	}
	if m.leftPane != leftEnv {
		t.Fatalf("leftPane = %v, want leftEnv", m.leftPane)
	}
}

func TestMouseWheelUpMovesEnvCursorUp(t *testing.T) {
	m := NewModel("", "e2e", ".", nil, nil)
	m.width = 120
	m.height = 40
	m.pane = paneLeft
	m.leftPane = leftEnv
	m.envCursor = 3

	bodyY := m.bodyTopY() + m.execListHeight() + 2
	m, _ = m.handleMouse(tea.MouseMsg{
		X:      sidebarWidth / 2,
		Y:      bodyY,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})

	if m.envCursor != 2 {
		t.Fatalf("envCursor = %d, want 2 after wheel up", m.envCursor)
	}
}

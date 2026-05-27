package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func envFilterModel() Model {
	return Model{
		pane:         paneLeft,
		leftPane:     leftEnv,
		envFiltering: true,
		envVars:      knownEnvVars,
	}
}

func TestEnvFilterTypingDoesNotQuit(t *testing.T) {
	m := envFilterModel()
	next, _ := m.handleKeyMain(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(Model)

	if m.envFilter != "q" {
		t.Fatalf("envFilter = %q, want %q", m.envFilter, "q")
	}
	if m.currentScreen == screenQuitConfirm {
		t.Fatal("typing q during filter should not open quit dialog")
	}
}

func TestQuitFromMainWindowWithQ(t *testing.T) {
	m := Model{currentScreen: screenMain}
	next, _ := m.handleKeyMain(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(Model)

	if m.currentScreen != screenQuitConfirm {
		t.Fatalf("currentScreen = %v, want screenQuitConfirm", m.currentScreen)
	}
}

func TestEditEnvTypingDoesNotQuit(t *testing.T) {
	m := Model{
		currentScreen: screenEditEnv,
		editInput:     textinput.New(),
	}
	next, _ := m.handleKeyEditEnv(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(Model)

	if m.currentScreen == screenQuitConfirm {
		t.Fatal("typing q in edit env dialog should not open quit dialog")
	}
}

func TestNewExecCustomFocusTypingDoesNotQuit(t *testing.T) {
	m := Model{
		currentScreen: screenNewExec,
		newExec: newExecModel{
			step:        newExecStepCustom,
			customInput: textinput.New(),
		},
	}
	m.newExec.customInput.Focus()
	next, _ := m.handleKeyNewExec(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(Model)

	if m.currentScreen == screenQuitConfirm {
		t.Fatal("typing q in new exec custom focus should not open quit dialog")
	}
}

func TestEnvFilterTypingDoesNotOpenEdit(t *testing.T) {
	m := envFilterModel()
	next, _ := m.handleKeyMain(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = next.(Model)

	if m.envFilter != "e" {
		t.Fatalf("envFilter = %q, want %q", m.envFilter, "e")
	}
	if m.currentScreen == screenEditEnv {
		t.Fatal("typing e during filter should not open edit dialog")
	}
}

func TestEnterOpensEnvEditWhenNotFiltering(t *testing.T) {
	m := Model{
		pane:     paneLeft,
		leftPane: leftEnv,
		envVars:  knownEnvVars,
	}
	m.editInput = textinput.New()
	next, _ := m.handleEnter()
	m = next.(Model)

	if m.currentScreen != screenEditEnv {
		t.Fatalf("currentScreen = %v, want screenEditEnv", m.currentScreen)
	}
}

func TestEnterExitsEnvFilterWithoutOpeningEdit(t *testing.T) {
	m := envFilterModel()
	m.envFilter = "api"
	next, _ := m.handleKeyMain(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)

	if m.envFiltering {
		t.Fatal("enter should exit env filter mode")
	}
	if m.envFilter != "api" {
		t.Fatalf("envFilter = %q, want %q", m.envFilter, "api")
	}
	if m.currentScreen == screenEditEnv {
		t.Fatal("enter during filter should not open edit dialog")
	}
}

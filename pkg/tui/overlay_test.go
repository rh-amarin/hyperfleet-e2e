package tui

import (
	"strings"
	"testing"
)

func TestWrapText(t *testing.T) {
	lines := wrapText("abcdef", 3)
	if len(lines) != 2 || lines[0] != "abc" || lines[1] != "def" {
		t.Fatalf("wrapText: got %q", lines)
	}
}

func TestViewEditEnvShowsFullKey(t *testing.T) {
	longKey := "ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING"
	m := Model{
		width:      100,
		height:     30,
		editEnvIdx: 0,
		envVars: []EnvVar{{
			Key:     longKey,
			Value:   "http://localhost:8000",
			Default: "amqp://guest:guest@rabbitmq.rabbitmq:5672",
		}},
	}
	m.editInput.Width = m.editEnvInputWidth()

	out := m.viewEditEnv()
	if !strings.Contains(out, longKey) {
		t.Fatalf("expected full key in modal output, got:\n%s", out)
	}
	if strings.Contains(out, longKey[:10]+"…") {
		t.Fatalf("key appears truncated in output:\n%s", out)
	}
}

func TestWideModalWidthUsesTerminal(t *testing.T) {
	m := Model{width: 80, height: 24}
	if w := m.wideModalWidth(); w != 74 {
		t.Fatalf("wideModalWidth(80) = %d, want 74", w)
	}
	m.width = 120
	if w := m.wideModalWidth(); w != 110 {
		t.Fatalf("wideModalWidth(120) = %d, want 110", w)
	}
}

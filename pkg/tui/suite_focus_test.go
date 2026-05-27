package tui

import (
	"os"
	"testing"
)

func TestHealSuiteFocusTier0Catalog(t *testing.T) {
	m := Model{}
	s := TestSuite{Name: "nodepool-update"}
	healed := m.healSuiteFocus(s, nil)
	want := Tier0Catalog[9].Focus
	if healed.Focus != want {
		t.Fatalf("Focus = %q, want %q", healed.Focus, want)
	}
}

func TestHealSuiteFocusPreservesExisting(t *testing.T) {
	m := Model{}
	s := TestSuite{Name: "x", Focus: "custom-regex"}
	healed := m.healSuiteFocus(s, nil)
	if healed.Focus != "custom-regex" {
		t.Fatalf("Focus = %q, want custom-regex", healed.Focus)
	}
}

func TestHealSuiteFocusExecFilter(t *testing.T) {
	m := Model{}
	exec := &Execution{Filter: ExecFilter{Focus: `\[Suite: cluster\]`}}
	s := TestSuite{Name: "[Suite: cluster] Create"}
	healed := m.healSuiteFocus(s, exec)
	if healed.Focus != `\[Suite: cluster\]` {
		t.Fatalf("Focus = %q", healed.Focus)
	}
}

func TestLogIndicatesNoSpecsRan(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.log"
	if err := os.WriteFile(path, []byte("Will run 0 of 14 specs\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !logIndicatesNoSpecsRan(path) {
		t.Fatal("expected true for 0 specs log")
	}
}

package tui

import "testing"

const maestroNegativeDescribe = "[Suite: adapter][maestro-transport][negative] Adapter Framework - Maestro Transport Negative Scenarios"

func TestMatchSuitesByCustomFocusMaestroNegative(t *testing.T) {
	suites := []TestSuite{
		{Name: maestroNegativeDescribe},
		{Name: "[Suite: adapter][maestro-transport] Adapter Framework - Maestro Transportation Layer"},
	}

	matched := matchSuitesByCustomFocus("Maestro Negative", suites)
	if len(matched) != 1 {
		t.Fatalf("matched %d suites, want 1", len(matched))
	}
	if matched[0].Name != maestroNegativeDescribe {
		t.Fatalf("matched %q, want maestro negative describe", matched[0].Name)
	}
	if matched[0].Focus != "" {
		t.Fatalf("Focus = %q, want empty so ginkgo uses Describe name", matched[0].Focus)
	}
}

func TestMatchSuitesByCustomFocusRegex(t *testing.T) {
	suites := []TestSuite{{Name: maestroNegativeDescribe}}
	matched := matchSuitesByCustomFocus("Maestro Transport Negative", suites)
	if len(matched) != 1 {
		t.Fatalf("matched %d suites, want 1", len(matched))
	}
}

func TestGinkgoFocusFromCustomFocusWords(t *testing.T) {
	got := ginkgoFocusFromCustomFocus("Maestro Negative")
	want := "(?i)Maestro.*Negative"
	if got != want {
		t.Fatalf("ginkgoFocusFromCustomFocus = %q, want %q", got, want)
	}
}

func TestResolveSuitesCustomFocusUsesDescribeName(t *testing.T) {
	m := Model{
		suites: []TestSuite{{Name: maestroNegativeDescribe}},
	}
	got := m.resolveSuites("", "Maestro Negative")
	if len(got) != 1 {
		t.Fatalf("resolveSuites returned %d suites, want 1", len(got))
	}
	if got[0].Focus != "" {
		t.Fatalf("Focus = %q, want empty", got[0].Focus)
	}
	if ginkgoFocus(got[0]) == "Maestro Negative" {
		t.Fatal("ginkgo focus should use quoted Describe name, not raw custom focus")
	}
}

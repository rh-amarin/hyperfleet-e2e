package tui

import (
	"strings"
	"testing"
)

func TestRenderEnvVarRowColorsKeyAndValue(t *testing.T) {
	row := renderEnvVarRow("HYPERFLEET_API_URL", "http://localhost:8000", 80, false)

	if !strings.Contains(row, styleEnvKey.Render("HYPERFLEET_API_URL")) {
		t.Fatal("expected key to use styleEnvKey")
	}
	if !strings.Contains(row, styleEnvValue.Render("http://localhost:8000")) {
		t.Fatal("expected value to use styleEnvValue")
	}
}

func TestColorizeEnvPlainLineTruncated(t *testing.T) {
	plain := truncate(" ADAPTER_CHART_REPO=https://github.com/openshift-hyperfleet/hyperfleet-adapter.git", 20)
	row := colorizeEnvPlainLine(plain)

	if !strings.HasPrefix(plain, " ") {
		t.Fatalf("plain = %q, want leading space", plain)
	}
	if strings.Contains(row, styleEnvKey.Render("ADAPTER_CHART_REPO")) {
		return
	}
	t.Fatalf("expected colored truncated key in %q", row)
}

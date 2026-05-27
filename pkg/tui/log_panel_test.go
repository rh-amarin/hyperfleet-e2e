package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogContentFullFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	content := strings.Repeat("line\n", 100)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got := readLogContent(path)
	if !strings.Contains(got, "line") {
		t.Fatalf("expected full log, got len=%d", len(got))
	}
	if strings.Count(got, "line") < 100 {
		t.Fatalf("expected 100 lines, got %d", strings.Count(got, "line"))
	}
}

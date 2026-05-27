package tui

import "testing"

func TestTestListColumnWidthsUsesAvailableSpace(t *testing.T) {
	suiteW, statusW, durW := testListColumnWidths(81)
	if statusW != 10 || durW != 8 {
		t.Fatalf("statusW=%d durW=%d", statusW, durW)
	}
	// 81 - 7 - 10 - 8 = 56 (was hardcoded 46)
	if suiteW != 56 {
		t.Fatalf("suiteW=%d, want 56 for content width 81", suiteW)
	}
}

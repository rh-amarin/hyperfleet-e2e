package tui

import (
	"fmt"
	"io"
	"os"
	"strings"
)

const maxLogFileBytes = 16 << 20 // 16 MiB safety cap

// refreshLog reloads the full log file for the selected test. On open or refresh the
// viewport scrolls to the tail; users can scroll up to read from the start. While a test
// is running, the view follows the tail until the user scrolls away.
func (m *Model) refreshLog() {
	if m.viewingExec < 0 || m.viewingExec >= len(m.executions) {
		return
	}
	e := m.executions[m.viewingExec]
	if m.testCursor < 0 || m.testCursor >= len(e.Tests) {
		return
	}
	t := e.Tests[m.testCursor]
	if t.LogFile == "" {
		return
	}

	viewKey := fmt.Sprintf("%s:%d:%s", e.ID, m.testCursor, t.LogFile)
	switched := viewKey != m.logViewKey
	if switched {
		m.logViewKey = viewKey
		m.logFollowTail = true
	}

	content := readLogContent(t.LogFile)
	if content == "" {
		if m.logLastContent != "(no output yet)" || switched {
			m.logLastContent = "(no output yet)"
			m.logViewport.SetContent(m.logLastContent)
			if m.logFollowTail {
				m.logViewport.GotoBottom()
			}
		}
		return
	}

	if !switched && content == m.logLastContent {
		return
	}

	yOff := m.logViewport.YOffset
	m.logLastContent = content
	m.logViewport.SetContent(content)

	if m.logFollowTail {
		m.logViewport.GotoBottom()
	} else {
		m.logViewport.SetYOffset(yOff)
	}
}

func (m *Model) resetLogView() {
	m.logViewKey = ""
	m.logLastContent = ""
	m.logFollowTail = true
}

func readLogContent(path string) string {
	info, err := os.Stat(path) //nolint:gosec
	if err != nil {
		return ""
	}
	if info.Size() > maxLogFileBytes {
		return readLogTailBytes(path, maxLogFileBytes)
	}
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\n")
}

// readLogTailBytes returns the last maxBytes of a file with a leading notice.
func readLogTailBytes(path string, maxBytes int64) string {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return ""
	}
	defer f.Close()

	size := maxBytes
	if st, err := f.Stat(); err == nil && st.Size() < size {
		size = st.Size()
	}
	if _, err := f.Seek(-size, io.SeekEnd); err != nil {
		return ""
	}
	buf := make([]byte, size)
	n, _ := f.Read(buf)
	text := strings.TrimRight(string(buf[:n]), "\n")
	notice := fmt.Sprintf("… log truncated (showing last %d MiB) …", maxBytes/(1<<20))
	return notice + "\n" + text
}

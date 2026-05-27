package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// modalBoxStyle is shared so modalInnerWidth matches rendered borders.
var modalBoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorAccent).
	Background(colorModalBG).
	Foreground(colorText).
	Padding(1, 3)

// modalBox renders bordered modal content at a fixed width so borders stay intact.
func modalBox(content string, width int) string {
	if width < 20 {
		width = 20
	}
	return modalBoxStyle.Width(width).Render(content)
}

// modalInnerWidth is the usable text width inside modalBox for the given outer width.
func modalInnerWidth(outerW int) int {
	inner := outerW - modalBoxStyle.GetHorizontalFrameSize()
	if inner < 10 {
		return 10
	}
	return inner
}

// wideModalWidth returns a modal width that uses most of the terminal (min 48, max 110).
func (m Model) wideModalWidth() int {
	if m.width <= 0 {
		return 88
	}
	w := m.width - 6
	if w > 110 {
		w = 110
	}
	if w < 48 {
		w = 48
	}
	return w
}

func (m Model) editEnvModalWidth() int  { return m.wideModalWidth() }
func (m Model) newExecModalWidth() int  { return m.wideModalWidth() }
func (m Model) editEnvInputWidth() int  { return modalInnerWidth(m.editEnvModalWidth()) }
func (m Model) newExecInputWidth() int  { return modalInnerWidth(m.newExecModalWidth()) }

// modalLine fits a styled line inside the modal content area without breaking the border.
func modalLine(line string, innerW int) string {
	if innerW < 1 {
		return line
	}
	return lipgloss.NewStyle().Width(innerW).MaxWidth(innerW).Render(line)
}

// wrapText breaks plain text into lines of at most width runes.
func wrapText(text string, width int) []string {
	if text == "" {
		return nil
	}
	if width < 1 {
		return []string{text}
	}
	runes := []rune(text)
	var lines []string
	for len(runes) > width {
		lines = append(lines, string(runes[:width]))
		runes = runes[width:]
	}
	lines = append(lines, string(runes))
	return lines
}

// modalScreen centers a modal on a dimmed full-screen backdrop.
func (m Model) modalScreen(box string) string {
	if m.width < 1 {
		m.width = 1
	}
	if m.height < 1 {
		m.height = 1
	}
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
		lipgloss.WithWhitespaceChars("·"),
		lipgloss.WithWhitespaceForeground(colorModalDots),
	)
}

// modalWidth returns a sensible modal width for the current terminal.
func (m Model) modalWidth(preferred int) int {
	w := preferred
	if w > m.width-4 {
		w = m.width - 4
	}
	if w < 24 {
		w = 24
	}
	return w
}

// padModalLine truncates plain (unstyled) text to fit inside a modal content area.
// Do not pass lipgloss-styled strings — ANSI codes break rune counting.
func padModalLine(line string, innerWidth int) string {
	return truncate(line, innerWidth)
}

// joinModalLines joins lines ensuring no trailing issues with empty strings.
func joinModalLines(lines ...string) string {
	return strings.Join(lines, "\n")
}

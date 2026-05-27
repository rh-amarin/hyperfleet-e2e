package tui

import "github.com/charmbracelet/lipgloss"

// High-contrast palette for dark terminals (ttyd/browser often use pure black bg).
var (
	colorText      = lipgloss.Color("#c0caf5")
	colorMuted     = lipgloss.Color("#a9b1d6")
	colorFaint     = lipgloss.Color("#7a849e")
	colorAccent    = lipgloss.Color("#7dcfff")
	colorGreen     = lipgloss.Color("#9ece6a")
	colorRed       = lipgloss.Color("#f7768e")
	colorYellow    = lipgloss.Color("#e0af68")
	colorOrange    = lipgloss.Color("#ff9e64")
	colorBorder    = lipgloss.Color("#565f89")
	colorHeaderBG  = lipgloss.Color("#3d59a1")
	colorHeaderFG  = lipgloss.Color("#e8edf5")
	colorFocusBG   = lipgloss.Color("#3d5a80")
	colorPanelAccent = lipgloss.Color("#7aa2f7")
	colorSelectBG  = lipgloss.Color("#2a3150")
	colorModalBG   = lipgloss.Color("#1f2335")
	colorModalDots = lipgloss.Color("#4a5272")

	styleHeader = lipgloss.NewStyle().
			Background(colorHeaderBG).
			Foreground(colorHeaderFG).
			Bold(true)

	styleFooter = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(lipgloss.Color("#16161e"))

	styleSectionTitle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	stylePanelTitleActive = lipgloss.NewStyle().
				Foreground(colorPanelAccent).
				Bold(true).
				Underline(true)

	stylePanelAccent = lipgloss.NewStyle().
			Foreground(colorPanelAccent).
			Bold(true)

	styleNormal = lipgloss.NewStyle().
			Foreground(colorText)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleFaint = lipgloss.NewStyle().
			Foreground(colorFaint)

	// styleDim kept as alias for muted body text (legacy name in call sites).
	styleDim = styleMuted

	styleSelected = lipgloss.NewStyle().
			Background(colorSelectBG).
			Foreground(colorMuted)

	styleFocused = lipgloss.NewStyle().
			Background(colorFocusBG).
			Foreground(colorText).
			Bold(true)

	stylePass = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleFail = lipgloss.NewStyle().Foreground(colorRed).Bold(true)
	styleRun  = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	styleStop = lipgloss.NewStyle().Foreground(colorOrange).Bold(true)
	stylePend = lipgloss.NewStyle().Foreground(colorFaint)

	styleAccent = lipgloss.NewStyle().Foreground(colorAccent)

	styleEnvKey = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	styleEnvValue = lipgloss.NewStyle().
			Foreground(colorText)

	styleBorder = lipgloss.NewStyle().Foreground(colorBorder)
)

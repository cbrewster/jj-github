package components

import "github.com/charmbracelet/lipgloss"

// Graph characters for the revision stack (jj-inspired)
const (
	GraphTrunk      = "◆"
	GraphPending    = "○"
	GraphInProgress = "◉"
	GraphCurrent    = "●"
	GraphSuccess    = "✓"
	GraphError      = "✗"
	GraphLine       = "│"
)

// Colors
var (
	ColorMuted   = lipgloss.Color("8")
	ColorSuccess = lipgloss.Color("2")
	ColorError   = lipgloss.Color("1")
	ColorAccent  = lipgloss.Color("5")
	ColorYellow  = lipgloss.Color("3")
)

// Styles
var (
	// Title style for the app header
	TitleStyle = lipgloss.NewStyle().
			Bold(true)

	// Muted text style
	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Success text style
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	// Error text style
	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError)

	// Accent text style
	AccentStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	// Yellow text style (for in-progress)
	YellowStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	// Help text style
	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Change ID Short style
	ChangeIDShortStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	// Change ID Rest style
	ChangeIDRestStyle = lipgloss.NewStyle().
				Foreground(ColorMuted)

	// PR number style
	PRNumberStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Status message style (sub-status below revision)
	StatusMsgStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(3)
)

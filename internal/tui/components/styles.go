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
	ColorMuted   = lipgloss.AdaptiveColor{Light: "#888888", Dark: "#666666"}
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#22c55e", Dark: "#4ade80"}
	ColorError   = lipgloss.AdaptiveColor{Light: "#ef4444", Dark: "#f87171"}
	ColorAccent  = lipgloss.AdaptiveColor{Light: "#3b82f6", Dark: "#60a5fa"}
	ColorYellow  = lipgloss.AdaptiveColor{Light: "#eab308", Dark: "#facc15"}
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

	// Change ID style
	ChangeIDStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	// PR number style
	PRNumberStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Status message style (sub-status below revision)
	StatusMsgStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			PaddingLeft(3)
)

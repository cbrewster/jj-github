package components

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Spinner wraps the bubbles spinner with consistent styling
type Spinner struct {
	spinner spinner.Model
}

// NewSpinner creates a new spinner with the dot style
func NewSpinner() Spinner {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#eab308", Dark: "#facc15"})
	return Spinner{spinner: s}
}

// Update handles spinner tick messages
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return s, cmd
}

// View renders the spinner
func (s Spinner) View() string {
	return s.spinner.View()
}

// Tick returns the spinner's tick command
func (s Spinner) Tick() tea.Cmd {
	return s.spinner.Tick
}

package merge

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the key bindings for the merge TUI
type KeyMap struct {
	Merge key.Binding
	Quit  key.Binding
}

// ShortHelp returns key bindings for the short help view
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Merge, k.Quit}
}

// FullHelp returns key bindings for the full help view
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Merge, k.Quit},
	}
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Merge: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "merge"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

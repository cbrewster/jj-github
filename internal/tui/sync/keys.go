package sync

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines the key bindings for the sync TUI
type KeyMap struct {
	Quit key.Binding
}

// DefaultKeyMap returns the default key bindings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

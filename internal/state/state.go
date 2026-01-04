package state

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	stateFileName = "jj-github-state.json"
	stateVersion  = 1
)

// PRState represents the state of a PR.
type PRState string

const (
	PRStateOpen   PRState = "open"
	PRStateMerged PRState = "merged"
	PRStateClosed PRState = "closed"
)

// State represents the persisted state of jj-github.
type State struct {
	Version int                   `json:"version"`
	Entries map[string]StackEntry `json:"entries"` // keyed by change_id
	path    string                // path to the state file (not serialized)
}

// StackEntry represents a tracked PR for a change.
type StackEntry struct {
	PRNumber int     `json:"pr_number"`
	Branch   string  `json:"branch"`
	State    PRState `json:"state"`
	Title    string  `json:"title"`
}

// Load loads the state from .jj/jj-github-state.json.
// If the file doesn't exist, returns an empty state.
func Load() (*State, error) {
	jjRoot, err := getJJRoot()
	if err != nil {
		return nil, err
	}

	statePath := filepath.Join(jjRoot, ".jj", stateFileName)

	data, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No state file yet, return empty state
			return &State{
				Version: stateVersion,
				Entries: make(map[string]StackEntry),
				path:    statePath,
			}, nil
		}
		return nil, err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		// Corrupted state file, return empty state
		return &State{
			Version: stateVersion,
			Entries: make(map[string]StackEntry),
			path:    statePath,
		}, nil
	}

	state.path = statePath
	if state.Entries == nil {
		state.Entries = make(map[string]StackEntry)
	}

	return &state, nil
}

// Save persists the state to .jj/jj-github-state.json.
func (s *State) Save() error {
	if s.path == "" {
		return errors.New("state path not set")
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.path, data, 0644)
}

// GetByChangeID returns the entry for a change ID, or nil if not found.
func (s *State) GetByChangeID(changeID string) *StackEntry {
	if entry, ok := s.Entries[changeID]; ok {
		return &entry
	}
	return nil
}

// Set updates or creates an entry for a change ID.
func (s *State) Set(changeID string, entry StackEntry) {
	s.Entries[changeID] = entry
}

// Remove deletes an entry for a change ID.
func (s *State) Remove(changeID string) {
	delete(s.Entries, changeID)
}

// GetMergedPRs returns all entries with state=merged.
func (s *State) GetMergedPRs() map[string]StackEntry {
	merged := make(map[string]StackEntry)
	for changeID, entry := range s.Entries {
		if entry.State == PRStateMerged {
			merged[changeID] = entry
		}
	}
	return merged
}

// getJJRoot returns the root of the jj repository.
func getJJRoot() (string, error) {
	output, err := exec.Command("jj", "root").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

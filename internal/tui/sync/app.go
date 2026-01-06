package sync

import (
	"context"
	"fmt"
	"strings"

	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/tui/components"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Phase represents the current phase of the sync workflow
type Phase int

const (
	PhaseFetching Phase = iota
	PhaseUpToDate
	PhaseRebasing
	PhaseComplete
	PhaseError
)

// BookmarkState represents the state of a bookmark during sync
type BookmarkState int

const (
	StatePending BookmarkState = iota
	StateInProgress
	StateSuccess
	StateSkipped // Commit became empty and was abandoned (e.g., after squash-merge)
	StateConflict
	StateError
)

// BookmarkItem represents a bookmark being synced
type BookmarkItem struct {
	Bookmark jj.Bookmark
	State    BookmarkState
	Error    error
}

// Messages for async operations
type (
	FetchCompleteMsg struct {
		Bookmarks []jj.Bookmark
		TrunkName string
		Err       error
	}

	RebaseCompleteMsg struct {
		ChangeID     string
		HasConflict  bool
		SkippedEmpty bool
		Err          error
	}
)

// Model is the main bubbletea model for the sync TUI
type Model struct {
	// State
	phase     Phase
	bookmarks []BookmarkItem
	spinner   components.Spinner
	keys      KeyMap
	err       error
	width     int
	trunkName string

	// Progress tracking
	currentIndex  int
	successCount  int
	skippedCount  int
	conflictCount int

	// Dependencies
	ctx context.Context
}

// NewModel creates a new sync TUI model
func NewModel(ctx context.Context) Model {
	return Model{
		phase:   PhaseFetching,
		spinner: components.NewSpinner(),
		keys:    DefaultKeyMap(),
		ctx:     ctx,
	}
}

// Init initializes the model and starts the fetch operation
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick(),
		m.fetchCmd(),
	)
}

// Update handles messages and updates the model
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}

	case FetchCompleteMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		m.trunkName = msg.TrunkName

		if len(msg.Bookmarks) == 0 {
			m.phase = PhaseUpToDate
			return m, tea.Quit
		}

		// Initialize bookmark items
		m.bookmarks = make([]BookmarkItem, len(msg.Bookmarks))
		for i, b := range msg.Bookmarks {
			m.bookmarks[i] = BookmarkItem{
				Bookmark: b,
				State:    StatePending,
			}
		}

		// Start rebasing
		m.phase = PhaseRebasing
		m.currentIndex = 0
		return m, m.rebaseNextCmd()

	case RebaseCompleteMsg:
		// Find the bookmark and update its state
		for i := range m.bookmarks {
			if m.bookmarks[i].Bookmark.ChangeID == msg.ChangeID {
				if msg.Err != nil {
					m.bookmarks[i].State = StateError
					m.bookmarks[i].Error = msg.Err
				} else if msg.SkippedEmpty {
					m.bookmarks[i].State = StateSkipped
					m.skippedCount++
				} else if msg.HasConflict {
					m.bookmarks[i].State = StateConflict
					m.conflictCount++
				} else {
					m.bookmarks[i].State = StateSuccess
					m.successCount++
				}
				break
			}
		}

		m.currentIndex++

		if m.currentIndex < len(m.bookmarks) {
			return m, m.rebaseNextCmd()
		}

		// All done
		m.phase = PhaseComplete
		return m, tea.Quit
	}

	// Update spinner
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the UI
func (m Model) View() string {
	var sb strings.Builder

	switch m.phase {
	case PhaseFetching:
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Fetching from remote...\n")

	case PhaseUpToDate:
		sb.WriteString(components.SuccessStyle.Render(components.GraphSuccess))
		sb.WriteString(" Already up to date - no bookmarks to rebase.\n")

	case PhaseRebasing:
		sb.WriteString(fmt.Sprintf("Rebasing onto %s:\n\n", components.AccentStyle.Render(m.trunkName)))
		sb.WriteString(m.renderBookmarks())
		sb.WriteString("\n")

	case PhaseComplete:
		sb.WriteString(fmt.Sprintf("Rebased onto %s:\n\n", components.AccentStyle.Render(m.trunkName)))
		sb.WriteString(m.renderBookmarks())
		sb.WriteString("\n")
		sb.WriteString(m.renderSummary())
		sb.WriteString("\n")

	case PhaseError:
		sb.WriteString(components.ErrorStyle.Render(components.GraphError + " Sync failed"))
		sb.WriteString("\n\n")
		if m.err != nil {
			sb.WriteString(components.ErrorStyle.Render(m.err.Error()))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// idDisplayWidth is the number of extra characters we show after the short ID
const idDisplayExtra = 4

// renderBookmarks renders the list of bookmarks with their states
func (m Model) renderBookmarks() string {
	// Calculate max ID width for alignment
	maxIDWidth := 0
	for _, item := range m.bookmarks {
		idWidth := len(item.Bookmark.ShortID) + idDisplayExtra
		if idWidth > maxIDWidth {
			maxIDWidth = idWidth
		}
	}

	var sb strings.Builder
	for _, item := range m.bookmarks {
		sb.WriteString(m.renderBookmarkItem(item, maxIDWidth))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderBookmarkItem renders a single bookmark item with fixed-width ID column
func (m Model) renderBookmarkItem(item BookmarkItem, idWidth int) string {
	var sb strings.Builder

	// Graph symbol based on state
	switch item.State {
	case StatePending:
		sb.WriteString(components.MutedStyle.Render(components.GraphPending))
	case StateInProgress:
		sb.WriteString(components.YellowStyle.Render(m.spinner.View()))
	case StateSuccess:
		sb.WriteString(components.SuccessStyle.Render(components.GraphSuccess))
	case StateSkipped:
		sb.WriteString(components.MutedStyle.Render(components.GraphSuccess))
	case StateConflict:
		sb.WriteString(components.ErrorStyle.Render(components.GraphError))
	case StateError:
		sb.WriteString(components.ErrorStyle.Render(components.GraphError))
	}

	sb.WriteString(" ")

	// Change ID (short part highlighted, rest muted) with padding for alignment
	shortID := item.Bookmark.ShortID
	fullID := item.Bookmark.ChangeID
	var idStr string
	if len(fullID) > len(shortID) {
		idStr = shortID + fullID[len(shortID):len(shortID)+idDisplayExtra]
	} else {
		idStr = shortID
	}

	// Pad the ID to fixed width
	padding := idWidth - len(idStr)
	if padding < 0 {
		padding = 0
	}

	sb.WriteString(components.ChangeIDShortStyle.Render(shortID))
	if len(fullID) > len(shortID) {
		sb.WriteString(components.ChangeIDRestStyle.Render(fullID[len(shortID) : len(shortID)+idDisplayExtra]))
	}
	sb.WriteString(strings.Repeat(" ", padding))

	sb.WriteString(" ")

	// Description (first line only)
	description := item.Bookmark.Description
	if idx := strings.Index(description, "\n"); idx != -1 {
		description = description[:idx]
	}
	if description == "" {
		description = "(no description)"
		sb.WriteString(components.MutedStyle.Render(description))
	} else {
		sb.WriteString(description)
	}

	// Status suffix
	switch item.State {
	case StateInProgress:
		sb.WriteString(components.MutedStyle.Render("  Rebasing..."))
	case StateSkipped:
		sb.WriteString(components.MutedStyle.Render("  skipped (already in trunk)"))
	case StateConflict:
		sb.WriteString(components.YellowStyle.Render("  conflict"))
	case StateError:
		if item.Error != nil {
			sb.WriteString(components.ErrorStyle.Render("  " + item.Error.Error()))
		}
	}

	return sb.String()
}

// renderSummary renders the completion summary
func (m Model) renderSummary() string {
	if m.conflictCount == 0 && m.skippedCount == 0 {
		return components.SuccessStyle.Render(fmt.Sprintf("%d stack(s) rebased successfully.", m.successCount))
	}

	var parts []string
	if m.successCount > 0 {
		parts = append(parts, fmt.Sprintf("%d rebased", m.successCount))
	}
	if m.skippedCount > 0 {
		parts = append(parts, components.MutedStyle.Render(fmt.Sprintf("%d skipped", m.skippedCount)))
	}
	if m.conflictCount > 0 {
		parts = append(parts, components.YellowStyle.Render(fmt.Sprintf("%d conflict(s)", m.conflictCount)))
	}

	summary := strings.Join(parts, ", ")
	if m.conflictCount > 0 {
		summary += "\n" + components.MutedStyle.Render("Run `jj resolve` to fix conflicts.")
	}
	return summary
}

// Commands

func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		// Fetch from remote
		if err := jj.GitFetch(); err != nil {
			return FetchCompleteMsg{Err: fmt.Errorf("git fetch: %w", err)}
		}

		// Get trunk name
		trunkName, err := jj.GetTrunkName()
		if err != nil {
			return FetchCompleteMsg{Err: fmt.Errorf("get trunk name: %w", err)}
		}

		// Get stack roots that need rebasing onto current trunk
		bookmarks, err := jj.GetStackRootsToRebase()
		if err != nil {
			return FetchCompleteMsg{Err: err}
		}

		return FetchCompleteMsg{
			Bookmarks: bookmarks,
			TrunkName: trunkName,
		}
	}
}

func (m Model) rebaseNextCmd() tea.Cmd {
	if m.currentIndex >= len(m.bookmarks) {
		return nil
	}

	item := &m.bookmarks[m.currentIndex]
	item.State = StateInProgress
	changeID := item.Bookmark.ChangeID

	return func() tea.Msg {
		result, err := jj.Rebase(changeID, "trunk()")
		return RebaseCompleteMsg{
			ChangeID:     changeID,
			HasConflict:  result.HasConflict,
			SkippedEmpty: result.SkippedEmpty,
			Err:          err,
		}
	}
}

package merge

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/tui/components"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gogithub "github.com/google/go-github/v80/github"
)

// Phase represents the current phase of the merge workflow
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseConfirmation
	PhaseSyncing
	PhaseWaitingForMergeable
	PhaseMerging
	PhaseSyncingAfterMerge
	PhaseComplete
	PhaseError
)

// pollInterval is how often we check if a PR is mergeable
const pollInterval = 5 * time.Second

// Help separator between key bindings
const helpSeparator = " • "

// Messages for async operations
type (
	LoadCompleteMsg struct {
		Changes     []jj.Change
		TrunkName   string
		ExistingPRs map[string]*gogithub.PullRequest
		Err         error
	}

	SyncCompleteMsg struct {
		HasConflict bool
		Err         error
	}

	MergeableCheckMsg struct {
		PRNumber   int
		Mergeable  bool
		MergeState string
		Err        error
	}

	MergeCompleteMsg struct {
		PRNumber int
		Err      error
	}
)

// Model is the main bubbletea model for the merge TUI
type Model struct {
	// State
	phase     Phase
	stack     components.Stack
	spinner   components.Spinner
	keys      KeyMap
	err       error
	width     int
	trunkName string

	// Merge progress tracking
	currentIndex int // Index into mutableRevisions (from bottom, so 0 = first to merge)
	mergedCount  int

	// Options
	noWait bool

	// Dependencies
	ctx         context.Context
	gh          *github.Client
	repo        github.Repo
	revset      string
	existingPRs map[string]*gogithub.PullRequest
	changes     []jj.Change
}

// NewModel creates a new merge TUI model
func NewModel(ctx context.Context, gh *github.Client, repo github.Repo, revset string, noWait bool) Model {
	return Model{
		phase:       PhaseLoading,
		spinner:     components.NewSpinner(),
		keys:        DefaultKeyMap(),
		ctx:         ctx,
		gh:          gh,
		repo:        repo,
		revset:      revset,
		noWait:      noWait,
		existingPRs: make(map[string]*gogithub.PullRequest),
	}
}

// Init initializes the model and starts loading
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick(),
		m.loadCmd(),
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
		case key.Matches(msg, m.keys.Merge) && m.phase == PhaseConfirmation:
			// Start the merge process
			m.phase = PhaseSyncing
			return m, m.syncCmd()
		}

	case LoadCompleteMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		m.changes = msg.Changes
		m.trunkName = msg.TrunkName
		m.existingPRs = msg.ExistingPRs
		m.stack = components.NewStack(msg.Changes, msg.TrunkName)

		// Set PR numbers on stack
		for i := range m.stack.Revisions {
			rev := &m.stack.Revisions[i]
			if rev.IsImmutable {
				continue
			}
			if pr, ok := m.existingPRs[rev.Change.GitPushBookmark]; ok {
				rev.PRNumber = pr.GetNumber()
			}
		}

		// Check that all revisions have PRs
		mutableRevs := m.stack.MutableRevisions()
		for _, rev := range mutableRevs {
			if rev.PRNumber == 0 {
				m.phase = PhaseError
				m.err = fmt.Errorf("revision %s has no PR - run 'jj-github submit' first", rev.Change.ShortID)
				return m, tea.Quit
			}
		}

		if len(mutableRevs) == 0 {
			m.phase = PhaseError
			m.err = fmt.Errorf("no revisions to merge")
			return m, tea.Quit
		}

		// Show confirmation before starting
		m.phase = PhaseConfirmation
		return m, nil

	case SyncCompleteMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		if msg.HasConflict {
			m.phase = PhaseError
			m.err = fmt.Errorf("sync resulted in conflicts - resolve with 'jj resolve' before merging")
			return m, tea.Quit
		}

		// After sync, check if we were syncing after a merge or initial sync
		if m.mergedCount > 0 && m.currentIndex < len(m.stack.MutableRevisions()) {
			// Continue to next PR
			return m, m.checkMergeableCmd()
		}

		// Initial sync complete, start merging from bottom
		m.currentIndex = 0
		return m, m.checkMergeableCmd()

	case MergeableCheckMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		mutableRevs := m.stack.MutableRevisions()
		// Revisions are in display order (current at top), we merge from bottom
		revIdx := len(mutableRevs) - 1 - m.currentIndex
		rev := mutableRevs[revIdx]

		if msg.Mergeable {
			// Ready to merge
			m.phase = PhaseMerging
			m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, "Merging...")
			return m, m.mergeCmd(msg.PRNumber)
		}

		// Not mergeable
		if m.noWait {
			m.phase = PhaseError
			m.err = fmt.Errorf("PR #%d is not mergeable (state: %s) - use without --no-wait to wait", msg.PRNumber, msg.MergeState)
			return m, tea.Quit
		}

		// Wait and poll again
		m.phase = PhaseWaitingForMergeable
		m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, fmt.Sprintf("Waiting for PR to be mergeable (%s)...", msg.MergeState))
		return m, tea.Tick(pollInterval, func(t time.Time) tea.Msg {
			return m.checkMergeableCmd()()
		})

	case MergeCompleteMsg:
		mutableRevs := m.stack.MutableRevisions()
		revIdx := len(mutableRevs) - 1 - m.currentIndex
		rev := mutableRevs[revIdx]

		if msg.Err != nil {
			// Check if this is an "out of date" error - if so, sync and retry
			errStr := msg.Err.Error()
			if strings.Contains(errStr, "out of date") || strings.Contains(errStr, "Head branch was modified") {
				// Need to sync (rebase + push) and retry
				m.phase = PhaseSyncingAfterMerge
				m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, "Branch out of date, syncing...")
				return m, m.syncCmd()
			}

			m.phase = PhaseError
			m.err = fmt.Errorf("failed to merge PR #%d: %w", msg.PRNumber, msg.Err)
			return m, tea.Quit
		}

		m.stack.SetRevisionState(rev.Change.ID, components.StateSuccess, "")
		m.mergedCount++
		m.currentIndex++

		if m.currentIndex >= len(mutableRevs) {
			// All done!
			m.phase = PhaseComplete
			return m, tea.Quit
		}

		// Sync before merging next PR (rebase remaining onto updated trunk)
		m.phase = PhaseSyncingAfterMerge
		return m, m.syncCmd()
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

	width := m.width
	if width == 0 {
		width = 80
	}

	viewOpts := components.ViewOptions{
		RepoOwner: m.repo.Owner,
		RepoName:  m.repo.Name,
		Width:     width,
	}

	switch m.phase {
	case PhaseLoading:
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Loading stack and PRs...\n")

	case PhaseConfirmation:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("\n")
		count := len(m.stack.MutableRevisions())
		fmt.Fprintf(&sb, "%d PR(s) will be merged (bottom to top).\n\n", count)
		sb.WriteString(renderHelp(m.keys))
		sb.WriteString("\n")

	case PhaseSyncing:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Syncing with remote (rebasing and pushing)...\n")

	case PhaseWaitingForMergeable:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("\n")

	case PhaseMerging:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("\n")

	case PhaseSyncingAfterMerge:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Rebasing and pushing remaining PRs onto updated trunk...\n")

	case PhaseComplete:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString(components.SuccessStyle.Render(fmt.Sprintf("Successfully merged %d PR(s)!", m.mergedCount)))
		sb.WriteString("\n")

	case PhaseError:
		if len(m.stack.Revisions) > 0 {
			sb.WriteString(m.stack.View(m.spinner, viewOpts))
		}
		sb.WriteString(components.ErrorStyle.Render("Merge failed"))
		sb.WriteString("\n\n")
		if m.err != nil {
			sb.WriteString(components.ErrorStyle.Render(m.err.Error()))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Commands

func (m Model) loadCmd() tea.Cmd {
	return func() tea.Msg {
		// Fetch from remote
		if err := jj.GitFetch(); err != nil {
			return LoadCompleteMsg{Err: fmt.Errorf("git fetch: %w", err)}
		}

		// Load revision stack with base branch info
		revInfo, err := jj.GetStackInfo(m.revset)
		if err != nil {
			return LoadCompleteMsg{Err: err}
		}
		changes := revInfo.Changes
		trunkName := revInfo.TrunkName

		// Collect branches for mutable changes
		var branches []string
		for _, change := range changes {
			if !change.Immutable && change.Description != "" {
				branches = append(branches, change.GitPushBookmark)
			}
		}

		if len(branches) == 0 {
			return LoadCompleteMsg{
				Changes:   changes,
				TrunkName: trunkName,
			}
		}

		// Fetch existing PRs
		existingPRs, err := m.gh.GetPullRequestsForBranches(m.ctx, m.repo, branches)
		if err != nil {
			return LoadCompleteMsg{Err: err}
		}

		return LoadCompleteMsg{
			Changes:     changes,
			TrunkName:   trunkName,
			ExistingPRs: existingPRs,
		}
	}
}

func (m Model) syncCmd() tea.Cmd {
	// Collect info about remaining (not yet merged) revisions
	mutableRevs := m.stack.MutableRevisions()

	type revInfo struct {
		changeID string
		prNumber int
	}
	var remaining []revInfo

	for i := m.currentIndex; i < len(mutableRevs); i++ {
		// Revisions are in display order (current at top), so iterate from currentIndex
		revIdx := len(mutableRevs) - 1 - i
		if revIdx >= 0 {
			rev := mutableRevs[revIdx]
			remaining = append(remaining, revInfo{
				changeID: rev.Change.ID,
				prNumber: rev.PRNumber,
			})
		}
	}

	// Capture values needed in closure
	ctx := m.ctx
	gh := m.gh
	repo := m.repo
	trunkName := m.trunkName

	return func() tea.Msg {
		// Fetch latest
		if err := jj.GitFetch(); err != nil {
			return SyncCompleteMsg{Err: fmt.Errorf("git fetch: %w", err)}
		}

		if len(remaining) == 0 {
			return SyncCompleteMsg{}
		}

		// Rebase the first remaining change (root of remaining stack) onto trunk
		// This will also rebase all its descendants
		rootChangeID := remaining[0].changeID
		result, err := jj.Rebase(rootChangeID, "trunk()")
		if err != nil {
			return SyncCompleteMsg{Err: err}
		}
		if result.HasConflict {
			return SyncCompleteMsg{HasConflict: true}
		}

		// Push each remaining change and update PR base branches
		for i, rev := range remaining {
			if err := jj.GitPush(rev.changeID); err != nil {
				return SyncCompleteMsg{Err: fmt.Errorf("push %s: %w", rev.changeID[:8], err)}
			}

			// Update PR base branch:
			// - First PR in remaining stack should have base = trunk
			// - Others keep their stacked bases (previous PR's branch)
			if i == 0 && rev.prNumber > 0 {
				if err := gh.UpdatePullRequestBase(ctx, repo, rev.prNumber, trunkName); err != nil {
					return SyncCompleteMsg{Err: fmt.Errorf("update PR #%d base: %w", rev.prNumber, err)}
				}
			}
		}

		return SyncCompleteMsg{}
	}
}

func (m Model) checkMergeableCmd() tea.Cmd {
	mutableRevs := m.stack.MutableRevisions()
	if m.currentIndex >= len(mutableRevs) {
		return nil
	}

	// Revisions are in display order (current at top), we merge from bottom
	revIdx := len(mutableRevs) - 1 - m.currentIndex
	rev := mutableRevs[revIdx]
	prNumber := rev.PRNumber

	m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, "Checking if mergeable...")

	return func() tea.Msg {
		pr, err := m.gh.GetPullRequest(m.ctx, m.repo, prNumber)
		if err != nil {
			return MergeableCheckMsg{PRNumber: prNumber, Err: err}
		}

		mergeable := pr.GetMergeable()
		mergeableState := pr.GetMergeableState()

		// mergeable can be nil if GitHub hasn't computed it yet
		// mergeableState "clean" means ready to merge
		// mergeableState "blocked" means checks haven't passed or branch protection
		// mergeableState "behind" means needs to be updated with base branch
		isMergeable := mergeable && mergeableState == "clean"

		return MergeableCheckMsg{
			PRNumber:   prNumber,
			Mergeable:  isMergeable,
			MergeState: mergeableState,
		}
	}
}

func (m Model) mergeCmd(prNumber int) tea.Cmd {
	return func() tea.Msg {
		// Use empty commit title to use GitHub's default
		err := m.gh.MergePullRequest(m.ctx, m.repo, prNumber, "")
		return MergeCompleteMsg{PRNumber: prNumber, Err: err}
	}
}

// renderHelp renders the help view with custom styling
func renderHelp(keys KeyMap) string {
	var b strings.Builder

	// Render merge key in accent color
	if keys.Merge.Enabled() {
		renderKey(&b, keys.Merge, components.AccentStyle)
	}

	// Render separator and quit key in muted
	if keys.Quit.Enabled() {
		if b.Len() > 0 {
			b.WriteString(components.MutedStyle.Render(helpSeparator))
		}
		renderKey(&b, keys.Quit, components.MutedStyle)
	}

	return b.String()
}

// renderKey renders a single key binding with the given style
func renderKey(b *strings.Builder, k key.Binding, style lipgloss.Style) {
	b.WriteString(style.Render(k.Help().Key))
	b.WriteString(" ")
	b.WriteString(style.Render(k.Help().Desc))
}

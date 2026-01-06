package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/tui/components"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	gogithub "github.com/google/go-github/v80/github"
)

// Phase represents the current phase of the sync workflow
type Phase int

const (
	PhaseLoading Phase = iota
	PhaseUpToDate
	PhaseConfirmation
	PhaseSyncing
	PhaseUpdatingComments
	PhaseComplete
	PhaseError
)

// Help separator between key bindings
const helpSeparator = " • "

// Messages for async operations
type (
	RevisionsLoadedMsg struct {
		Changes       []jj.Change
		TrunkName     string
		ExistingPRs   map[string]*gogithub.PullRequest
		NeedsSync     bool
		NeedsSyncByID map[string]bool // Maps change ID to whether it needs sync
		Err           error
	}

	RevisionPushedMsg struct {
		Change jj.Change
		Err    error
	}

	RevisionSyncedMsg struct {
		ChangeID string
		PRNumber int
		Created  bool
		Err      error
	}

	AllCommentsUpdatedMsg struct {
		Err error
	}
)

// Model is the main bubbletea model for the TUI
type Model struct {
	// State
	phase   Phase
	stack   components.Stack
	spinner components.Spinner
	keys    KeyMap
	err     error
	width   int // Terminal width

	// Tracking sync progress
	currentIndex int
	totalCount   int

	// Dependencies
	ctx    context.Context
	gh     *github.Client
	repo   github.Repo
	revset string

	// Data from loading phase
	changes       []jj.Change
	existingPRs   map[string]*gogithub.PullRequest
	stackComments map[int]*gogithub.IssueComment
}

// NewModel creates a new TUI model
func NewModel(ctx context.Context, gh *github.Client, repo github.Repo, revset string) Model {
	return Model{
		phase:       PhaseLoading,
		spinner:     components.NewSpinner(),
		keys:        DefaultKeyMap(),
		ctx:         ctx,
		gh:          gh,
		repo:        repo,
		revset:      revset,
		existingPRs: make(map[string]*gogithub.PullRequest),
	}
}

// Init initializes the model and starts loading revisions
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick(),
		m.loadRevisionsAndPRsCmd(),
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
		case key.Matches(msg, m.keys.Submit) && m.phase == PhaseConfirmation:
			m.phase = PhaseSyncing
			m.currentIndex = 0
			return m, m.pushNextRevisionCmd()
		}

	case RevisionsLoadedMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		m.changes = msg.Changes
		m.existingPRs = msg.ExistingPRs
		m.stack = components.NewStack(msg.Changes, msg.TrunkName)
		m.totalCount = len(m.stack.MutableRevisions())

		// Set PR numbers and sync status for existing PRs on the stack
		for i := range m.stack.Revisions {
			rev := &m.stack.Revisions[i]
			if rev.IsImmutable {
				continue
			}
			// Set whether this revision needs sync
			if needsSync, ok := msg.NeedsSyncByID[rev.Change.ID]; ok {
				rev.NeedsSync = needsSync
			}
			if pr, ok := m.existingPRs[rev.Change.GitPushBookmark]; ok {
				rev.PRNumber = pr.GetNumber()
				if !msg.NeedsSync {
					// Mark as success if everything is up to date
					rev.State = components.StateSuccess
				}
			}
		}

		if !msg.NeedsSync {
			m.phase = PhaseUpToDate
			return m, tea.Quit
		}

		m.phase = PhaseConfirmation
		return m, nil

	case RevisionPushedMsg:
		if msg.Err != nil {
			m.stack.SetRevisionError(msg.Change.ID, msg.Err)
			m.phase = PhaseError
			m.err = msg.Err
			return m, nil
		}

		// Push succeeded, now sync the PR
		return m, m.syncRevisionPRCmd(msg.Change)

	case RevisionSyncedMsg:
		if msg.Err != nil {
			m.stack.SetRevisionError(msg.ChangeID, msg.Err)
			m.phase = PhaseError
			m.err = msg.Err
			return m, nil
		}

		m.stack.SetRevisionPR(msg.ChangeID, msg.PRNumber)
		m.stack.SetRevisionState(msg.ChangeID, components.StateSuccess, "")
		m.currentIndex++

		mutableRevs := m.stack.MutableRevisions()
		if m.currentIndex < len(mutableRevs) {
			return m, m.pushNextRevisionCmd()
		}

		// Move to comments phase
		m.phase = PhaseUpdatingComments
		return m, m.updateAllCommentsCmd()

	case AllCommentsUpdatedMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, nil
		}

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
	
	// Default terminal width if not yet received
	width := m.width
	if width == 0 {
		width = 80 // default fallback
	}
	
	viewOpts := components.ViewOptions{
		RepoOwner: m.repo.Owner,
		RepoName:  m.repo.Name,
		Width:     width,
	}

	switch m.phase {
	case PhaseLoading:
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Fetching remote state...\n")

	case PhaseUpToDate:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("\n")
		sb.WriteString(components.SuccessStyle.Render("All PRs are up to date!"))
		sb.WriteString("\n")

	case PhaseConfirmation:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("\n")
		syncCount := m.stack.RevisionsNeedingSync()
		totalCount := len(m.stack.MutableRevisions())
		if syncCount == totalCount {
			fmt.Fprintf(&sb, "%d revision(s) will be synced to GitHub.\n\n", syncCount)
		} else {
			fmt.Fprintf(&sb, "%d of %d revision(s) will be synced to GitHub.\n\n", syncCount, totalCount)
		}
		sb.WriteString(renderHelp(m.keys))
		sb.WriteString("\n")

	case PhaseSyncing:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString("Syncing revisions...\n\n")

	case PhaseUpdatingComments:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Updating stack comments...\n\n")

	case PhaseComplete:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		count := len(m.stack.MutableRevisions())
		fmt.Fprintf(&sb, "%d pull request(s) synced successfully.\n", count)

	case PhaseError:
		sb.WriteString(m.stack.View(m.spinner, viewOpts))
		sb.WriteString(components.ErrorStyle.Render("Sync failed"))
		sb.WriteString("\n\n")
		if m.err != nil {
			sb.WriteString(components.ErrorStyle.Render(m.err.Error()))
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// Commands for async operations

func (m Model) loadRevisionsAndPRsCmd() tea.Cmd {
	return func() tea.Msg {
		// First, load revisions to identify which branches to fetch
		// (before fetching, to avoid updating trunk which could cause rebasing issues)
		changes, err := jj.GetChanges(fmt.Sprintf("trunk()::(%s) & ~empty()", m.revset))
		if err != nil {
			return RevisionsLoadedMsg{Err: err}
		}

		// Collect branches for mutable changes only (not trunk)
		var branches []string
		for _, change := range changes {
			if !change.Immutable && change.Description != "" {
				branches = append(branches, change.GitPushBookmark)
			}
		}

		// Fetch only the mutable revision branches from remote (not trunk)
		// This prevents trunk from being updated which could cause the stack
		// to show as empty since revisions would no longer be descendants of trunk()
		if len(branches) > 0 {
			if err := jj.GitFetch(branches...); err != nil {
				return RevisionsLoadedMsg{Err: fmt.Errorf("git fetch: %w", err)}
			}
		}

		// Determine trunk name
		trunkName := "main"
		if len(changes) > 0 && changes[0].Immutable {
			if len(changes[0].Bookmarks) > 0 {
				trunkName = changes[0].Bookmarks[0].Name
			}
		}

		// Re-collect mutable changes after fetch
		var mutableChanges []jj.Change
		for _, change := range changes {
			if !change.Immutable && change.Description != "" {
				mutableChanges = append(mutableChanges, change)
			}
		}

		if len(mutableChanges) == 0 {
			return RevisionsLoadedMsg{
				Changes:   changes,
				TrunkName: trunkName,
				NeedsSync: false,
			}
		}

		// Fetch existing PRs
		existingPRs, err := m.gh.GetPullRequestsForBranches(m.ctx, m.repo, branches)
		if err != nil {
			return RevisionsLoadedMsg{Err: err}
		}

		// Check if sync is needed per revision
		needsSync := false
		needsSyncByID := make(map[string]bool)
		changesByID := make(map[string]*jj.Change)
		for i := range changes {
			changesByID[changes[i].ID] = &changes[i]
		}

		for _, change := range mutableChanges {
			parent := changesByID[change.Parents[0].ChangeID]
			base := parent.GitPushBookmark
			if parent.Immutable {
				if len(parent.Bookmarks) > 0 {
					base = parent.Bookmarks[0].Name
				}
			}

			title, body, _ := strings.Cut(change.Description, "\n")
			isDraft := strings.Contains(strings.ToLower(title), "wip")

			pr, exists := existingPRs[change.GitPushBookmark]
			if !exists {
				needsSync = true
				needsSyncByID[change.ID] = true
				continue
			}

			// Check if local commit matches remote head (need to push if different)
			if pr.GetHead().GetSHA() != change.CommitID {
				needsSync = true
				needsSyncByID[change.ID] = true
				continue
			}

			// Check if PR metadata needs update
			// Normalize body comparison by trimming trailing whitespace, as GitHub may strip it
			if pr.GetTitle() != title ||
				strings.TrimRight(pr.GetBody(), " \t\n\r") != strings.TrimRight(body, " \t\n\r") ||
				pr.GetBase().GetRef() != base ||
				pr.GetDraft() != isDraft {
				needsSync = true
				needsSyncByID[change.ID] = true
				continue
			}

			// This revision doesn't need sync
			needsSyncByID[change.ID] = false
		}

		return RevisionsLoadedMsg{
			Changes:       changes,
			TrunkName:     trunkName,
			ExistingPRs:   existingPRs,
			NeedsSync:     needsSync,
			NeedsSyncByID: needsSyncByID,
		}
	}
}

func (m Model) pushNextRevisionCmd() tea.Cmd {
	mutableRevs := m.stack.MutableRevisions()
	// Revisions are in reverse order (current at top), so we process from the end
	idx := len(mutableRevs) - 1 - m.currentIndex
	if idx < 0 {
		return nil
	}
	rev := mutableRevs[idx]
	m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, "Pushing...")

	return func() tea.Msg {
		change := rev.Change

		// Push the branch
		if err := jj.GitPush(change.ID); err != nil {
			return RevisionPushedMsg{Change: change, Err: fmt.Errorf("push: %w", err)}
		}

		return RevisionPushedMsg{Change: change}
	}
}

func (m Model) syncRevisionPRCmd(change jj.Change) tea.Cmd {
	// Determine if we're creating or updating
	_, exists := m.existingPRs[change.GitPushBookmark]
	if exists {
		m.stack.SetRevisionState(change.ID, components.StateInProgress, "Updating PR...")
	} else {
		m.stack.SetRevisionState(change.ID, components.StateInProgress, "Creating PR...")
	}

	return func() tea.Msg {
		// Build the PR options
		changesByID := make(map[string]*jj.Change)
		for i := range m.changes {
			changesByID[m.changes[i].ID] = &m.changes[i]
		}

		parent := changesByID[change.Parents[0].ChangeID]
		base := parent.GitPushBookmark
		if parent.Immutable {
			if len(parent.Bookmarks) > 0 {
				base = parent.Bookmarks[0].Name
			}
		}

		title, body, _ := strings.Cut(change.Description, "\n")
		isDraft := strings.Contains(strings.ToLower(title), "wip")

		if pr, ok := m.existingPRs[change.GitPushBookmark]; ok {
			// Check if update needed
			// Normalize body comparison by trimming trailing whitespace, as GitHub may strip it
			if pr.GetTitle() == title &&
				strings.TrimRight(pr.GetBody(), " \t\n\r") == strings.TrimRight(body, " \t\n\r") &&
				pr.GetHead().GetRef() == change.GitPushBookmark &&
				pr.GetBase().GetRef() == base &&
				pr.GetDraft() == isDraft {
				return RevisionSyncedMsg{
					ChangeID: change.ID,
					PRNumber: pr.GetNumber(),
					Created:  false,
				}
			}

			err := m.gh.UpdatePullRequest(m.ctx, m.repo, *pr.Number, github.PullRequestOptions{
				Title:  title,
				Body:   body,
				Branch: change.GitPushBookmark,
				Base:   base,
				Draft:  isDraft,
			})
			return RevisionSyncedMsg{
				ChangeID: change.ID,
				PRNumber: pr.GetNumber(),
				Created:  false,
				Err:      err,
			}
		}

		// Create new PR
		pr, err := m.gh.CreatePullRequest(m.ctx, m.repo, github.PullRequestOptions{
			Title:  title,
			Body:   body,
			Branch: change.GitPushBookmark,
			Base:   base,
			Draft:  isDraft,
		})
		if err != nil {
			return RevisionSyncedMsg{ChangeID: change.ID, Err: err}
		}
		// Store for later use
		m.existingPRs[change.GitPushBookmark] = pr
		return RevisionSyncedMsg{
			ChangeID: change.ID,
			PRNumber: pr.GetNumber(),
			Created:  true,
		}
	}
}

func (m Model) updateAllCommentsCmd() tea.Cmd {
	return func() tea.Msg {
		// Fetch existing stack comments
		var prNumbers []int
		for _, pr := range m.existingPRs {
			prNumbers = append(prNumbers, pr.GetNumber())
		}

		stackComments, err := m.gh.GetPRCommentsContaining(
			m.ctx,
			m.repo,
			prNumbers,
			"<!-- managed-by: jj-github -->",
		)
		if err != nil {
			return AllCommentsUpdatedMsg{Err: err}
		}

		// Update comments for each PR
		for _, rev := range m.stack.Revisions {
			if rev.IsImmutable {
				continue
			}

			pr, ok := m.existingPRs[rev.Change.GitPushBookmark]
			if !ok {
				continue
			}

			// Build the stack comment
			builder := &strings.Builder{}
			builder.WriteString("<!-- managed-by: jj-github -->\n")
			builder.WriteString("**Pull Request Stack**\n\n")

			// Show PRs in display order (current at top)
			for _, r := range m.stack.Revisions {
				if r.IsImmutable {
					continue
				}
				prForRev, ok := m.existingPRs[r.Change.GitPushBookmark]
				if !ok {
					continue
				}

				suffix := ""
				if pr.GetNumber() == prForRev.GetNumber() {
					suffix = " ←"
				}
				fmt.Fprintf(builder, "- #%d%s\n", prForRev.GetNumber(), suffix)
			}

			builder.WriteString("\n---\n")
			builder.WriteString("*Stack managed with [jj-github](https://github.com/cbrewster/jj-github)*")

			commentBody := builder.String()

			// Check if comment already exists and matches
			if existingComment, ok := stackComments[pr.GetNumber()]; ok {
				if existingComment.GetBody() == commentBody {
					continue
				}

				if err := m.gh.UpdatePullRequestComment(
					m.ctx,
					m.repo,
					existingComment.GetID(),
					commentBody,
				); err != nil {
					return AllCommentsUpdatedMsg{Err: err}
				}
				continue
			}

			// Create new comment
			if err := m.gh.CreatePullRequestComment(
				m.ctx,
				m.repo,
				pr.GetNumber(),
				commentBody,
			); err != nil {
				return AllCommentsUpdatedMsg{Err: err}
			}
		}

		return AllCommentsUpdatedMsg{}
	}
}

// renderHelp renders the help view with custom styling for submit (magenta) and quit (muted)
func renderHelp(keys KeyMap) string {
	var b strings.Builder

	// Render submit key in magenta
	if keys.Submit.Enabled() {
		renderKey(&b, keys.Submit, components.AccentStyle)
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

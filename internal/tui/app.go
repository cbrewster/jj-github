package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/state"
	"github.com/cbrewster/jj-github/internal/tui/components"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	gogithub "github.com/google/go-github/v80/github"
	"golang.org/x/sync/errgroup"
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

// Messages for async operations
type (
	RevisionsLoadedMsg struct {
		Changes     []jj.Change
		TrunkName   string
		ExistingPRs map[string]*gogithub.PullRequest
		State       *state.State
		MergedPRs   []components.MergedPR
		NeedsSync   bool
		Err         error
	}

	RevisionSyncedMsg struct {
		ChangeID string
		PRNumber int
		Title    string
		Branch   string
		Created  bool
		Err      error
	}

	AllRevisionsSyncedMsg struct {
		Results []RevisionSyncedMsg
		Err     error
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
	help    help.Model
	err     error

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

	// Persisted state for tracking PRs
	prState   *state.State
	mergedPRs []components.MergedPR
}

// NewModel creates a new TUI model
func NewModel(ctx context.Context, gh *github.Client, repo github.Repo, revset string) Model {
	h := help.New()
	h.ShortSeparator = " • "

	return Model{
		phase:       PhaseLoading,
		spinner:     components.NewSpinner(),
		keys:        DefaultKeyMap(),
		help:        h,
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
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Submit) && m.phase == PhaseConfirmation:
			m.phase = PhaseSyncing
			// Mark all mutable revisions as in progress
			for _, rev := range m.stack.MutableRevisions() {
				m.stack.SetRevisionState(rev.Change.ID, components.StateInProgress, "Syncing...")
			}
			return m, m.syncAllRevisionsCmd()
		}

	case RevisionsLoadedMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		m.changes = msg.Changes
		m.existingPRs = msg.ExistingPRs
		m.prState = msg.State
		m.mergedPRs = msg.MergedPRs
		m.stack = components.NewStack(msg.Changes, msg.TrunkName, msg.MergedPRs)
		m.totalCount = len(m.stack.MutableRevisions())

		// Set PR numbers for existing PRs on the stack
		for i := range m.stack.Revisions {
			rev := &m.stack.Revisions[i]
			if rev.IsImmutable {
				continue
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

	case AllRevisionsSyncedMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
		}

		// Process all results
		for _, result := range msg.Results {
			if result.Err != nil {
				m.stack.SetRevisionError(result.ChangeID, result.Err)
				m.phase = PhaseError
				m.err = result.Err
				return m, tea.Quit
			}

			m.stack.SetRevisionPR(result.ChangeID, result.PRNumber)
			m.stack.SetRevisionState(result.ChangeID, components.StateSuccess, "")

			// Update persisted state with the synced PR
			if m.prState != nil {
				m.prState.Set(result.ChangeID, state.StackEntry{
					PRNumber: result.PRNumber,
					Branch:   result.Branch,
					State:    state.PRStateOpen,
					Title:    result.Title,
				})
			}

			// Update existingPRs map for comment generation
			if m.existingPRs[result.Branch] == nil {
				m.existingPRs[result.Branch] = &gogithub.PullRequest{
					Number: &result.PRNumber,
				}
			}
		}

		// Move to comments phase
		m.phase = PhaseUpdatingComments
		return m, m.updateAllCommentsCmd()

	case AllCommentsUpdatedMsg:
		if msg.Err != nil {
			m.phase = PhaseError
			m.err = msg.Err
			return m, tea.Quit
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

	switch m.phase {
	case PhaseLoading:
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Fetching remote state...\n")

	case PhaseUpToDate:
		sb.WriteString(m.stack.View(m.spinner))
		sb.WriteString("\n")
		sb.WriteString(components.SuccessStyle.Render("All PRs are up to date!"))
		sb.WriteString("\n")

	case PhaseConfirmation:
		sb.WriteString(m.stack.View(m.spinner))
		sb.WriteString("\n")
		count := len(m.stack.MutableRevisions())
		fmt.Fprintf(&sb, "%d revision(s) will be synced to GitHub.\n\n", count)
		sb.WriteString(m.help.View(m.keys))
		sb.WriteString("\n")

	case PhaseSyncing:
		sb.WriteString(m.stack.View(m.spinner))
		sb.WriteString("Syncing revisions...\n\n")

	case PhaseUpdatingComments:
		sb.WriteString(m.stack.View(m.spinner))
		sb.WriteString(m.spinner.View())
		sb.WriteString(" Updating stack comments...\n\n")

	case PhaseComplete:
		sb.WriteString(m.stack.View(m.spinner))
		count := len(m.stack.MutableRevisions())
		fmt.Fprintf(&sb, "%d pull request(s) synced successfully.\n", count)

	case PhaseError:
		sb.WriteString(m.stack.View(m.spinner))
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
		// Fetch from remote to get latest state (read-only for local repo)
		if err := jj.GitFetch(); err != nil {
			return RevisionsLoadedMsg{Err: fmt.Errorf("git fetch: %w", err)}
		}

		// Load revisions
		changes, err := jj.GetChanges(fmt.Sprintf("trunk()::(%s) & ~empty()", m.revset))
		if err != nil {
			return RevisionsLoadedMsg{Err: err}
		}

		// Load persisted state
		prState, err := state.Load()
		if err != nil {
			return RevisionsLoadedMsg{Err: fmt.Errorf("load state: %w", err)}
		}

		// Determine trunk name
		trunkName := "main"
		if len(changes) > 0 && changes[0].Immutable {
			if len(changes[0].Bookmarks) > 0 {
				trunkName = changes[0].Bookmarks[0].Name
			}
		}

		// Collect branches for mutable changes
		var branches []string
		var mutableChanges []jj.Change
		changesByID := make(map[string]*jj.Change)
		for i := range changes {
			changesByID[changes[i].ID] = &changes[i]
			if !changes[i].Immutable && changes[i].Description != "" {
				branches = append(branches, changes[i].GitPushBookmark)
				mutableChanges = append(mutableChanges, changes[i])
			}
		}

		if len(mutableChanges) == 0 {
			return RevisionsLoadedMsg{
				Changes:   changes,
				TrunkName: trunkName,
				State:     prState,
				NeedsSync: false,
			}
		}

		// Fetch existing PRs by branch matching
		existingPRs, err := m.gh.GetPullRequestsForBranches(m.ctx, m.repo, branches)
		if err != nil {
			return RevisionsLoadedMsg{Err: err}
		}

		// Update state: add any PRs found by branch matching that aren't in state
		for _, change := range mutableChanges {
			if pr, ok := existingPRs[change.GitPushBookmark]; ok {
				title, _, _ := strings.Cut(change.Description, "\n")
				prState.Set(change.ID, state.StackEntry{
					PRNumber: pr.GetNumber(),
					Branch:   change.GitPushBookmark,
					State:    state.PRStateOpen,
					Title:    title,
				})
			}
		}

		// Update PR states from GitHub and build merged PRs list
		var mergedPRs []components.MergedPR
		var entriesToRemove []string

		for changeID, entry := range prState.Entries {
			// Skip entries for changes that are still in the local stack
			if _, inStack := changesByID[changeID]; inStack {
				continue
			}

			// Fetch PR state from GitHub
			pr, err := m.gh.GetPullRequest(m.ctx, m.repo, entry.PRNumber)
			if err != nil {
				return RevisionsLoadedMsg{Err: fmt.Errorf("get PR #%d: %w", entry.PRNumber, err)}
			}

			if pr == nil {
				// PR was deleted
				entriesToRemove = append(entriesToRemove, changeID)
				continue
			}

			// Update state based on PR status
			if !pr.GetMergedAt().IsZero() {
				// PR is merged - add to merged PRs list for display
				entry.State = state.PRStateMerged
				prState.Set(changeID, entry)
				mergedPRs = append(mergedPRs, components.MergedPR{
					ChangeID: changeID,
					PRNumber: entry.PRNumber,
					Title:    entry.Title,
				})
			} else if pr.GetState() == "closed" {
				// PR is closed but not merged - remove from state
				entriesToRemove = append(entriesToRemove, changeID)
			}
			// If still open, keep in state as-is
		}

		// Remove closed/deleted PRs from state
		for _, changeID := range entriesToRemove {
			prState.Remove(changeID)
		}

		// Check if sync is needed
		needsSync := false
		for _, change := range mutableChanges {
			// Determine the base branch for this change
			base := trunkName
			if len(change.Parents) > 0 {
				parentID := change.Parents[0].ChangeID
				for parentID != "" {
					parent := changesByID[parentID]
					if parent == nil {
						// Parent not in current stack (possibly merged), use trunk
						break
					}
					if parent.Immutable {
						if len(parent.Bookmarks) > 0 {
							base = parent.Bookmarks[0].Name
						}
						break
					}
					// Check if parent has an open PR
					if _, hasOpenPR := existingPRs[parent.GitPushBookmark]; hasOpenPR {
						base = parent.GitPushBookmark
						break
					}
					// Try grandparent
					if len(parent.Parents) > 0 {
						parentID = parent.Parents[0].ChangeID
					} else {
						break
					}
				}
			}

			title, body, _ := strings.Cut(change.Description, "\n")
			isDraft := strings.Contains(strings.ToLower(title), "wip")

			pr, exists := existingPRs[change.GitPushBookmark]
			if !exists {
				needsSync = true
				break
			}

			// Check if local commit matches remote head (need to push if different)
			if pr.GetHead().GetSHA() != change.CommitID {
				needsSync = true
				break
			}

			// Check if PR metadata needs update
			if pr.GetTitle() != title ||
				pr.GetBody() != body ||
				pr.GetBase().GetRef() != base ||
				pr.GetDraft() != isDraft {
				needsSync = true
				break
			}
		}

		return RevisionsLoadedMsg{
			Changes:     changes,
			TrunkName:   trunkName,
			ExistingPRs: existingPRs,
			State:       prState,
			MergedPRs:   mergedPRs,
			NeedsSync:   needsSync,
		}
	}
}

func (m Model) syncAllRevisionsCmd() tea.Cmd {
	return func() tea.Msg {
		mutableRevs := m.stack.MutableRevisions()
		if len(mutableRevs) == 0 {
			return AllRevisionsSyncedMsg{}
		}

		// Build helper maps
		changesByID := make(map[string]*jj.Change)
		for i := range m.changes {
			changesByID[m.changes[i].ID] = &m.changes[i]
		}

		// Phase 1: Push all branches sequentially (jj operations must be sequential)
		// Process from base to tip (reverse of display order)
		for i := len(mutableRevs) - 1; i >= 0; i-- {
			change := mutableRevs[i].Change
			if err := jj.GitPush(change.ID); err != nil {
				return AllRevisionsSyncedMsg{Err: fmt.Errorf("push %s: %w", change.ID[:8], err)}
			}
		}

		// Phase 2: Create/update all PRs concurrently (GitHub API calls are safe to parallelize)
		var mu sync.Mutex
		results := make([]RevisionSyncedMsg, 0, len(mutableRevs))
		createdPRs := make(map[string]*gogithub.PullRequest)

		eg, ctx := errgroup.WithContext(m.ctx)
		eg.SetLimit(8)

		for _, rev := range mutableRevs {
			change := rev.Change
			eg.Go(func() error {
				// Determine the base branch for this PR
				base := m.stack.TrunkName
				if len(change.Parents) > 0 {
					parentID := change.Parents[0].ChangeID
					for parentID != "" {
						parent := changesByID[parentID]
						if parent == nil {
							// Parent not in current stack, check if it's a merged PR
							if m.prState != nil {
								if entry := m.prState.GetByChangeID(parentID); entry != nil && entry.State == state.PRStateMerged {
									break
								}
							}
							break
						}
						if parent.Immutable {
							if len(parent.Bookmarks) > 0 {
								base = parent.Bookmarks[0].Name
							}
							break
						}
						// Check if parent has an open PR (valid branch)
						if _, hasOpenPR := m.existingPRs[parent.GitPushBookmark]; hasOpenPR {
							base = parent.GitPushBookmark
							break
						}
						// Try grandparent
						if len(parent.Parents) > 0 {
							parentID = parent.Parents[0].ChangeID
						} else {
							break
						}
					}
				}

				title, body, _ := strings.Cut(change.Description, "\n")
				isDraft := strings.Contains(strings.ToLower(title), "wip")

				var result RevisionSyncedMsg
				result.ChangeID = change.ID
				result.Title = title
				result.Branch = change.GitPushBookmark

				if pr, ok := m.existingPRs[change.GitPushBookmark]; ok {
					// Update existing PR
					err := m.gh.UpdatePullRequest(ctx, m.repo, pr.GetNumber(), github.PullRequestOptions{
						Title:  title,
						Body:   body,
						Branch: change.GitPushBookmark,
						Base:   base,
						Draft:  isDraft,
					})
					result.PRNumber = pr.GetNumber()
					result.Created = false
					result.Err = err
				} else {
					// Create new PR
					pr, err := m.gh.CreatePullRequest(ctx, m.repo, github.PullRequestOptions{
						Title:  title,
						Body:   body,
						Branch: change.GitPushBookmark,
						Base:   base,
						Draft:  isDraft,
					})
					if err != nil {
						result.Err = err
					} else {
						result.PRNumber = pr.GetNumber()
						result.Created = true
						mu.Lock()
						createdPRs[change.GitPushBookmark] = pr
						mu.Unlock()
					}
				}

				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return AllRevisionsSyncedMsg{Err: err}
		}

		// Store created PRs for comment generation
		for branch, pr := range createdPRs {
			m.existingPRs[branch] = pr
		}

		return AllRevisionsSyncedMsg{Results: results}
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
			if rev.IsImmutable || rev.IsMergedPR {
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

				if r.IsMergedPR {
					// Merged PR
					fmt.Fprintf(builder, "- #%d (merged)\n", r.PRNumber)
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

		// Save the state to disk
		if m.prState != nil {
			if err := m.prState.Save(); err != nil {
				return AllCommentsUpdatedMsg{Err: fmt.Errorf("save state: %w", err)}
			}
		}

		return AllCommentsUpdatedMsg{}
	}
}

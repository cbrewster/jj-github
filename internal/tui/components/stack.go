package components

import (
	"strings"

	"github.com/cbrewster/jj-github/internal/jj"
)

// Stack represents the full revision stack with trunk at bottom
type Stack struct {
	Revisions []Revision
	TrunkName string
}

// NewStack creates a new stack from a list of changes
// Changes should be in topological order (trunk first, current last)
// The stack will display in reverse order (current at top, trunk at bottom)
func NewStack(changes []jj.Change, trunkName string) Stack {
	revisions := make([]Revision, 0, len(changes)+1)

	// Add changes in reverse order (current at top)
	for i := len(changes) - 1; i >= 0; i-- {
		change := changes[i]
		if change.Immutable {
			continue // Skip immutable changes, we'll add trunk at the end
		}
		revisions = append(revisions, NewRevision(change))
	}

	// Add trunk at the bottom
	revisions = append(revisions, NewTrunkRevision(trunkName))

	return Stack{
		Revisions: revisions,
		TrunkName: trunkName,
	}
}

// SetRevisionState updates the state of a revision by change ID
func (s *Stack) SetRevisionState(changeID string, state RevisionState, statusMsg string) {
	for i := range s.Revisions {
		if s.Revisions[i].Change.ID == changeID {
			s.Revisions[i].State = state
			s.Revisions[i].StatusMsg = statusMsg
			return
		}
	}
}

// SetRevisionPR sets the PR number for a revision
func (s *Stack) SetRevisionPR(changeID string, prNumber int) {
	for i := range s.Revisions {
		if s.Revisions[i].Change.ID == changeID {
			s.Revisions[i].PRNumber = prNumber
			return
		}
	}
}

// SetRevisionError sets an error state for a revision
func (s *Stack) SetRevisionError(changeID string, err error) {
	for i := range s.Revisions {
		if s.Revisions[i].Change.ID == changeID {
			s.Revisions[i].State = StateError
			s.Revisions[i].Error = err
			s.Revisions[i].StatusMsg = "Error: " + err.Error()
			return
		}
	}
}

// MutableRevisions returns only the mutable (non-trunk) revisions
func (s *Stack) MutableRevisions() []Revision {
	var result []Revision
	for _, r := range s.Revisions {
		if !r.IsImmutable {
			result = append(result, r)
		}
	}
	return result
}

// RevisionsNeedingSync returns the count of revisions that need to be synced
func (s *Stack) RevisionsNeedingSync() int {
	count := 0
	for _, r := range s.Revisions {
		if !r.IsImmutable && r.NeedsSync {
			count++
		}
	}
	return count
}

// View renders the full stack
func (s Stack) View(spinner Spinner, opts ViewOptions) string {
	var sb strings.Builder
	sb.WriteString("\nRevisions:\n\n")

	for i, rev := range s.Revisions {
		// Show connector unless this is the last revision (trunk)
		showConnector := i < len(s.Revisions)-1
		sb.WriteString(rev.View(spinner, showConnector, opts))
	}

	return sb.String()
}

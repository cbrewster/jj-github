package components

import (
	"fmt"
	"strings"

	"github.com/cbrewster/jj-github/internal/jj"
)

// RevisionState represents the sync state of a revision
type RevisionState int

const (
	StatePending RevisionState = iota
	StateInProgress
	StateSuccess
	StateError
)

// Revision represents a single revision in the stack with its sync state
type Revision struct {
	Change      jj.Change
	State       RevisionState
	StatusMsg   string // Sub-status message (e.g., "Pushing...", "Creating PR...")
	PRNumber    int    // PR number if created/exists
	Error       error  // Error if state is StateError
	IsImmutable bool   // Is this an immutable revision (trunk)?
	IsMergedPR  bool   // Is this a merged PR (no local change)?
}

// NewRevision creates a new revision from a jj.Change
func NewRevision(change jj.Change) Revision {
	return Revision{
		Change:      change,
		State:       StatePending,
		IsImmutable: change.Immutable,
	}
}

// NewTrunkRevision creates a trunk/base revision marker
func NewTrunkRevision(branchName string) Revision {
	return Revision{
		Change: jj.Change{
			Description: branchName,
		},
		IsImmutable: true,
	}
}

// View renders the revision row
func (r Revision) View(spinner Spinner, showConnector bool) string {
	var sb strings.Builder

	// Determine the graph symbol
	symbol := r.graphSymbol(spinner)

	// Build the main line: symbol + change ID + description + PR number
	if r.IsImmutable && !r.IsMergedPR {
		// Trunk/immutable revision
		sb.WriteString(MutedStyle.Render(symbol))
		sb.WriteString("  ")
		sb.WriteString(MutedStyle.Render(r.Change.Description))
	} else if r.IsMergedPR {
		// Merged PR (no local change)
		sb.WriteString(SuccessStyle.Render(GraphSuccess))
		sb.WriteString("  ")
		// Description (first line, truncated)
		desc := r.firstLine(r.Change.Description)
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		sb.WriteString(MutedStyle.Render(desc))
		sb.WriteString("  ")
		sb.WriteString(MergedPRStyle.Render(fmt.Sprintf("#%d (merged)", r.PRNumber)))
	} else {
		sb.WriteString(symbol)
		sb.WriteString("  ")
		// Short change ID (first 8 chars)
		changeID := r.Change.ID
		if len(changeID) > 8 {
			changeID = changeID[:8]
		}
		sb.WriteString(ChangeIDShortStyle.Render(r.Change.ShortID))
		sb.WriteString(ChangeIDRestStyle.Render(changeID[len(r.Change.ShortID):]))
		sb.WriteString("  ")

		// Description (first line, truncated)
		desc := r.firstLine(r.Change.Description)
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		sb.WriteString(desc)

		// PR number if exists
		sb.WriteString("  ")
		if r.PRNumber > 0 {
			sb.WriteString(PRNumberStyle.Render(fmt.Sprintf("#%d", r.PRNumber)))
		} else {
			sb.WriteString(PRNumberStyle.Render("(new PR)"))
		}
	}

	sb.WriteString("\n")

	// Connector line to next revision (if not the last one)
	if showConnector {
		sb.WriteString(GraphLine)
	}

	// Status message line (if in progress or error)
	if r.StatusMsg != "" && (r.State == StateInProgress || r.State == StateError) {
		sb.WriteString("  ")
		if r.State == StateError {
			sb.WriteString(ErrorStyle.Render(r.StatusMsg))
		} else {
			sb.WriteString(MutedStyle.Render(r.StatusMsg))
		}
	}

	sb.WriteString("\n")

	return sb.String()
}

func (r Revision) graphSymbol(spinner Spinner) string {
	switch {
	case r.IsImmutable:
		return GraphTrunk
	case r.State == StateError:
		return ErrorStyle.Render(GraphError)
	case r.State == StateSuccess:
		return SuccessStyle.Render(GraphSuccess)
	case r.State == StateInProgress:
		return spinner.View()
	default:
		return GraphPending
	}
}

func (r Revision) firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx != -1 {
		return s[:idx]
	}
	return s
}

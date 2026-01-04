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
	IsCurrent   bool   // Is this the current working copy (@)?
	IsImmutable bool   // Is this an immutable revision (trunk)?
}

// NewRevision creates a new revision from a jj.Change
func NewRevision(change jj.Change, isCurrent bool) Revision {
	return Revision{
		Change:      change,
		State:       StatePending,
		IsCurrent:   isCurrent,
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
	if r.IsImmutable {
		// Trunk/immutable revision
		sb.WriteString(MutedStyle.Render(symbol))
		sb.WriteString("  ")
		sb.WriteString(MutedStyle.Render(r.Change.Description))
	} else {
		sb.WriteString(symbol)
		sb.WriteString("  ")
		// Short change ID (first 8 chars)
		changeID := r.Change.ID
		if len(changeID) > 8 {
			changeID = changeID[:8]
		}
		sb.WriteString(ChangeIDStyle.Render(changeID))
		sb.WriteString("  ")

		// Description (first line, truncated)
		desc := r.firstLine(r.Change.Description)
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		sb.WriteString(desc)

		// PR number if exists
		if r.PRNumber > 0 {
			sb.WriteString("  ")
			sb.WriteString(PRNumberStyle.Render(fmt.Sprintf("#%d", r.PRNumber)))
		}

		// Current marker
		if r.IsCurrent && r.State == StateSuccess {
			sb.WriteString("  ")
			sb.WriteString(MutedStyle.Render("← @"))
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
	case r.IsCurrent:
		return GraphCurrent
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

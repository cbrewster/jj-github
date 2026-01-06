package components

import (
	"fmt"
	"strings"

	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/rivo/uniseg"
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
	NeedsSync   bool   // Whether this revision needs to be synced
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

// ViewOptions contains options for rendering a revision
type ViewOptions struct {
	RepoOwner string
	RepoName  string
	Width     int
}

// View renders the revision row
func (r Revision) View(spinner Spinner, showConnector bool, opts ViewOptions) string {
	var sb strings.Builder

	// Determine the graph symbol
	symbol := r.graphSymbol(spinner)

	// Build the main line: symbol + change ID + description + PR link
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
		changeIDStr := ChangeIDShortStyle.Render(r.Change.ShortID) +
			ChangeIDRestStyle.Render(changeID[len(r.Change.ShortID):])
		sb.WriteString(changeIDStr)
		sb.WriteString("  ")

		// Build PR link or "(new PR)" text
		var prText string
		if r.PRNumber > 0 {
			prText = fmt.Sprintf("https://github.com/%s/%s/pull/%d", opts.RepoOwner, opts.RepoName, r.PRNumber)
		} else {
			prText = "(new PR)"
		}

		// Calculate available width for description
		// Layout: symbol(1-2) + "  " + changeID(8) + "  " + description + "  " + prLink
		// Symbol width varies (✓, ○, etc.) but we'll use 2 as a safe estimate
		symbolWidth := 2  // graph symbol width
		spacing := 2 + 2 + 2  // three "  " separators
		changeIDWidth := 8    // fixed change ID width
		prTextWidth := uniseg.StringWidth(prText)
		
		fixedWidth := symbolWidth + spacing + changeIDWidth + prTextWidth
		availableWidth := opts.Width - fixedWidth
		if availableWidth < 10 {
			availableWidth = 10 // Minimum width for description
		}

		// Description (first line, truncated based on available width)
		desc := r.firstLine(r.Change.Description)
		desc = truncateString(desc, availableWidth)
		sb.WriteString(desc)

		// PR link
		sb.WriteString("  ")
		sb.WriteString(PRNumberStyle.Render(prText))
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
	case r.State == StatePending && !r.NeedsSync:
		// Already up to date, show success indicator
		return SuccessStyle.Render(GraphSuccess)
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

// truncateString truncates a string to the specified width, adding "..." if truncated.
// It uses grapheme clustering to handle Unicode correctly.
func truncateString(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	
	// If the string width is already within limits, return as-is
	width := uniseg.StringWidth(s)
	if width <= maxWidth {
		return s
	}
	
	// Need to truncate - account for "..." (3 chars width)
	if maxWidth <= 3 {
		return s[:maxWidth] // Not enough room for ellipsis
	}
	
	targetWidth := maxWidth - 3
	var result strings.Builder
	currentWidth := 0
	
	gr := uniseg.NewGraphemes(s)
	for gr.Next() {
		grapheme := gr.Str()
		graphemeWidth := uniseg.StringWidth(grapheme)
		if currentWidth+graphemeWidth > targetWidth {
			break
		}
		result.WriteString(grapheme)
		currentWidth += graphemeWidth
	}
	
	result.WriteString("...")
	return result.String()
}

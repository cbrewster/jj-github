package components

import (
	"strings"
	"testing"

	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/stretchr/testify/assert"
)

func TestRevisionsNeedingSync(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Stack    Stack
		Expected int
	}{
		{
			Name: "all need sync",
			Stack: Stack{
				Revisions: []Revision{
					{Change: jj.Change{ID: "1"}, NeedsSync: true},
					{Change: jj.Change{ID: "2"}, NeedsSync: true},
					{IsImmutable: true}, // trunk
				},
			},
			Expected: 2,
		},
		{
			Name: "none need sync",
			Stack: Stack{
				Revisions: []Revision{
					{Change: jj.Change{ID: "1"}, NeedsSync: false},
					{Change: jj.Change{ID: "2"}, NeedsSync: false},
					{IsImmutable: true}, // trunk
				},
			},
			Expected: 0,
		},
		{
			Name: "some need sync",
			Stack: Stack{
				Revisions: []Revision{
					{Change: jj.Change{ID: "1"}, NeedsSync: true},
					{Change: jj.Change{ID: "2"}, NeedsSync: false},
					{Change: jj.Change{ID: "3"}, NeedsSync: true},
					{IsImmutable: true}, // trunk
				},
			},
			Expected: 2,
		},
		{
			Name: "empty stack (only trunk)",
			Stack: Stack{
				Revisions: []Revision{
					{IsImmutable: true}, // trunk
				},
			},
			Expected: 0,
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			count := tc.Stack.RevisionsNeedingSync()
			assert.Equal(t, tc.Expected, count)
		})
	}
}

func TestGraphSymbolNeedsSync(t *testing.T) {
	spinner := NewSpinner()

	// Revision that needs sync should show pending
	revNeedsSync := Revision{
		Change:    jj.Change{ID: "1"},
		State:     StatePending,
		NeedsSync: true,
	}
	assert.Equal(t, GraphPending, revNeedsSync.graphSymbol(spinner))

	// Revision that doesn't need sync should show success
	revUpToDate := Revision{
		Change:    jj.Change{ID: "2"},
		State:     StatePending,
		NeedsSync: false,
	}
	assert.Equal(t, SuccessStyle.Render(GraphSuccess), revUpToDate.graphSymbol(spinner))
}

func TestTruncateString(t *testing.T) {
	for _, tc := range []struct {
		Name     string
		Input    string
		MaxWidth int
		Expected string
	}{
		{
			Name:     "no truncation needed",
			Input:    "hello",
			MaxWidth: 10,
			Expected: "hello",
		},
		{
			Name:     "exact width",
			Input:    "hello",
			MaxWidth: 5,
			Expected: "hello",
		},
		{
			Name:     "truncate with ellipsis",
			Input:    "hello world",
			MaxWidth: 8,
			Expected: "hello...",
		},
		{
			Name:     "very short max width",
			Input:    "hello",
			MaxWidth: 3,
			Expected: "hel",
		},
		{
			Name:     "zero max width",
			Input:    "hello",
			MaxWidth: 0,
			Expected: "",
		},
		{
			Name:     "empty string",
			Input:    "",
			MaxWidth: 10,
			Expected: "",
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			result := truncateString(tc.Input, tc.MaxWidth)
			assert.Equal(t, tc.Expected, result)
		})
	}
}

func TestRevisionViewWithPRLink(t *testing.T) {
	spinner := NewSpinner()
	opts := ViewOptions{
		RepoOwner: "testowner",
		RepoName:  "testrepo",
		Width:     120,
	}

	// Revision with PR number should show full link
	rev := Revision{
		Change: jj.Change{
			ID:          "abcdefgh12345678",
			ShortID:     "abc",
			Description: "Test description",
		},
		State:     StatePending,
		NeedsSync: true,
		PRNumber:  42,
	}

	output := rev.View(spinner, true, opts)
	assert.Contains(t, output, "https://github.com/testowner/testrepo/pull/42")
	assert.Contains(t, output, "abc")
	assert.Contains(t, output, "Test description")

	// Revision without PR number should show "(new PR)"
	revNew := Revision{
		Change: jj.Change{
			ID:          "abcdefgh12345678",
			ShortID:     "abc",
			Description: "New revision",
		},
		State:     StatePending,
		NeedsSync: true,
		PRNumber:  0,
	}

	outputNew := revNew.View(spinner, true, opts)
	assert.Contains(t, outputNew, "(new PR)")
	assert.Contains(t, outputNew, "New revision")
}

func TestRevisionViewTruncation(t *testing.T) {
	spinner := NewSpinner()
	opts := ViewOptions{
		RepoOwner: "owner",
		RepoName:  "repo",
		Width:     80, // narrower width to test truncation
	}

	// Revision with long description should be truncated
	rev := Revision{
		Change: jj.Change{
			ID:          "abcdefgh12345678",
			ShortID:     "abc",
			Description: "This is a very long description that should be truncated to fit the available width",
		},
		State:     StatePending,
		NeedsSync: true,
		PRNumber:  123,
	}

	output := rev.View(spinner, true, opts)
	// Should contain the PR link (prioritized)
	assert.Contains(t, output, "https://github.com/owner/repo/pull/123")
	// Should contain the change ID (prioritized)
	assert.Contains(t, output, "abc")
	// Description should be truncated (contains "...")
	assert.True(t, strings.Contains(output, "...") || len(rev.Change.Description) <= 40,
		"Long descriptions should be truncated")
}

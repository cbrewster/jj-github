package components

import (
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

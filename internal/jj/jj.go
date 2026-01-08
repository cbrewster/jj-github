package jj

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

const (
	logTemplate = `"{\"id\": \"" ++ change_id ++ "\", \"short_id\": \"" ++ change_id.shortest() ++ "\", \"commit_id\": \"" ++ commit_id ++ "\", \"immutable\": " ++ immutable ++ ", \"description\": " ++ json(description) ++ ", \"bookmarks\": " ++ json(bookmarks) ++ ", \"git_push_bookmark\": \"" ++ %s ++ "\", \"parents\": " ++ json(parents) ++ "}"`
)

// Change represents a Jujutsu revision with its metadata.
type Change struct {
	ID              string `json:"id"`
	ShortID         string `json:"short_id"`
	CommitID        string `json:"commit_id"`
	Immutable       bool   `json:"immutable"`
	GitPushBookmark string `json:"git_push_bookmark"`
	Description     string `json:"description"`
	Bookmarks       []struct {
		Name string `json:"name"`
	} `json:"bookmarks"`
	Parents []struct {
		ChangeID string `json:"change_id"`
		CommitID string `json:"commit_id"`
	} `json:"parents"`
}

// GetChanges returns changes matching the given revsets in topological order.
func GetChanges(revsets ...string) ([]Change, error) {
	gitPushBookmark, err := GetTemplate("git_push_bookmark")
	if err != nil {
		return nil, err
	}

	args := []string{
		"log",
		"--no-graph",
		"--reversed",
		"-T", fmt.Sprintf(logTemplate, gitPushBookmark),
	}

	for _, revset := range revsets {
		args = append(args, "-r", revset)
	}

	out, err := exec.Command("jj", args...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			print(string(ee.Stderr))
		}
		return nil, err
	}

	var changes []Change
	decoder := json.NewDecoder(bytes.NewReader(out))
	for decoder.More() {
		var change Change
		if err := decoder.Decode(&change); err != nil {
			return nil, err
		}

		changes = append(changes, change)
	}

	return changes, nil
}

// GetTemplate returns a Jujutsu template value from the user's config.
func GetTemplate(name string) (string, error) {
	output, err := exec.Command("jj", "config", "get", "templates."+name).Output()
	if err != nil {
		return "", fmt.Errorf("get template %q: %w", name, err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetRemote returns the URL for the named Git remote.
func GetRemote(name string) (string, error) {
	output, err := exec.Command("jj", "git", "remote", "list").Output()
	if err != nil {
		return "", fmt.Errorf("jj git remote list: %w", err)
	}

	for line := range strings.Lines(string(output)) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		remote, url, ok := strings.Cut(trimmed, " ")
		if !ok {
			return "", fmt.Errorf("unknown remote format %q", line)
		}

		if remote == name {
			return url, nil
		}
	}

	return "", fmt.Errorf("remote named %q not found", name)
}

// GitPush pushes the specified change to its Git branch.
func GitPush(changeID string) error {
	return exec.Command("jj", "git", "push", "-c", fmt.Sprintf("change_id(%s)", changeID)).Run()
}

// GitFetch fetches from the Git remote to get the latest state.
// If branches are provided, only those branches are fetched.
// If branches is empty, all branches are fetched.
func GitFetch(branches ...string) error {
	args := []string{"git", "fetch"}
	for _, branch := range branches {
		args = append(args, "-b", branch)
	}
	return exec.Command("jj", args...).Run()
}

// Bookmark represents a jj bookmark with its associated revision.
type Bookmark struct {
	Name        string `json:"name"`
	ChangeID    string `json:"change_id"`
	ShortID     string `json:"short_id"`
	CommitID    string `json:"commit_id"`
	Description string `json:"description"`
}

// GetStackRootsToRebase returns the root revisions of stacks that need rebasing onto trunk.
// A "root" is a mutable revision whose parent is immutable (i.e., on or derived from trunk).
// This handles the case where a PR was squash-merged into trunk - the original commits
// are no longer direct children of trunk but still need to be rebased (and will become
// empty, which --skip-emptied will handle).
//
// Only returns roots that are NOT already parented on the current trunk commit.
func GetStackRootsToRebase() ([]Bookmark, error) {
	// Get the current trunk commit ID to check if roots are already parented on it
	trunkChanges, err := GetChanges("trunk()")
	if err != nil {
		return nil, fmt.Errorf("get trunk: %w", err)
	}
	if len(trunkChanges) == 0 {
		return nil, fmt.Errorf("trunk not found")
	}
	trunkCommitID := trunkChanges[0].CommitID

	// Find "roots" - mutable revisions whose parent is immutable.
	// The revset "roots(mutable())" gives us all mutable revisions that have no mutable ancestors.
	// These are the starting points of all working stacks.
	changes, err := GetChanges("roots(mutable())")
	if err != nil {
		return nil, fmt.Errorf("get stack roots: %w", err)
	}

	var bookmarks []Bookmark
	for _, change := range changes {
		// Skip if already parented on current trunk (no rebase needed)
		if len(change.Parents) > 0 && change.Parents[0].CommitID == trunkCommitID {
			continue
		}

		// Find the first local bookmark (skip remote-tracking ones with @)
		// If no bookmark, use the change ID as the identifier
		var bookmarkName string
		for _, b := range change.Bookmarks {
			if !strings.Contains(b.Name, "@") {
				bookmarkName = b.Name
				break
			}
		}

		bookmarks = append(bookmarks, Bookmark{
			Name:        bookmarkName,
			ChangeID:    change.ID,
			ShortID:     change.ShortID,
			CommitID:    change.CommitID,
			Description: change.Description,
		})
	}

	return bookmarks, nil
}

// RebaseResult contains the result of a rebase operation.
type RebaseResult struct {
	HasConflict  bool
	SkippedEmpty bool
}

// Rebase rebases a source revision and its descendants onto a destination.
// Uses `jj rebase -s <source> -d <destination> --skip-emptied` to rebase the entire subtree.
// The --skip-emptied flag automatically abandons commits that become empty after rebasing,
// which handles the case where a PR was squash-merged into trunk.
// Returns whether the rebase resulted in conflicts or skipped empty commits.
// jj treats conflicts as first-class, so we continue even if there's a conflict.
func Rebase(source, destination string) (RebaseResult, error) {
	cmd := exec.Command("jj", "rebase", "-s", source, "-d", destination, "--skip-emptied")
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// Check for conflicts and skipped commits in output
	hasConflict := strings.Contains(outputStr, "conflict")
	skippedEmpty := strings.Contains(outputStr, "Skipped rebase of") || strings.Contains(outputStr, "became empty")

	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// jj rebase can exit with error but still complete with conflicts
			// We consider it a success if there's conflict markers in output
			if hasConflict || skippedEmpty {
				return RebaseResult{HasConflict: hasConflict, SkippedEmpty: skippedEmpty}, nil
			}
			return RebaseResult{}, fmt.Errorf("rebase: %s", outputStr)
		}
		return RebaseResult{}, err
	}

	return RebaseResult{HasConflict: hasConflict, SkippedEmpty: skippedEmpty}, nil
}

// GetTrunkName returns the name of the trunk bookmark (e.g., "main" or "master").
func GetTrunkName() (string, error) {
	// Get the trunk revision and its bookmarks
	changes, err := GetChanges("trunk()")
	if err != nil {
		return "", err
	}
	if len(changes) == 0 {
		return "main", nil // Default fallback
	}
	if len(changes[0].Bookmarks) > 0 {
		return changes[0].Bookmarks[0].Name, nil
	}
	return "main", nil // Default fallback
}

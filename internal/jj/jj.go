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
	logTemplate = `"{\"id\": \"" ++ change_id ++ "\", \"commit_id\": \"" ++ commit_id ++ "\", \"immutable\": " ++ immutable ++ ", \"description\": " ++ json(description) ++ ", \"bookmarks\": " ++ json(bookmarks) ++ ", \"git_push_bookmark\": \"" ++ %s ++ "\", \"parents\": " ++ json(parents) ++ "}"`
)

// Change represents a Jujutsu revision with its metadata.
type Change struct {
	ID              string `json:"id"`
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
func GitFetch() error {
	return exec.Command("jj", "git", "fetch").Run()
}

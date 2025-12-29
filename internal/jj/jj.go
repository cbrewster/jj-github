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
	logTemplate = `"{\"id\": \"" ++ change_id ++ "\", \"commit_id\": \"" ++ commit_id ++ "\", \"immutable\": " ++ immutable ++ ", \"git_push_bookmark\": \"" ++ %s ++ "\", \"parents\": " ++ json(parents) ++ "}"`
)

type Change struct {
	ID              string `json:"id"`
	CommitID        string `json:"commit_id"`
	Immutable       bool   `json:"immutable"`
	GitPushBookmark string `json:"git_push_bookmark"`
	Parents         []struct {
		ChangeID string `json:"change_id"`
		CommitID string `json:"commit_id"`
	} `json:"parents"`
}

func GetChanges(revsets ...string) ([]Change, error) {
	gitPushBookmark, err := GetTemplate("git_push_bookmark")
	if err != nil {
		return nil, err
	}

	args := []string{"log", "--no-graph", "-T", fmt.Sprintf(logTemplate, gitPushBookmark)}

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

func GetTemplate(name string) (string, error) {
	output, err := exec.Command("jj", "config", "get", "templates."+name).Output()
	if err != nil {
		return "", fmt.Errorf("get template %q: %w", name, err)
	}

	return strings.TrimSpace(string(output)), nil
}

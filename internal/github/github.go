package github

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type Repo struct {
	Owner string
	Name  string
}

// GetRepoFromRemote returns repo information from the given URL.
// This supports both HTTPS and SSH URLs:
// - https://github.com/cbrewster/jj-github.git
// - git@github.com:cbrewster/jj-github.git
func GetRepoFromRemote(remote string) (Repo, error) {
	if strings.HasPrefix(remote, "https://") {
		return parseHttpsRemote(remote)
	}
	if strings.HasPrefix(remote, "git@") {
		return parseSshRemote(remote)
	}

	return Repo{}, errors.New("unknown remote format")
}

func parseSshRemote(remote string) (Repo, error) {
	first, second, ok := strings.Cut(remote, ":")
	if !ok {
		return Repo{}, errors.New("expected ssh remote to have \":\"")
	}

	_, host, ok := strings.Cut(first, "@")
	if !ok {
		return Repo{}, errors.New("expected ssh remote to have \"@\"")
	}

	if host != "github.com" {
		return Repo{}, errors.New("only github.com remotes are allowed")
	}

	owner, repo, ok := strings.Cut(second, "/")
	if !ok {
		return Repo{}, errors.New("expected https remote to have / delimiter")
	}

	repo, ok = strings.CutSuffix(repo, ".git")
	if !ok {
		return Repo{}, errors.New("expected https remote to end with .git")
	}

	return Repo{Owner: owner, Name: repo}, nil
}

func parseHttpsRemote(remote string) (Repo, error) {
	parsedUrl, err := url.Parse(remote)
	if err != nil {
		return Repo{}, nil
	}

	if parsedUrl.Host != "github.com" {
		return Repo{}, errors.New("only github.com remotes are allowed")
	}

	owner, repo, ok := strings.Cut(strings.TrimPrefix(parsedUrl.Path, "/"), "/")
	if !ok {
		return Repo{}, errors.New("expected https remote to have / delimiter")
	}

	repo, ok = strings.CutSuffix(repo, ".git")
	if !ok {
		return Repo{}, errors.New("expected https remote to end with .git")
	}

	return Repo{Owner: owner, Name: repo}, nil
}

// GetGHAuthToken returns a GitHub auth token using the gh cli.
func GetGHAuthToken() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("gh auth token: %w", err)
	}

	return strings.TrimSpace(string(out)), nil
}

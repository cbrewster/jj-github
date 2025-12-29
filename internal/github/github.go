package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/google/go-github/v80/github"
	"golang.org/x/sync/errgroup"
)

const (
	ghConcurrency = 8
)

type Client struct {
	client *github.Client
}

func NewClient() (*Client, error) {
	token, err := GetGHAuthToken()
	if err != nil {
		return nil, fmt.Errorf("get auth token from gh cli: %w", err)
	}

	return &Client{
		client: github.NewClient(nil).WithAuthToken(token),
	}, nil
}

// GetPullRequestsForBranches gets all the open pull requests for the specified branches.
// This expects only a single pull request to be open per branch.
func (c *Client) GetPullRequestsForBranches(
	ctx context.Context,
	repo Repo,
	branches []string,
) (map[string]*github.PullRequest, error) {
	var mu sync.Mutex
	result := make(map[string]*github.PullRequest)

	eg, ctx := errgroup.WithContext(ctx)

	for _, branch := range branches {
		eg.Go(func() error {
			prs, _, err := c.client.PullRequests.ListPullRequestsWithCommit(ctx, repo.Owner, repo.Name, branch, nil)
			if err != nil {
				return err
			}

			// Filter out closed PRs.
			prs = slices.DeleteFunc(prs, func(pr *github.PullRequest) bool {
				return pr.ClosedAt != nil || *pr.Head.Ref != branch
			})

			if len(prs) == 0 {
				return nil
			}

			if len(prs) > 1 {
				return fmt.Errorf("branch %q unexpectedly has %d open pull requests", branch, len(prs))
			}

			mu.Lock()
			result[branch] = prs[0]
			mu.Unlock()

			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

type PullRequestOptions struct {
	Title  string
	Body   string
	Branch string
	Base   string
	Draft  bool
}

func (c *Client) CreatePullRequest(
	ctx context.Context,
	repo Repo,
	opts PullRequestOptions,
) error {
	_, _, err := c.client.PullRequests.Create(ctx, repo.Owner, repo.Name, &github.NewPullRequest{
		Title: &opts.Title,
		Head:  &opts.Branch,
		Base:  &opts.Base,
		Body:  &opts.Body,
		Draft: &opts.Draft,
	})
	return err
}

func (c *Client) UpdatePullRequest(
	ctx context.Context,
	repo Repo,
	number int,
	opts PullRequestOptions,
) error {
	_, _, err := c.client.PullRequests.Edit(ctx, repo.Owner, repo.Name, number, &github.PullRequest{
		Title: &opts.Title,
		Head: &github.PullRequestBranch{
			Ref: &opts.Branch,
		},
		Base: &github.PullRequestBranch{
			Ref: &opts.Base,
		},
		Body:  &opts.Body,
		Draft: &opts.Draft,
	})
	return err
}

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

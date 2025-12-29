package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gh, err := github.NewClient()
	if err != nil {
		slog.Error("new github client", "error", err)
		os.Exit(1)
	}

	remote, err := jj.GetRemote("origin")
	if err != nil {
		slog.Error("get remote", "error", err)
		os.Exit(1)
	}
	slog.Info("remote", "remote", remote)

	repo, err := github.GetRepoFromRemote(remote)
	if err != nil {
		slog.Error("parse remote", "error", err)
		os.Exit(1)
	}
	slog.Info("repo", "repo", repo)

	revset := "@"
	if len(os.Args) > 1 {
		revset = os.Args[1]
	}

	changes, err := jj.GetChanges(fmt.Sprintf("trunk()::(%s) & ~empty()", revset))
	if err != nil {
		slog.Error("get changes", "error", err)
		os.Exit(1)
	}

	var branches []string
	changesByID := make(map[string]*jj.Change)

	slog.Info("pushing all commits")

	for _, change := range changes {
		changesByID[change.ID] = &change

		if change.Description == "" {
			slog.Error("change is missing description", "change", change.ID)
			continue
		}

		if change.Immutable {
			slog.Error("change is immutable", "change", change.ID)
			continue
		}

		slog.Info("pushing", "change", change.ID, "commit", change.CommitID)
		branches = append(branches, change.GitPushBookmark)
		if err := jj.GitPush(change.ID); err != nil {
			slog.Error("push change", "error", err)
			os.Exit(1)
		}
	}

	prs, err := gh.GetPullRequestsForBranches(ctx, repo, branches)
	if err != nil {
		slog.Error("get pull requests", "error", err)
		os.Exit(1)
	}

	slog.Info("updating pull requests")

	for _, change := range changes {
		if change.Description == "" {
			slog.Error("change is missing description", "change", change.ID)
			continue
		}

		if change.Immutable {
			slog.Error("change is immutable", "change", change.ID)
			continue
		}

		parent := changesByID[change.Parents[0].ChangeID]
		base := parent.GitPushBookmark
		if parent.Immutable {
			base = parent.Bookmarks[0].Name
		}

		title, body, _ := strings.Cut(change.Description, "\n")

		if pr, ok := prs[change.GitPushBookmark]; ok {
			slog.Info("updating pull request")
			if err := gh.UpdatePullRequest(ctx, repo, *pr.Number, github.PullRequestOptions{
				Title:  title,
				Body:   body,
				Branch: change.GitPushBookmark,
				Base:   base,
				Draft:  strings.Contains(strings.ToLower(title), "wip"),
			}); err != nil {
				slog.Error("failed to update pull request", "error", err)
				os.Exit(1)
			}
		} else {
			slog.Info("creating pull request")
			if err := gh.CreatePullRequest(ctx, repo, github.PullRequestOptions{
				Title:  title,
				Body:   body,
				Branch: change.GitPushBookmark,
				Base:   base,
				Draft:  strings.Contains(strings.ToLower(title), "wip"),
			}); err != nil {
				slog.Error("failed to create pull request", "error", err)
				os.Exit(1)
			}
		}
	}
}

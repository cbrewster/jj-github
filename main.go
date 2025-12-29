package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
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
	for _, change := range changes {
		branches = append(branches, change.GitPushBookmark)
		changesByID[change.ID] = &change
	}
	prs, err := gh.GetPullRequestsForBranches(ctx, repo, branches)
	if err != nil {
		slog.Error("get pull requests", "error", err)
		os.Exit(1)
	}

	slog.Info("Updating all branches")
	for _, change := range changes {
		if change.Description == "" {
			slog.Error("change is missing description", "change", change.ID)
			continue
		}

		if change.Immutable {
			slog.Error("change is immutable", "change", change.ID)
			continue
		}

		slog.Info("pushing", "change", change.ID, "commit", change.CommitID)
		if err := jj.GitPush(change.ID); err != nil {
			slog.Error("push change", "error", err)
			os.Exit(1)
		}

		if _, ok := prs[change.GitPushBookmark]; !ok {
			slog.Info(
				"would create PR!",
				"parent",
				changesByID[change.Parents[0].ChangeID].GitPushBookmark,
			)
		}
	}
}

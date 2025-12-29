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

		isDraft := strings.Contains(strings.ToLower(title), "wip")

		if pr, ok := prs[change.GitPushBookmark]; ok {
			if pr.GetTitle() == title &&
				pr.GetBody() == body &&
				pr.GetHead().GetRef() == change.GitPushBookmark &&
				pr.GetBase().GetRef() == base &&
				pr.GetDraft() == isDraft {
				continue
			}

			slog.Info("updating pull request")
			if err := gh.UpdatePullRequest(ctx, repo, *pr.Number, github.PullRequestOptions{
				Title:  title,
				Body:   body,
				Branch: change.GitPushBookmark,
				Base:   base,
				Draft:  isDraft,
			}); err != nil {
				slog.Error("failed to update pull request", "error", err)
				os.Exit(1)
			}

			continue
		}

		slog.Info("creating pull request")
		pr, err := gh.CreatePullRequest(ctx, repo, github.PullRequestOptions{
			Title:  title,
			Body:   body,
			Branch: change.GitPushBookmark,
			Base:   base,
			Draft:  isDraft,
		})
		if err != nil {
			slog.Error("failed to create pull request", "error", err)
			os.Exit(1)
		}
		prs[change.GitPushBookmark] = pr
	}

	slog.Info("adding comments")
	var prNumbers []int
	for _, pr := range prs {
		prNumbers = append(prNumbers, pr.GetNumber())
	}

	stackComments, err := gh.GetCommentsForPullRequestsWithContents(
		ctx,
		repo,
		prNumbers,
		"<!-- managed-by: jj-github -->",
	)
	if err != nil {
		slog.Error("get stack comments", "error", err)
		os.Exit(1)
	}

	for _, change := range changes {
		if change.Description == "" {
			slog.Error("change is missing description", "change", change.ID)
			continue
		}

		if change.Immutable {
			slog.Error("change is immutable", "change", change.ID)
			continue
		}

		currentPr, ok := prs[change.GitPushBookmark]
		if !ok {
			slog.Error("missing pr info for change", "change", change.ID)
			os.Exit(1)
		}

		builder := &strings.Builder{}
		builder.WriteString("<!-- managed-by: jj-github -->\n")
		builder.WriteString("**Pull Request Stack**\n\n")

		for i := len(changes) - 1; i >= 0; i-- {
			change := changes[i]
			pr, ok := prs[change.GitPushBookmark]
			if !ok {
				continue
			}

			suffix := ""
			if currentPr.GetNumber() == pr.GetNumber() {
				suffix = " ←"
			}

			fmt.Fprintf(builder, "- #%d%s\n", pr.GetNumber(), suffix)
		}

		builder.WriteString("\n---\n")
		builder.WriteString("*Stack managed with [jj-github](https://github.com/cbrewster/jj-github)*")

		slog.Info("pr info", "number", currentPr.Number, "comments", currentPr.GetComments())
		if comment, ok := stackComments[*currentPr.Number]; ok {
			if comment.GetBody() == builder.String() {
				continue
			}

			if err := gh.UpdatePullRequestComment(
				ctx,
				repo,
				comment.GetID(),
				builder.String(),
			); err != nil {
				slog.Error("failed to update pull request comment", "error", err)
				os.Exit(1)
			}

			continue
		}

		slog.Info("creating comment")
		if err := gh.CreatePullRequestComment(
			ctx,
			repo,
			currentPr.GetNumber(),
			builder.String(),
		); err != nil {
			slog.Error("failed to create pull request comment", "error", err)
			os.Exit(1)
		}
	}
}

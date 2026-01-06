package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"

	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/tui/submit"
	"github.com/cbrewster/jj-github/internal/tui/sync"
)

func main() {
	app := &cli.App{
		Name:  "jj-github",
		Usage: "Manage stacked pull requests with Jujutsu and GitHub",
		Commands: []*cli.Command{
			{
				Name:  "sync",
				Usage: "Fetch from remote and rebase bookmarks onto updated trunk",
				Action: func(c *cli.Context) error {
					return runSync(c.Context)
				},
			},
			{
				Name:      "submit",
				Usage:     "Submit revisions as pull requests to GitHub",
				ArgsUsage: "[revset]",
				Action: func(c *cli.Context) error {
					revset := "@"
					if c.Args().First() != "" {
						revset = c.Args().First()
					}
					return runSubmit(c.Context, revset)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runSync(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	model := sync.NewModel(ctx)
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}

func runSubmit(ctx context.Context, revset string) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gh, err := github.NewClient()
	if err != nil {
		return fmt.Errorf("creating GitHub client: %w", err)
	}

	remote, err := jj.GetRemote("origin")
	if err != nil {
		return fmt.Errorf("getting remote: %w", err)
	}

	repo, err := github.GetRepoFromRemote(remote)
	if err != nil {
		return fmt.Errorf("parsing remote: %w", err)
	}

	model := submit.NewModel(ctx, gh, repo, revset)
	p := tea.NewProgram(model)
	_, err = p.Run()
	return err
}

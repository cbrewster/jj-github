package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/cbrewster/jj-github/internal/github"
	"github.com/cbrewster/jj-github/internal/jj"
	"github.com/cbrewster/jj-github/internal/tui"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	gh, err := github.NewClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating GitHub client: %v\n", err)
		os.Exit(1)
	}

	remote, err := jj.GetRemote("origin")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting remote: %v\n", err)
		os.Exit(1)
	}

	repo, err := github.GetRepoFromRemote(remote)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing remote: %v\n", err)
		os.Exit(1)
	}

	revset := "@"
	if len(os.Args) > 1 {
		revset = os.Args[1]
	}

	model := tui.NewModel(ctx, gh, repo, revset)
	p := tea.NewProgram(model)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(1)
	}
}

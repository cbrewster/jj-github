package main

import (
	"encoding/json"
	"log/slog"
	"os"

	"github.com/cbrewster/jj-github/internal/jj"
)

func main() {
	remote, err := jj.GetRemote("origin")
	if err != nil {
		slog.Error("get remote", "error", err)
		os.Exit(1)
	}
	slog.Info("remote", "remote", remote)

	changes, err := jj.GetChanges(os.Args[1:]...)
	if err != nil {
		slog.Error("get changes", "error", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, change := range changes {
		if err := enc.Encode(change); err != nil {
			slog.Error("encode", "error", err)
			os.Exit(1)
		}
	}
}

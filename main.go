// Package main is the entrypoint for the hardcover CLI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/KIRKR101/hardcover-cli/cmd"
	"github.com/KIRKR101/hardcover-cli/internal/ui"
)

func main() {
	// Root context cancels on SIGINT/SIGTERM. Commands that make
	// HTTP requests thread this through, so Ctrl-C cancels in-flight
	// requests rather than just killing the UI.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	root := cmd.NewRootCmd()
	root.SetContext(ctx)

	if err := root.ExecuteContext(ctx); err != nil {
		ui.Exit(err)
	}
}

// Package cmd implements the cobra subcommands for the hardcover CLI.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

// Version is the CLI version, set via -ldflags at build time.
var Version = "0.1.0"

// Global flags shared across subcommands.
type globalFlags struct {
	noColor bool
	jsonOut bool
}

var gf = &globalFlags{}

// ctxKey is unexported so other packages can't collide.
type ctxKey string

const (
	ctxKeyClient ctxKey = "client"
	ctxKeyFlags  ctxKey = "flags"
	ctxKeyStyles ctxKey = "styles"
)

// NewRootCmd builds the root command and all subcommands.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hardcover",
		Short: "Manage your hardcover.app library from the terminal",
		Long:  "An unofficial CLI for the Hardcover book tracking service.",
		Version: Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// --no-color is a persistent flag; we read it on the root.
			gf.noColor, _ = cmd.Flags().GetBool("no-color")
			// json flag is per-subcommand; nothing to do globally here.
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.PersistentFlags().BoolVar(&gf.noColor, "no-color", false,
		"Disable colored output (also respects NO_COLOR env var)")
	root.SetVersionTemplate("hardcover {{.Version}}\n")

	// Subcommands.
	root.AddCommand(newSetupCmd())
	root.AddCommand(newWhoamiCmd())
	root.AddCommand(newLibraryCmd())
	root.AddCommand(newStatsCmd())
	root.AddCommand(newProgressCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newGoalsCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newDailyCmd())
	root.AddCommand(newLogCmd())
	root.AddCommand(newCompletionCmd())

	return root
}

// stylesFromCmd returns the cached styles for the current command.
func stylesFromCmd(cmd *cobra.Command) *ui.Styles {
	if s, ok := cmd.Context().Value(ctxKeyStyles).(*ui.Styles); ok {
		return s
	}
	s := ui.NewStyles(ui.HasColor(gf.noColor))
	cmd.SetContext(context.WithValue(cmd.Context(), ctxKeyStyles, s))
	return s
}

// flagsFromCmd returns the per-invocation flags.
func flagsFromCmd(cmd *cobra.Command) *globalFlags {
	return gf
}

// jsonFromCmd reads --json from any subcommand.
func jsonFromCmd(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

// exitError prints err to stderr and exits with the right code.
func exitError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "error: "+err.Error())
	os.Exit(ui.ExitCodeFor(err))
}

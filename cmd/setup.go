package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"

	"github.com/spf13/cobra"
)

func userHomeDir() (string, error) { return os.UserHomeDir() }

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup [token]",
		Short: "Save your Hardcover API token to ~/.hardcover.json",
		Long: `Save your Hardcover API token. Get one from
https://hardcover.app/account/api.

If no token is passed as an argument, you'll be prompted (input is
echoed, but on TTYs you can paste without echoing; we read the line
either way). Avoid passing the token as a positional argument on
shared machines; it'll land in your shell history.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var token string
			if len(args) == 1 {
				token = strings.TrimSpace(args[0])
				fmt.Fprintln(cmd.ErrOrStderr(),
					"warning: passing the token as an argument may leave it in your shell history.")
			} else {
				t, err := config.PromptToken()
				if err != nil {
					return err
				}
				token = strings.TrimSpace(t)
			}
			if token == "" {
				return fmt.Errorf("no token provided: %w", errs.ErrInvalid)
			}
			if err := config.SaveToken(token); err != nil {
				return err
			}
			path, _ := configPath()
			fmt.Fprintf(cmd.OutOrStdout(), "Token saved to %s (mode 0600)\n", path)
			return nil
		},
	}
}

// configPath is a tiny wrapper used by the setup command to print
// the destination path. Lives here to avoid exposing internals.
func configPath() (string, error) {
	// We import the path from config indirectly via SaveToken's behavior.
	// Reproduce the path here to keep things simple.
	home, err := userHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, config.FileName), nil
}

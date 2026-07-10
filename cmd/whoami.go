package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show current user info",
		RunE:  runWhoami,
	}
	cmd.Flags().Bool("json", false, "Output raw JSON instead of formatted text")
	return cmd
}

func runWhoami(cmd *cobra.Command, _ []string) error {
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)
	token, err := config.LoadToken()
	if err != nil {
		return err
	}
	c := api.New(token)

	var user api.User
	err = ui.WithSpinner(cmd.Context(), func(ctx context.Context) error {
		var resp struct {
			Me []api.User `json:"me"`
		}
		if gerr := c.GQL(ctx, api.QueryMe, nil, &resp); gerr != nil {
			return gerr
		}
		if len(resp.Me) == 0 {
			return fmt.Errorf("empty user response: %w", errs.ErrInvalid)
		}
		user = resp.Me[0]
		return nil
	})
	if err != nil {
		return err
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(user, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	// Formatted output.
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Title, "Hardcover Profile"))
	fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Dim, "┌──────────────────────────────────────┐"))
	fmt.Fprintf(cmd.OutOrStdout(),
		"  %s : %s\n",
		styles.Apply(styles.Bold, "Username"),
		user.Username)
	fmt.Fprintf(cmd.OutOrStdout(),
		"  %s       : %d\n",
		styles.Apply(styles.Bold, "ID"),
		user.ID)
	fmt.Fprintf(cmd.OutOrStdout(),
		"  %s    : %s\n",
		styles.Apply(styles.Bold, "Books"),
		styles.Apply(styles.BGreen, fmt.Sprintf("%d", user.BooksCount)))
	fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Dim, "└──────────────────────────────────────┘"))
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

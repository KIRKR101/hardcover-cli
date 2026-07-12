package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
	err = ui.WithSpinner(cmd.Context(), jsonMode, func(ctx context.Context) error {
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

	username := user.Username
	idStr := fmt.Sprintf("%d", user.ID)
	booksStr := fmt.Sprintf("%d", user.BooksCount)

	// Compute dynamic box width using styled rows so the border aligns exactly.
	styledUser := styles.Apply(styles.Bold, "Username")
	styledID := styles.Apply(styles.Bold, "ID")
	styledBooks := styles.Apply(styles.Bold, "Books")
	row1 := "  " + styledUser + " : " + username
	row2 := "  " + styledID + "       : " + idStr
	row3 := "  " + styledBooks + "    : " + styles.Apply(styles.BGreen, booksStr)
	boxInner := max(ui.VisibleWidth(row1), ui.VisibleWidth(row2), ui.VisibleWidth(row3))

	dim := styles.Apply(styles.Dim, "")
	fmt.Fprintln(cmd.OutOrStdout(), dim+"┌"+strings.Repeat("─", boxInner)+"┐")
	fmt.Fprintln(cmd.OutOrStdout(), row1)
	fmt.Fprintln(cmd.OutOrStdout(), row2)
	fmt.Fprintln(cmd.OutOrStdout(), row3)
	fmt.Fprintln(cmd.OutOrStdout(), dim+"└"+strings.Repeat("─", boxInner)+"┘")
	fmt.Fprintln(cmd.OutOrStdout())
	return nil
}

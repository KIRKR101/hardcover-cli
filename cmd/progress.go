package cmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newProgressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "progress",
		Short: "Show progress for books currently being read",
		RunE:  runProgress,
	}
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

func runProgress(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	token, err := config.LoadToken()
	if err != nil {
		return err
	}
	c := api.New(token)

	var me api.User
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		var gerr error
		me, gerr = getMe(ctx, c)
		return gerr
	})
	if err != nil {
		return err
	}

	var resp struct {
		UserBooks []api.UserBook `json:"user_books"`
	}
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		return c.GQL(ctx, api.QueryProgress, map[string]any{"userId": me.ID}, &resp)
	})
	if err != nil {
		return err
	}
	books := resp.UserBooks

	if jsonMode {
		raw, _ := json.MarshalIndent(books, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	if len(books) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Yellow, "No books currently being read."))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, fmt.Sprintf("Currently Reading (%d)", len(books))))
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────"))

	for _, ub := range books {
		book := ub.Book
		total := effectivePages(ub)
		current := 0
		if len(ub.UserBookReads) > 0 {
			current = ub.UserBookReads[0].ProgressPages
		}
		pct := 0.0
		if total > 0 {
			pct = float64(current) / float64(total) * 100
		}
		bar := styles.ProgressBar(pct, 30)
		totalDisplay := fmt.Sprintf("%d", total)
		if total <= 0 {
			totalDisplay = "?"
		}
		fmt.Fprintf(out, "\n  %s\n", styles.Apply(styles.Bold, book.Title))
		fmt.Fprintf(out, "  %s  %s  %s\n",
			bar,
			styles.Apply(styles.Bold, fmt.Sprintf("%.0f%%", pct)),
			styles.Apply(styles.Dim, fmt.Sprintf("(%d/%s pages)", current, totalDisplay)),
		)
	}
	fmt.Fprintln(out)
	return nil
}

// effectivePages returns the page count to use for progress display:
// the edition's page count if set, otherwise the book's.
func effectivePages(ub api.UserBook) int {
	if ub.Edition != nil && ub.Edition.Pages > 0 {
		return ub.Edition.Pages
	}
	if ub.Book.Pages > 0 {
		return ub.Book.Pages
	}
	return 0
}

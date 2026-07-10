package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "List books in your library",
		RunE:  runLibrary,
	}
	cmd.Flags().StringP("status", "s", "", "Filter by status (want, reading, read, paused, dnf, ignored)")
	cmd.Flags().IntP("limit", "l", 25, "Max books to show")
	cmd.Flags().IntP("offset", "o", 0, "Offset for pagination")
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

func runLibrary(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	status, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	if limit < 0 || offset < 0 {
		return fmt.Errorf("limit and offset must be non-negative: %w", errs.ErrInvalid)
	}

	token, err := config.LoadToken()
	if err != nil {
		return err
	}
	c := api.New(token)

	var me api.User
	err = ui.WithSpinner(ctx, func(ctx context.Context) error {
		var gerr error
		me, gerr = getMe(ctx, c)
		return gerr
	})
	if err != nil {
		return err
	}

	var statusID *int
	if status != "" {
		id, ok := ui.StatusShort[status]
		if !ok {
			return fmt.Errorf("unknown status %q (want: want, reading, read, paused, dnf, ignored): %w", status, errs.ErrInvalid)
		}
		statusID = &id
	}

	vars := map[string]any{
		"userId": me.ID,
		"limit":  limit,
		"offset": offset,
	}
	query := api.QueryLibraryNoStatus
	if statusID != nil {
		query = api.QueryLibrary
		vars["statusId"] = *statusID
	}

	var resp struct {
		UserBooks []api.UserBook `json:"user_books"`
	}
	err = ui.WithSpinner(ctx, func(ctx context.Context) error {
		return c.GQL(ctx, query, vars, &resp)
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
		if status == "" {
			fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Yellow, "No books found."))
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\n", styles.Apply(styles.Yellow, fmt.Sprintf("No books with status '%s'.", status)))
		}
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, styles.Apply(styles.Title, "Your Library"))
	renderLibraryTable(out, styles, books)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, fmt.Sprintf("Showing %d books (offset=%d)", len(books), offset)))
	fmt.Fprintln(out)
	return nil
}

// tableColumn widths. Total = sum + 4 separators × 3 chars + 2 = ~.
const (
	colIdx    = 4
	colStatus = 20
	colRating = 8
	titleW    = 44
	colAuth   = 22
)

// renderLibraryTable prints a fixed-column table. Each row is built
// from padded cells separated by dim " │ " markers; horizontal rules
// are drawn with box-drawing characters. Simple and obvious layout.
func renderLibraryTable(out io.Writer, styles *ui.Styles, books []api.UserBook) {
	dim := styles.Apply(styles.Dim, "")
	bold := func(s string) string { return styles.Apply(styles.Bold, s) }
	sep := dim + " │ "
	total := colIdx + colStatus + colRating + titleW + colAuth + (3 * 4) // 4 separators
	top := dim + "┌" + strings.Repeat("─", total) + "┐"
	mid := dim + "├" + strings.Repeat("─", total) + "┤"
	bot := dim + "└" + strings.Repeat("─", total) + "┘"

	header := sep +
		ui.PadRight(bold("#"), colIdx) + sep +
		ui.PadRight(bold("Status"), colStatus) + sep +
		ui.PadRight(bold("Rating"), colRating) + sep +
		ui.PadRight(bold("Title"), titleW) + sep +
		ui.PadRight(bold("Authors"), colAuth) + sep + dim

	fmt.Fprintln(out, top)
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, mid)

	for i, ub := range books {
		statusName := ui.StatusName(ub.StatusID)
		statusCell := ui.PadRight(styles.Apply(styles.StatusColor(ub.StatusID), statusName), colStatus)

		var ratingCell string
		if ub.Rating > 0 {
			ratingCell = styles.Apply(styles.BYellow, fmt.Sprintf("★ %.1f", ub.Rating))
		} else {
			ratingCell = styles.Apply(styles.Dim, "—")
		}
		ratingCell = ui.PadRight(ratingCell, colRating)

		authors := make([]string, 0, len(ub.Book.Contributions))
		for _, c := range ub.Book.Contributions {
			authors = append(authors, c.Author.Name)
		}
		authorStr := strings.Join(authors, ", ")
		if len(ub.Book.Contributions) > 2 {
			authorStr += "…"
		}
		authorStr = ui.Truncate(authorStr, colAuth)
		authorCell := ui.PadRight(authorStr, colAuth)

		title := ub.Book.Title
		maxTitle := titleW
		owned := ""
		if ub.Owned {
			owned = " " + styles.Apply(styles.Green, "[own]")
			maxTitle = titleW - len(" [own]")
		}
		title = ui.Truncate(title, maxTitle)
		titleCell := ui.PadRight(title+owned, titleW)

		idx := ui.PadRight(fmt.Sprintf("%d", i+1), colIdx)

		row := sep +
			idx + sep +
			statusCell + sep +
			ratingCell + sep +
			titleCell + sep +
			authorCell + sep + dim
		fmt.Fprintln(out, row)
	}
	fmt.Fprintln(out, bot)
}

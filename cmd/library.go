package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/filter"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newLibraryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "library",
		Short: "List books in your library",
		Long: `List books in your library with optional filtering.

Filter syntax examples:
  --filter "rating>=4 AND year=2026"
  --filter "status=reading OR status=want"
  --filter "title~'philosophy' AND owned=true"
  --filter "author~'^Fyodor'"

Fields: status, owned, rating, year, pages, added, title, author
Operators: =, !=, >, <, >=, <=, ~ (regex), !~ (not regex)
Logical: AND, OR, NOT, parentheses for grouping

Saved views:
  --save "name"        Save current filter as a named view
  --load "name"        Load a saved view
  --list-views         List all saved views
  --delete-view "name" Delete a saved view`,
		RunE: runLibrary,
	}
	cmd.Flags().StringP("status", "s", "", "Filter by status (want, reading, read, paused, dnf, ignored)")
	cmd.Flags().IntP("limit", "l", 25, "Max books to show")
	cmd.Flags().IntP("offset", "o", 0, "Offset for pagination")
	cmd.Flags().String("filter", "", "Filter expression (e.g. 'rating>=4 AND year=2026')")
	cmd.Flags().String("save", "", "Save current filter as a named view")
	cmd.Flags().String("load", "", "Load a saved view by name")
	cmd.Flags().Bool("list-views", false, "List all saved views")
	cmd.Flags().String("delete-view", "", "Delete a saved view")
	return cmd
}

func runLibrary(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	status, _ := cmd.Flags().GetString("status")
	limit, _ := cmd.Flags().GetInt("limit")
	offset, _ := cmd.Flags().GetInt("offset")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	filterStr, _ := cmd.Flags().GetString("filter")
	saveName, _ := cmd.Flags().GetString("save")
	loadName, _ := cmd.Flags().GetString("load")
	listViews, _ := cmd.Flags().GetBool("list-views")
	deleteViewName, _ := cmd.Flags().GetString("delete-view")

	out := cmd.OutOrStdout()

	if listViews {
		return handleListViews(out, styles)
	}

	if deleteViewName != "" {
		return handleDeleteView(out, styles, deleteViewName)
	}

	if limit < 0 || offset < 0 {
		return fmt.Errorf("limit and offset must be non-negative: %w", errs.ErrInvalid)
	}

	var finalFilter string
	if loadName != "" {
		views, err := config.LoadViews()
		if err != nil {
			return err
		}
		loaded, ok := views[loadName]
		if !ok {
			return fmt.Errorf("view %q not found: %w", loadName, errs.ErrInvalid)
		}
		finalFilter = loaded
		if filterStr != "" {
			finalFilter = "(" + finalFilter + ") AND (" + filterStr + ")"
		}
	} else if filterStr != "" {
		finalFilter = filterStr
	}

	if status != "" {
		if _, ok := ui.StatusShort[status]; !ok {
			return fmt.Errorf("unknown status %q (want: want, reading, read, paused, dnf, ignored): %w", status, errs.ErrInvalid)
		}
		statusFilter := fmt.Sprintf("status=%s", status)
		if finalFilter != "" {
			finalFilter = "(" + finalFilter + ") AND (" + statusFilter + ")"
		} else {
			finalFilter = statusFilter
		}
	}

	if saveName != "" {
		if finalFilter == "" {
			return fmt.Errorf("--save requires --filter or --status: %w", errs.ErrInvalid)
		}
		return handleSaveView(out, styles, saveName, finalFilter)
	}

	var parsedFilter filter.Expr
	if finalFilter != "" {
		var err error
		parsedFilter, err = filter.Parse(finalFilter)
		if err != nil {
			return fmt.Errorf("filter parse error: %w", err)
		}
	}

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

	var allBooks []api.UserBook
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		pageOffset := 0
		for {
			var resp struct {
				UserBooks []api.UserBook `json:"user_books"`
			}
			if gerr := c.GQL(ctx, api.QueryLibraryNoStatus, map[string]any{
				"userId": me.ID,
				"limit":  api.LibraryFetchLimit,
				"offset": pageOffset,
			}, &resp); gerr != nil {
				return gerr
			}
			allBooks = append(allBooks, resp.UserBooks...)
			if len(resp.UserBooks) < api.LibraryFetchLimit {
				break
			}
			pageOffset += api.LibraryFetchLimit
		}
		return nil
	})
	if err != nil {
		return err
	}

	if parsedFilter != nil {
		var filtered []api.UserBook
		for _, ub := range allBooks {
			match, err := filter.Eval(parsedFilter, ub)
			if err != nil {
				return fmt.Errorf("filter eval error: %w", err)
			}
			if match {
				filtered = append(filtered, ub)
			}
		}
		allBooks = filtered
	}

	sort.Slice(allBooks, func(i, j int) bool {
		return allBooks[i].DateAdded > allBooks[j].DateAdded
	})

	total := len(allBooks)
	if offset >= total {
		allBooks = nil
	} else {
		end := offset + limit
		if end > total {
			end = total
		}
		allBooks = allBooks[offset:end]
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(allBooks, "", "  ")
		fmt.Fprintln(out, string(raw))
		return nil
	}

	if len(allBooks) == 0 {
		if finalFilter == "" && status == "" {
			fmt.Fprintln(out, styles.Apply(styles.Yellow, "No books found."))
		} else {
			fmt.Fprintf(out, "%s\n", styles.Apply(styles.Yellow, "No books match the filter."))
		}
		return nil
	}

	fmt.Fprintln(out)
	if finalFilter != "" {
		fmt.Fprintln(out, styles.Apply(styles.Title, "Filtered Library"))
	} else {
		fmt.Fprintln(out, styles.Apply(styles.Title, "Your Library"))
	}
	renderLibraryTable(out, styles, allBooks)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, fmt.Sprintf("Showing %d of %d books (offset=%d)", len(allBooks), total, offset)))
	fmt.Fprintln(out)
	return nil
}

func handleListViews(out io.Writer, styles *ui.Styles) error {
	views, err := config.LoadViews()
	if err != nil {
		return err
	}

	if len(views) == 0 {
		fmt.Fprintln(out, styles.Apply(styles.Yellow, "No saved views."))
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, styles.Apply(styles.Title, "Saved Views"))
	fmt.Fprintln(out)

	names := make([]string, 0, len(views))
	for name := range views {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		query := views[name]
		fmt.Fprintf(out, "  %s %s\n", styles.Apply(styles.Cyan, name), styles.Apply(styles.Dim, query))
	}
	fmt.Fprintln(out)
	return nil
}

func handleSaveView(out io.Writer, styles *ui.Styles, name, query string) error {
	if err := config.SaveView(name, query); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Green, fmt.Sprintf("Saved view %q", name)))
	return nil
}

func handleDeleteView(out io.Writer, styles *ui.Styles, name string) error {
	if err := config.DeleteView(name); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Green, fmt.Sprintf("Deleted view %q", name)))
	return nil
}

const (
	colIdx    = 4
	colStatus = 20
	colRating = 8
	titleW    = 44
	colAuth   = 22
)

const tableTotal = colIdx + colStatus + colRating + titleW + colAuth + (6 * 3)

func renderLibraryTable(out io.Writer, styles *ui.Styles, books []api.UserBook) {
	dim := styles.Apply(styles.Dim, "")
	bold := func(s string) string { return styles.Apply(styles.Bold, s) }
	sep := dim + " │ "
	top := dim + "┌" + strings.Repeat("─", tableTotal) + "┐"
	mid := dim + "├" + strings.Repeat("─", tableTotal) + "┤"
	bot := dim + "└" + strings.Repeat("─", tableTotal) + "┘"

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

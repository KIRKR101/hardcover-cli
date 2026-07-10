package cmd

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export reading journal events to CSV",
		RunE:  runExport,
	}
	cmd.Flags().StringP("output", "o", "hardcover_export.csv", "Output CSV file")
	cmd.Flags().Bool("json", false, "Output raw JSON to stdout instead of writing CSV")
	return cmd
}

// exportEvent is one row of the CSV / one element of the JSON output.
type exportEvent struct {
	Book            string `json:"book"`
	Date            string `json:"date"`
	Timestamp       string `json:"timestamp"`
	CumulativePages int    `json:"cumulative_pages"`
	PagesRead       int    `json:"pages_read"`
}

func runExport(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	output, _ := cmd.Flags().GetString("output")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

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

	// Book title lookup. We only need id -> title.
	type bookRow struct {
		Book struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"book"`
	}
	var bookResp struct {
		UserBooks []bookRow `json:"user_books"`
	}
	err = ui.WithSpinner(ctx, func(ctx context.Context) error {
		offset := 0
		for {
			var resp struct {
				UserBooks []bookRow `json:"user_books"`
			}
			if gerr := c.GQL(ctx, `
query ($userId: Int!, $limit: Int!, $offset: Int!) {
  user_books(
    where: { user_id: { _eq: $userId } }
    limit: $limit
    offset: $offset
  ) {
    book { id title }
  }
}
`, map[string]any{
				"userId": me.ID,
				"limit":  api.LibraryFetchLimit,
				"offset": offset,
			}, &resp); gerr != nil {
				return gerr
			}
			bookResp.UserBooks = append(bookResp.UserBooks, resp.UserBooks...)
			if len(resp.UserBooks) < api.LibraryFetchLimit {
				break
			}
			offset += api.LibraryFetchLimit
		}
		return nil
	})
	if err != nil {
		return err
	}
	titles := make(map[int]string)
	for _, b := range bookResp.UserBooks {
		titles[b.Book.ID] = b.Book.Title
	}

	// All journals.
	var journals []api.ReadingJournal
	err = ui.WithSpinner(ctx, func(ctx context.Context) error {
		offset := 0
		for {
			var resp struct {
				Journals []api.ReadingJournal `json:"reading_journals"`
			}
			if gerr := c.GQL(ctx, api.QueryJournals, map[string]any{
				"userId": me.ID,
				"limit":  api.JournalFetchLimit,
				"offset": offset,
			}, &resp); gerr != nil {
				return gerr
			}
			journals = append(journals, resp.Journals...)
			if len(resp.Journals) < api.JournalFetchLimit {
				break
			}
			offset += api.JournalFetchLimit
		}
		return nil
	})
	if err != nil {
		return err
	}

	events := []exportEvent{}
	daily := map[string]map[string]int{} // date -> book -> pages
	for _, j := range journals {
		var meta struct {
			ProgressPages    *int `json:"progress_pages"`
			ProgressPagesWas *int `json:"progress_pages_was"`
		}
		if j.Metadata != nil {
			raw, _ := json.Marshal(j.Metadata)
			_ = json.Unmarshal(raw, &meta)
		}
		if meta.ProgressPages == nil {
			continue
		}
		current := *meta.ProgressPages
		was := 0
		if meta.ProgressPagesWas != nil {
			was = *meta.ProgressPagesWas
		}
		pages := current - was
		if pages <= 0 {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, j.ActionAt)
		if err != nil {
			t, _ = time.Parse(time.RFC3339, j.ActionAt)
		}
		dt := t.Format("2006-01-02")
		title := titles[j.BookID]
		if title == "" {
			title = fmt.Sprintf("book_id %d", j.BookID)
		}
		events = append(events, exportEvent{
			Book:            title,
			Date:            dt,
			Timestamp:       j.ActionAt,
			CumulativePages: current,
			PagesRead:       pages,
		})
		if daily[dt] == nil {
			daily[dt] = map[string]int{}
		}
		daily[dt][title] += pages
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(events, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create %s: %w", output, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write([]string{"book", "date", "timestamp", "cumulative_pages", "pages_read"}); err != nil {
		return err
	}
	for _, e := range events {
		if err := w.Write([]string{
			e.Book, e.Date, e.Timestamp,
			strconv.Itoa(e.CumulativePages), strconv.Itoa(e.PagesRead),
		}); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "  %s\n", styles.Success(fmt.Sprintf("Wrote %d events to %s", len(events), styles.Apply(styles.Bold, output))))
	fmt.Fprintf(out, "\n  %s\n", styles.Apply(styles.Bold, "Pages read per day:"))
	dates := sortedStringKeys(daily)
	for _, dt := range dates {
		books := daily[dt]
		total := 0
		for _, p := range books {
			total += p
		}
		fmt.Fprintf(out, "    %s %s: %s\n",
			styles.Bullet(),
			styles.Apply(styles.Bold, dt),
			styles.Apply(styles.Green, fmt.Sprintf("%d pages", total)),
		)
		bookNames := sortedStringKeys(books)
		for _, b := range bookNames {
			fmt.Fprintf(out, "      %s %s: %dp\n",
				styles.Apply(styles.Dim, "├─"),
				b, books[b],
			)
		}
	}
	return nil
}

// sortedStringKeys returns the keys of m sorted lexicographically.
// Used for both the outer date->book map and the inner book->pages map.
func sortedStringKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

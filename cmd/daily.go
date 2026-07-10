package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newDailyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daily",
		Short: "Show daily reading log",
		RunE:  runDaily,
	}
	cmd.Flags().IntP("days", "d", 7, "Number of days to show")
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

type dailyEntry struct {
	Title       string `json:"title"`
	Pages       int    `json:"pages"`
	Cumulative  int    `json:"cumulative"`
	TotalPages  *int   `json:"total_book_pages"`
}

type dailyResult struct {
	Date       string        `json:"date"`
	TotalPages int           `json:"total_pages"`
	Books      []dailyEntry  `json:"books"`
}

func runDaily(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	days, _ := cmd.Flags().GetInt("days")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	if days <= 0 {
		return fmt.Errorf("--days must be positive: %w", errs.ErrInvalid)
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

	// Book metadata: id -> { title, pages }.
	type bookRow struct {
		Book struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Pages int    `json:"pages"`
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
    book { id title pages }
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
	bookInfo := map[int]bookRow{}
	for _, b := range bookResp.UserBooks {
		bookInfo[b.Book.ID] = b
	}

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

	// Group journals by date -> book_id -> { pages, cumulative, total, title }.
	daily := map[string]map[int]map[string]any{}
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
		bid := j.BookID
		info := bookInfo[bid]
		title := info.Book.Title
		if title == "" {
			title = fmt.Sprintf("book_id %d", bid)
		}
		totalPages := info.Book.Pages
		if daily[dt] == nil {
			daily[dt] = map[int]map[string]any{}
		}
		entry, ok := daily[dt][bid]
		if !ok {
			entry = map[string]any{
				"pages":     0,
				"cumulative": 0,
				"title":     title,
			}
			if totalPages > 0 {
				entry["total_pages"] = totalPages
			}
		}
		entry["pages"] = entry["pages"].(int) + pages
		entry["cumulative"] = current
		daily[dt][bid] = entry
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -(days - 1))

	// Build ordered results.
	type dayKey string
	var dates []string
	for d := range daily {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			continue
		}
		if !t.Before(cutoff) && !t.After(today) {
			dates = append(dates, d)
		}
	}
	for i := 1; i < len(dates); i++ {
		for j := i; j > 0 && dates[j-1] > dates[j]; j-- {
			dates[j-1], dates[j] = dates[j], dates[j-1]
		}
	}

	jsonResult := []dailyResult{}
	flat := map[string]map[int]map[string]any{}
	for _, d := range dates {
		books := daily[d]
		dayTotal := 0
		var entries []dailyEntry
		// Sort books by pages desc.
		type kv struct {
			bid int
			e   map[string]any
		}
		var sorted []kv
		for bid, e := range books {
			sorted = append(sorted, kv{bid, e})
			dayTotal += e["pages"].(int)
		}
		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && sorted[j-1].e["pages"].(int) < sorted[j].e["pages"].(int); j-- {
				sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
			}
		}
		for _, k := range sorted {
			e := k.e
			ent := dailyEntry{
				Title:      e["title"].(string),
				Pages:      e["pages"].(int),
				Cumulative: e["cumulative"].(int),
			}
			if tp, ok := e["total_pages"].(int); ok {
				ent.TotalPages = &tp
			}
			entries = append(entries, ent)
		}
		jsonResult = append(jsonResult, dailyResult{
			Date: d, TotalPages: dayTotal, Books: entries,
		})
		flat[d] = books
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(jsonResult, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	if len(jsonResult) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n",
			styles.Apply(styles.Yellow, fmt.Sprintf("No reading activity in the last %d days.", days)),
		)
		return nil
	}

	weekdayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  %s\n", styles.Apply(styles.Title, fmt.Sprintf("Reading Log (last %d days)", days)))
	fmt.Fprintf(out, "  %s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────────────────────────"))

	totalAll := 0
	for _, entry := range jsonResult {
		dayTotal := entry.TotalPages
		totalAll += dayTotal
		t, _ := time.Parse("2006-01-02", entry.Date)
		wd := weekdayNames[int(t.Weekday())]
		fmt.Fprintf(out, "\n  %s %s %s  %s\n",
			styles.Bullet(),
			styles.Apply(styles.Bold, fmt.Sprintf("%s %s", entry.Date, wd)),
			"",
			styles.Apply(styles.Green, fmt.Sprintf("+%d pages", dayTotal)),
		)
		for i, b := range entry.Books {
			isLast := i == len(entry.Books)-1
			tree := "├──"
			if isLast {
				tree = "└──"
			}
			cumul := b.Cumulative
			totalStr := ""
			if b.TotalPages != nil {
				totalStr = fmt.Sprintf("/%d", *b.TotalPages)
			}
			// Truncate long titles so the dotted line stays consistent.
			// 40 chars leaves room for at least 2 dots before the page count.
			const maxTitle = 40
			displayTitle := b.Title
			if len(displayTitle) > maxTitle {
				displayTitle = ui.Truncate(displayTitle, maxTitle)
			}
			dots := 45 - len(displayTitle)
			if dots < 2 {
				dots = 2
			}
			fmt.Fprintf(out, "    %s %s %s  %s  %s\n",
				styles.Apply(styles.Dim, tree),
				fmt.Sprintf("%s %s", displayTitle, styles.Apply(styles.Dim, repeatDot(dots))),
				styles.Apply(styles.Bold, fmt.Sprintf("%dp", b.Pages)),
				styles.Apply(styles.Dim, fmt.Sprintf("(cumulative %d%s)", cumul, totalStr)),
				"",
			)
		}
	}
	avg := 0
	if len(jsonResult) > 0 {
		avg = totalAll / len(jsonResult)
	}
	fmt.Fprintf(out, "\n  %s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────────────────────────"))
	fmt.Fprintf(out, "  %s %s over %s %s %s\n",
		styles.Apply(styles.Bold, "Summary:"),
		styles.Apply(styles.Green, fmt.Sprintf("%d pages", totalAll)),
		styles.Apply(styles.Bold, fmt.Sprintf("%d", len(jsonResult))),
		styles.Apply(styles.Dim, "active days │"),
		styles.Apply(styles.BGreen, fmt.Sprintf("Avg: %d pages/day", avg)),
	)
	fmt.Fprintln(out)
	return nil
}

func repeatDot(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = '.'
	}
	return string(out)
}

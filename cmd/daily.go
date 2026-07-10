package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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
			if gerr := c.GQL(ctx, api.QueryBookTitlesAndPages, map[string]any{
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

	// Group journals by date -> book_id -> entry.
	daily := map[string]map[int]*dailyEntry{}
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
			daily[dt] = map[int]*dailyEntry{}
		}
		entry, ok := daily[dt][bid]
		if !ok {
			entry = &dailyEntry{Title: title}
			if totalPages > 0 {
				entry.TotalPages = &totalPages
			}
			daily[dt][bid] = entry
		}
		entry.Pages += pages
		entry.Cumulative = current
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	cutoff := today.AddDate(0, 0, -(days - 1))

	// Build ordered results.
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
	slices.Sort(dates)

	jsonResult := []dailyResult{}
	for _, d := range dates {
		books := daily[d]
		dayTotal := 0
		var sorted []*dailyEntry
		for _, e := range books {
			sorted = append(sorted, e)
			dayTotal += e.Pages
		}
		slices.SortFunc(sorted, func(a, b *dailyEntry) int {
			return b.Pages - a.Pages
		})
		var entries []dailyEntry
		for _, e := range sorted {
			entries = append(entries, *e)
		}
		jsonResult = append(jsonResult, dailyResult{
			Date: d, TotalPages: dayTotal, Books: entries,
		})
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(jsonResult, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	if len(jsonResult) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n",
			styles.Apply(styles.Yellow, fmt.Sprintf("No reading activity in the last %d days.", days)),
		)
		return nil
	}

	weekdayNames := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, fmt.Sprintf("Reading Log (last %d days)", days)))
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────────────────────────"))

	totalAll := 0
	for _, entry := range jsonResult {
		dayTotal := entry.TotalPages
		totalAll += dayTotal
		t, _ := time.Parse("2006-01-02", entry.Date)
		wd := weekdayNames[int(t.Weekday())]
		fmt.Fprintf(out, "\n%s %s %s  %s\n",
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
			const maxTitle = 40
			displayTitle := b.Title
			if utf8.RuneCountInString(displayTitle) > maxTitle {
				displayTitle = ui.Truncate(displayTitle, maxTitle)
			}
			titleLen := utf8.RuneCountInString(displayTitle)
			dots := 50 - titleLen
			if dots < 2 {
				dots = 2
			}
			fmt.Fprintf(out, "  %s %s%s  %3dp  %s\n",
				styles.Apply(styles.Dim, tree),
				displayTitle,
				styles.Apply(styles.Dim, strings.Repeat(".", dots)),
				b.Pages,
				styles.Apply(styles.Dim, fmt.Sprintf("(cumulative %d%s)", cumul, totalStr)),
			)
		}
	}
	avg := 0
	if len(jsonResult) > 0 {
		avg = totalAll / len(jsonResult)
	}
	fmt.Fprintf(out, "\n%s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────────────────────────"))
	fmt.Fprintf(out, "%s %s over %s %s %s\n",
		styles.Apply(styles.Bold, "Summary:"),
		styles.Apply(styles.Green, fmt.Sprintf("%d pages", totalAll)),
		styles.Apply(styles.Bold, fmt.Sprintf("%d", len(jsonResult))),
		styles.Apply(styles.Dim, "active days │"),
		styles.Apply(styles.BGreen, fmt.Sprintf("Avg: %d pages/day", avg)),
	)
	fmt.Fprintln(out)
	return nil
}

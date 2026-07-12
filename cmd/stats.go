package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show reading statistics and active goals",
		RunE:  runStats,
	}
	return cmd
}

func runStats(cmd *cobra.Command, _ []string) error {
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

	var statsResp struct {
		Total   *aggregateCount    `json:"total"`
		Read    *aggregateCount    `json:"read"`
		Reading *aggregateCount    `json:"reading"`
		Want    *aggregateCount    `json:"want"`
		DNF     *aggregateCount    `json:"dnf"`
		Rated   *aggregateCountAvg `json:"rated"`
		Goals   []api.Goal         `json:"goals"`
	}
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		return c.GQL(ctx, api.QueryStats, map[string]any{"userId": me.ID}, &statsResp)
	})
	if err != nil {
		return err
	}

	totalCount := countOf(statsResp.Total)
	readCount := countOf(statsResp.Read)
	readingCount := countOf(statsResp.Reading)
	wantCount := countOf(statsResp.Want)
	dnfCount := countOf(statsResp.DNF)
	ratedCount := countOfAvg(statsResp.Rated)
	var avgRating float64
	if statsResp.Rated != nil && statsResp.Rated.Aggregate.Avg != nil {
		avgRating = statsResp.Rated.Aggregate.Avg.Rating
	}
	goals := statsResp.Goals

	var totalPages int
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		var perr error
		totalPages, perr = fetchReadTotalPages(ctx, c, me.ID)
		return perr
	})
	if err != nil {
		return err
	}

	if jsonMode {
		out := map[string]any{
			"username":     me.Username,
			"total":        totalCount,
			"read":         readCount,
			"reading":      readingCount,
			"want":         wantCount,
			"dnf":          dnfCount,
			"total_pages":  totalPages,
			"rated":        ratedCount,
			"avg_rating":   avgRating,
			"active_goals": goals,
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, fmt.Sprintf("Library Statistics: %s", me.Username)))
	boxInner := statsInnerWidth
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, "┌"+strings.Repeat("─", boxInner)+"┐"))

	type stat struct {
		label string
		value string
		style *lipgloss.Style
	}
	stats := []stat{
		{"Total books", fmt.Sprintf("%d", totalCount), nil},
		{"Read", fmt.Sprintf("%d", readCount), &styles.BGreen},
		{"Currently reading", fmt.Sprintf("%d", readingCount), &styles.BYellow},
		{"Want to read", fmt.Sprintf("%d", wantCount), &styles.Cyan},
		{"DNF (Did Not Finish)", fmt.Sprintf("%d", dnfCount), &styles.Red},
		{"Total pages read", fmt.Sprintf("%d", totalPages), &styles.Bold},
	}
	for _, s := range stats {
		val := s.value
		if s.style != nil {
			val = styles.Apply(*s.style, val)
		}
		dots := boxInner - 4 - ui.VisibleWidth(s.label) - ui.VisibleWidth(val)
		if dots < 1 {
			dots = 1
		}
		fmt.Fprintf(out, "%s  %s %s%s %s\n",
			styles.Apply(styles.Dim, "│"),
			s.label,
			styles.Apply(styles.Dim, strings.Repeat(".", dots)),
			val,
			styles.Apply(styles.Dim, "│"),
		)
	}

	if ratedCount > 0 {
		ratingVal := styles.Apply(styles.BYellow, fmt.Sprintf("★ %.2f", avgRating)) +
			" " + styles.Apply(styles.Dim, fmt.Sprintf("(%d rated)", ratedCount))
		dots := boxInner - 4 - ui.VisibleWidth("Avg rating") - ui.VisibleWidth(ratingVal)
		if dots < 1 {
			dots = 1
		}
		fmt.Fprintf(out, "%s  %s %s%s %s\n",
			styles.Apply(styles.Dim, "│"),
			"Avg rating",
			styles.Apply(styles.Dim, strings.Repeat(".", dots)),
			ratingVal,
			styles.Apply(styles.Dim, "│"),
		)
	}
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, "└"+strings.Repeat("─", boxInner)+"┘"))
	fmt.Fprintln(out)

	if len(goals) > 0 {
		fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, "Active Goals"))
		fmt.Fprintf(out, "%s\n", styles.Separator("─", 42))
		for _, g := range goals {
			pct := 0.0
			if g.Goal > 0 {
				pct = g.Progress / g.Goal * 100
			}
			metricName := titleCase(strings.ReplaceAll(g.Metric, "_", " "))
			bar := styles.ProgressBar(pct, 24)
			fmt.Fprintf(out, "  %s\n", styles.Apply(styles.Bold, metricName))
			fmt.Fprintf(out, "  %s  %s  %s\n",
				bar,
				styles.Apply(styles.Bold, fmt.Sprintf("%.0f%%", pct)),
				styles.Apply(styles.Dim, fmt.Sprintf("(%.0f/%.0f)", g.Progress, g.Goal)),
			)
			fmt.Fprintf(out, "  %s\n\n",
				styles.Apply(styles.Dim, fmt.Sprintf("Timeframe: %s to %s", g.StartDate, g.EndDate)),
			)
		}
	}
	return nil
}

type aggregateCount struct {
	Aggregate struct {
		Count int `json:"count"`
	} `json:"aggregate"`
}

type aggregateCountAvg struct {
	Aggregate struct {
		Count int `json:"count"`
		Avg   *struct {
			Rating float64 `json:"rating"`
		} `json:"avg"`
	} `json:"aggregate"`
}

func countOf(a *aggregateCount) int {
	if a == nil {
		return 0
	}
	return a.Aggregate.Count
}

func countOfAvg(a *aggregateCountAvg) int {
	if a == nil {
		return 0
	}
	return a.Aggregate.Count
}

// fetchReadTotalPages sums the page count of every book the user has
// marked as read, paginating. Uses the edition page count if available,
// otherwise the book's default.
func fetchReadTotalPages(ctx context.Context, c *api.Client, userID int) (int, error) {
	total := 0
	offset := 0
	for {
		var resp struct {
			UserBooks []struct {
				Edition *struct {
					Pages int `json:"pages"`
				} `json:"edition"`
				Book *struct {
					Pages int `json:"pages"`
				} `json:"book"`
			} `json:"user_books"`
		}
		err := c.GQL(ctx, api.QueryReadBooksPages, map[string]any{
			"userId": userID,
			"limit":  api.LibraryFetchLimit,
			"offset": offset,
		}, &resp)
		if err != nil {
			return 0, err
		}
		for _, ub := range resp.UserBooks {
			pages := 0
			if ub.Edition != nil && ub.Edition.Pages > 0 {
				pages = ub.Edition.Pages
			} else if ub.Book != nil && ub.Book.Pages > 0 {
				pages = ub.Book.Pages
			}
			total += pages
		}
		if len(resp.UserBooks) < api.LibraryFetchLimit {
			break
		}
		offset += api.LibraryFetchLimit
	}
	return total, nil
}

// titleCase capitalizes the first letter of each space-separated word.
func titleCase(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// statsInnerWidth is the inner content width for the stats box.
const statsInnerWidth = 38

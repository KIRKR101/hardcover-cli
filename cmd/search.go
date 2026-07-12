package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newSearchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for books on Hardcover",
		Args:  cobra.ExactArgs(1),
		RunE:  runSearch,
	}
	cmd.Flags().IntP("limit", "l", 10, "Max results")
	return cmd
}

func runSearch(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	query := args[0]
	limit, _ := cmd.Flags().GetInt("limit")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	token, err := config.LoadToken()
	if err != nil {
		return err
	}
	c := api.New(token)

	var resp struct {
		Search struct {
			Results struct {
				Hits []api.SearchHit `json:"hits"`
			} `json:"results"`
		} `json:"search"`
	}
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		return c.GQL(ctx, api.QuerySearch, map[string]any{
			"query":   query,
			"perPage": limit,
		}, &resp)
	})
	if err != nil {
		return err
	}
	hits := resp.Search.Results.Hits

	if jsonMode {
		raw, _ := json.MarshalIndent(hits, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	if len(hits) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", styles.Apply(styles.Yellow, fmt.Sprintf("No results for %q.", query)))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, fmt.Sprintf("Search Results for %q", query)))
	fmt.Fprintf(out, "%s\n", styles.Separator("─", 76))

	for i, hit := range hits {
		r := decodeHit(hit)
		title, _ := r["title"].(string)
		author, _ := r["author_names"].([]any)
		pages := asInt(r["pages"])
		year := asInt(r["release_year"])
		rating := asFloat(r["rating"])

		if title == "" {
			title = "?"
		}
		authorStr := ""
		if len(author) > 0 {
			parts := make([]string, 0, len(author))
			for _, a := range author {
				if s, ok := a.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
			authorStr = ui.JoinStrings(parts, ", ")
		}

		meta := []string{}
		if authorStr != "" {
			meta = append(meta, "by "+styles.Apply(styles.Bold, authorStr))
		}
		if pages > 0 {
			meta = append(meta, fmt.Sprintf("%dp", pages))
		}
		if year > 0 {
			meta = append(meta, strconv.Itoa(year))
		}
		if rating > 0 {
			meta = append(meta, styles.Apply(styles.BYellow, fmt.Sprintf("★ %.1f", rating)))
		}

		fmt.Fprintf(out, "%s %s\n", styles.Apply(styles.BCyan, fmt.Sprintf("%2d.", i+1)), styles.Apply(styles.Bold, title))
		if len(meta) > 0 {
			fmt.Fprintf(out, "   %s %s\n", styles.Apply(styles.Dim, "│"), ui.JoinStrings(meta, "  •  "))
		}
	}
	fmt.Fprintln(out)
	return nil
}

// decodeHit turns a SearchHit into a generic map. The search endpoint
// returns `document` either as an object or a JSON-encoded string; the
// Python implementation handles both, so we do too.
func decodeHit(h api.SearchHit) map[string]any {
	if len(h.Document) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(h.Document, &m); err != nil {
		// Document might be a JSON-encoded string instead of an object.
		var s string
		if err2 := json.Unmarshal(h.Document, &s); err2 == nil {
			_ = json.Unmarshal([]byte(s), &m)
		}
	}
	return m
}

func asInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	}
	return 0
}

func asFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	}
	return 0
}

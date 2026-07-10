package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newGoalsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goals",
		Short: "Show reading goals",
		RunE:  runGoals,
	}
	cmd.Flags().Bool("all", false, "Include archived goals")
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

func runGoals(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	all, _ := cmd.Flags().GetBool("all")
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

	query := api.QueryGoalsActive
	if all {
		query = api.QueryGoalsAll
	}
	var resp struct {
		Goals []api.Goal `json:"goals"`
	}
	err = ui.WithSpinner(ctx, func(ctx context.Context) error {
		return c.GQL(ctx, query, map[string]any{"userId": me.ID}, &resp)
	})
	if err != nil {
		return err
	}
	goals := resp.Goals

	if jsonMode {
		raw, _ := json.MarshalIndent(goals, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	if len(goals) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), styles.Apply(styles.Yellow, "No reading goals found."))
		return nil
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Title, "Reading Goals"))
	fmt.Fprintf(out, "%s\n", styles.Apply(styles.Dim, "────────────────────────────────────────────────────────────"))

	for _, g := range goals {
		pct := 0.0
		if g.Goal > 0 {
			pct = g.Progress / g.Goal * 100
		}
		bar := styles.ProgressBar(pct, 25)

		stateColor := styles.Dim
		switch g.State {
		case "active":
			stateColor = styles.BCyan
		case "completed":
			stateColor = styles.BGreen
		case "failed":
			stateColor = styles.Red
		}
		stateTag := styles.Apply(stateColor, fmt.Sprintf("[%s]", strings.ToUpper(g.State)))

		metricDisplay := titleCase(strings.ReplaceAll(g.Metric, "_", " "))
		fmt.Fprintf(out, "\n%s  %s\n", stateTag, styles.Apply(styles.Bold, metricDisplay+" Goal"))
		fmt.Fprintf(out, "   %s  %s  %s\n",
			bar,
			styles.Apply(styles.Bold, fmt.Sprintf("%.0f%%", pct)),
			styles.Apply(styles.Dim, fmt.Sprintf("(%.0f/%.0f)", g.Progress, g.Goal)),
		)
		fmt.Fprintf(out, "   %s\n",
			styles.Apply(styles.Dim, fmt.Sprintf("Period: %s ➔ %s", g.StartDate, g.EndDate)),
		)
		if g.Description != nil && *g.Description != "" {
			fmt.Fprintf(out, "   %s\n",
				styles.Apply(styles.Dim, "Note: "+*g.Description),
			)
		}
	}
	fmt.Fprintln(out)
	return nil
}

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [query]",
		Short: "Add a book to your library from the catalog",
		Long: `Search the Hardcover catalog for a book, pick a book and edition
from interactive selectors, then add it to your library. At the
end you'll be prompted to set a reading status.

Use --id to add a book directly by its Hardcover book ID, skipping
the search and edition pickers.`,
		Args: func(cmd *cobra.Command, args []string) error {
			id, _ := cmd.Flags().GetInt("id")
			if id == 0 && len(args) == 0 {
				return fmt.Errorf("add requires a book title or --id: %w", errs.ErrInvalid)
			}
			if id != 0 && len(args) > 0 {
				return fmt.Errorf("--id and a book title are mutually exclusive: %w", errs.ErrInvalid)
			}
			return nil
		},
		RunE: runAdd,
	}
	cmd.Flags().Int("id", 0, "Book ID (skips search and edition pickers)")
	cmd.Flags().Int("limit", 10, "Max search results")
	cmd.Flags().Int("edition-limit", 25, "Max editions to fetch")
	cmd.Flags().Int("status", 0, "Reading status ID (required in non-interactive mode)")
	cmd.Flags().Bool("json", false, "Output raw JSON")
	return cmd
}

// addResult is the JSON-friendly summary of an add operation.
type addResult struct {
	Book      string  `json:"book"`
	BookID    int     `json:"book_id"`
	EditionID int     `json:"edition_id"`
	Status    *string `json:"status,omitempty"`
}

func runAdd(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	idFlag, _ := cmd.Flags().GetInt("id")
	limit, _ := cmd.Flags().GetInt("limit")
	editionLimit, _ := cmd.Flags().GetInt("edition-limit")
	statusFlag, _ := cmd.Flags().GetInt("status")
	jsonMode := jsonFromCmd(cmd)
	styles := stylesFromCmd(cmd)

	token, err := config.LoadToken()
	if err != nil {
		return err
	}
	c := api.New(token)

	if !ui.IsInteractive() && statusFlag == 0 {
		return fmt.Errorf("--status is required in non-interactive mode: %w", errs.ErrInvalid)
	}

	var (
		bookID    int
		bookTitle string
		editionID int
	)

	if idFlag != 0 {
		// Direct add by book ID: skip search but still pick edition.
		bookID = idFlag
		bookTitle = fmt.Sprintf("book #%d", bookID)
	} else {
		query := args[0]

		// Step 1: Search the catalog.
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

		// Decode search hits into SearchResult values.
		results := make([]api.SearchResult, 0, len(resp.Search.Results.Hits))
		var dropped int
		for _, hit := range resp.Search.Results.Hits {
			sr := decodeSearchHit(hit)
			if sr != nil {
				results = append(results, *sr)
			} else {
				dropped++
			}
		}

		if len(results) == 0 {
			if jsonMode {
				if dropped > 0 {
					resp := struct {
						Results []api.SearchResult `json:"results"`
						Dropped int                `json:"dropped"`
					}{results, dropped}
					raw, _ := json.MarshalIndent(resp, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				} else {
					raw, _ := json.MarshalIndent(results, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(raw))
				}
			} else {
				msg := fmt.Sprintf("No results for %q.", query)
				if dropped > 0 {
					msg = fmt.Sprintf("No results for %q (%d hits could not be parsed).", query, dropped)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n", styles.Apply(styles.Yellow, msg))
			}
			return nil
		}

		// Step 2: Pick a book.
		chosen, err := ui.SelectSearchResult(ctx, results, styles)
		if err != nil {
			return err
		}
		if chosen == nil {
			return fmt.Errorf("selection cancelled: %w", errs.ErrInvalid)
		}

		bookID, err = strconv.Atoi(chosen.ID)
		if err != nil {
			return fmt.Errorf("invalid book id %q: %w", chosen.ID, errs.ErrInvalid)
		}
		bookTitle = chosen.Title
	}

	// Fetch editions for the book.
	var editionsResp struct {
		Book *struct {
			Editions []api.EditionResult `json:"editions"`
		} `json:"books_by_pk"`
	}
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		return c.GQL(ctx, api.QueryBookEditions, map[string]any{
			"bookId": bookID,
			"limit":  editionLimit,
		}, &editionsResp)
	})
	if err != nil {
		return err
	}

	editions := []api.EditionResult{}
	if editionsResp.Book != nil {
		editions = editionsResp.Book.Editions
	}

	// Pick an edition.
	edition, err := ui.SelectEdition(ctx, editions, styles)
	if err != nil {
		return err
	}
	if edition == nil {
		return fmt.Errorf("selection cancelled: %w", errs.ErrInvalid)
	}
	editionID = edition.ID

	// Insert the book into the library.
	insertResp := struct {
		InsertUserBook *struct {
			ID    *int    `json:"id"`
			Error *string `json:"error"`
		} `json:"insert_user_book"`
	}{}
	insertObj := map[string]any{"book_id": bookID, "edition_id": editionID}
	err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
		return c.GQL(ctx, api.MutationInsertUserBook, map[string]any{
			"object": insertObj,
		}, &insertResp)
	})
	if err != nil {
		return err
	}
	if insertResp.InsertUserBook == nil {
		return fmt.Errorf("failed to add book: unexpected nil response: %w", errs.ErrInvalid)
	}
	if insertResp.InsertUserBook.Error != nil {
		return fmt.Errorf("failed to add book: %s: %w", *insertResp.InsertUserBook.Error, errs.ErrInvalid)
	}
	if insertResp.InsertUserBook.ID == nil {
		return fmt.Errorf("failed to add book: unexpected nil id: %w", errs.ErrInvalid)
	}

	result := addResult{
		Book:      bookTitle,
		BookID:    bookID,
		EditionID: editionID,
	}

	// Pick a status (at the end).
	var statusID int
	if !ui.IsInteractive() {
		statusID = statusFlag
	} else {
		statusID, err = ui.SelectStatus(ctx, styles)
		if err != nil {
			return err
		}
	}
	if statusID > 0 {
		err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
			return c.GQL(ctx, api.MutationUpdateUserBook, map[string]any{
				"id":     *insertResp.InsertUserBook.ID,
				"object": map[string]any{"status_id": statusID},
			}, nil)
		})
		if err != nil {
			return err
		}
		name := ui.StatusName(statusID)
		result.Status = &name
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	out := cmd.OutOrStdout()
	parts := []string{
		styles.Apply(styles.Green, "Added '"),
		styles.Apply(styles.SuccessBold, bookTitle),
		styles.Apply(styles.Green, "' to your library"),
	}
	if result.Status != nil {
		parts = append(parts,
			styles.Apply(styles.Green, " as '"),
			styles.Apply(styles.Cyan, *result.Status),
			styles.Apply(styles.Green, "'"),
		)
	}
	fmt.Fprintln(out, ui.JoinStrings(parts, ""))
	return nil
}

// decodeSearchHit extracts a SearchResult from a raw search hit document.
func decodeSearchHit(h api.SearchHit) *api.SearchResult {
	m := decodeHit(h)
	if m == nil {
		return nil
	}

	sr := &api.SearchResult{
		Rating: asFloat(m["rating"]),
	}

	if id, ok := m["id"].(string); ok {
		sr.ID = id
	} else if id, ok := m["id"].(float64); ok {
		sr.ID = strconv.Itoa(int(id))
	}

	if title, ok := m["title"].(string); ok {
		sr.Title = title
	}

	if authors, ok := m["author_names"].([]any); ok {
		for _, a := range authors {
			if s, ok := a.(string); ok && s != "" {
				sr.AuthorNames = append(sr.AuthorNames, s)
			}
		}
	}

	if pages := asInt(m["pages"]); pages > 0 {
		sr.Pages = &pages
	}
	if year := asInt(m["release_year"]); year > 0 {
		sr.ReleaseYear = &year
	}

	return sr
}

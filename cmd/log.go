package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KIRKR101/hardcover-cli/internal/api"
	"github.com/KIRKR101/hardcover-cli/internal/config"
	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/KIRKR101/hardcover-cli/internal/ui"

	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [book]",
		Short: "Log reading progress, status, or rating for a book",
		Long: `Log reading activity for a book. Match by fuzzy title search, or
use --id to target a specific user_book by id. If multiple books
match the title, an interactive picker is launched in TTY mode.

Provide at least one of: --pages, --percent, --status, --rating.`,
		Args: func(cmd *cobra.Command, args []string) error {
			id, _ := cmd.Flags().GetInt("id")
			if id == 0 && len(args) == 0 {
				return fmt.Errorf("log requires a book title or --id: %w", errs.ErrInvalid)
			}
			if id != 0 && len(args) > 0 {
				return fmt.Errorf("--id and a book title are mutually exclusive: %w", errs.ErrInvalid)
			}
			return nil
		},
		RunE: runLog,
	}
	cmd.Flags().Int("id", 0, "user_book id (skips title search)")
	cmd.Flags().Int("pages", 0, "Log cumulative pages read")
	cmd.Flags().Float64("percent", -1, "Log as percentage of total pages (0-100)")
	cmd.Flags().String("status", "", "Update status (want, reading, read, paused, dnf, ignored)")
	cmd.Flags().Float64("rating", -1, "Rate the book (0-5, supports halves)")
	return cmd
}

// logResult is the JSON-friendly summary of what was logged.
type logResult struct {
	Book         string   `json:"book"`
	UserBookID   int      `json:"user_book_id"`
	PagesLogged  *int     `json:"pages_logged,omitempty"`
	Status       *string  `json:"status,omitempty"`
	Rating       *float64 `json:"rating,omitempty"`
}

func runLog(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	styles := stylesFromCmd(cmd)
	jsonMode := jsonFromCmd(cmd)

	idFlag, _ := cmd.Flags().GetInt("id")
	pages, _ := cmd.Flags().GetInt("pages")
	percent, _ := cmd.Flags().GetFloat64("percent")
	statusArg, _ := cmd.Flags().GetString("status")
	rating, _ := cmd.Flags().GetFloat64("rating")

	// Validate inputs.
	if percent >= 0 && !(percent <= 100) {
		return fmt.Errorf("--percent must be between 0 and 100: %w", errs.ErrInvalid)
	}
	if rating >= 0 && !(rating <= 5) {
		return fmt.Errorf("--rating must be between 0 and 5: %w", errs.ErrInvalid)
	}
	if pages < 0 {
		return fmt.Errorf("--pages cannot be negative: %w", errs.ErrInvalid)
	}
	if pages == 0 && percent < 0 && statusArg == "" && rating < 0 {
		return fmt.Errorf("nothing to log. Use --pages, --percent, --status, or --rating: %w", errs.ErrInvalid)
	}
	pagesSet := pages > 0 || percent >= 0
	hasStatus := statusArg != ""
	hasRating := rating >= 0

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

	// Resolve the target user_book.
	var (
		userBookID int
		bookTitle  string
		bookPages  int
		editionID  *int
	)

	if idFlag != 0 {
		userBookID = idFlag
		bookTitle = fmt.Sprintf("user_book #%d", idFlag)
		if pagesSet {
			// Need book pages and edition_id for percent->pages conversion
			// and for setting the edition on the user_book_read.
			var resp struct {
				UserBook *struct {
					EditionID *int          `json:"edition_id"`
					Edition   *api.Edition  `json:"edition"`
					Book      *api.Book     `json:"book"`
				} `json:"user_books_by_pk"`
			}
			err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
				return c.GQL(ctx, api.QueryUserBookByID, map[string]any{"id": idFlag}, &resp)
			})
			if err != nil {
				return err
			}
			if resp.UserBook == nil {
				return fmt.Errorf("user_book #%d not found: %w", idFlag, errs.ErrInvalid)
			}
			editionID = resp.UserBook.EditionID
			if resp.UserBook.Edition != nil && resp.UserBook.Edition.Pages > 0 {
				bookPages = resp.UserBook.Edition.Pages
			} else if resp.UserBook.Book != nil {
				bookPages = resp.UserBook.Book.Pages
			}
		}
	} else {
		queryStr := strings.ToLower(args[0])
		var allBooks []api.UserBook
		err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
			offset := 0
			for {
				var resp struct {
					UserBooks []api.UserBook `json:"user_books"`
				}
				if gerr := c.GQL(ctx, api.QueryUserBooksByTitle, map[string]any{
					"userId": me.ID,
					"limit":  api.LibraryFetchLimit,
					"offset": offset,
				}, &resp); gerr != nil {
					return gerr
				}
				allBooks = append(allBooks, resp.UserBooks...)
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

		// Filter on the client. The Hardcover API restricts _ilike on
		// book.title, so we fetch the library and match here. We prefer
		// exact matches; otherwise substring matches in title order.
		var matches []api.UserBook
		for _, ub := range allBooks {
			title := strings.ToLower(ub.Book.Title)
			if title == queryStr {
				matches = []api.UserBook{ub}
				break
			}
			if strings.Contains(title, queryStr) {
				matches = append(matches, ub)
			}
		}
		if len(matches) == 0 {
			return fmt.Errorf("no book matching %q in your library: %w", args[0], errs.ErrInvalid)
		}
		if len(matches) == 1 {
			ub := matches[0]
			userBookID = ub.ID
			bookTitle = ub.Book.Title
			editionID = ub.EditionID
			if ub.Edition != nil && ub.Edition.Pages > 0 {
				bookPages = ub.Edition.Pages
			} else {
				bookPages = ub.Book.Pages
			}
		} else {
			// Multiple matches: launch the bubbletea selector.
			if !ui.IsInteractive() {
				return fmt.Errorf("multiple matches for %q; use --id to specify which book: %w", args[0], errs.ErrInvalid)
			}
			chosen, err := ui.SelectBook(ctx, matches, styles)
			if err != nil {
				return err
			}
			if chosen == nil {
				return fmt.Errorf("selection cancelled: %w", errs.ErrInvalid)
			}
			userBookID = chosen.ID
			bookTitle = chosen.Book.Title
			editionID = chosen.EditionID
			if chosen.Edition != nil && chosen.Edition.Pages > 0 {
				bookPages = chosen.Edition.Pages
			} else {
				bookPages = chosen.Book.Pages
			}
		}
	}

	// Apply --percent: convert to pages.
	if percent >= 0 {
		if bookPages <= 0 {
			return fmt.Errorf("could not determine page count for %q; use --pages instead: %w", bookTitle, errs.ErrInvalid)
		}
		pages = int(float64(bookPages) * percent / 100.0)
	}

	result := logResult{Book: bookTitle, UserBookID: userBookID}

	if pagesSet {
		// Fetch the active read, update or insert.
		var readResp struct {
			UserBookReads []struct {
				ID int `json:"id"`
			} `json:"user_book_reads"`
		}
		err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
			return c.GQL(ctx, api.QueryActiveRead, map[string]any{
				"userBookId": userBookID,
			}, &readResp)
		})
		if err != nil {
			return err
		}
		readInput := map[string]any{"progress_pages": pages}
		if editionID != nil {
			readInput["edition_id"] = *editionID
		}

		if len(readResp.UserBookReads) > 0 {
			readID := readResp.UserBookReads[0].ID
			err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
				return c.GQL(ctx, api.MutationUpdateUserBookRead, map[string]any{
					"id":     readID,
					"object": readInput,
				}, nil)
			})
		} else {
			err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
				return c.GQL(ctx, api.MutationInsertUserBookRead, map[string]any{
					"userBookId":   userBookID,
					"userBookRead": readInput,
				}, nil)
			})
		}
		if err != nil {
			return err
		}
		result.PagesLogged = &pages
	}

	if hasStatus {
		statusID, ok := ui.StatusShort[statusArg]
		if !ok {
			return fmt.Errorf("unknown status %q: %w", statusArg, errs.ErrInvalid)
		}
		err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
			return c.GQL(ctx, api.MutationUpdateUserBook, map[string]any{
				"id":     userBookID,
				"object": map[string]any{"status_id": statusID},
			}, nil)
		})
		if err != nil {
			return err
		}
		name := ui.StatusName(statusID)
		result.Status = &name
	}

	if hasRating {
		err = ui.WithSpinner(ctx, jsonMode, func(ctx context.Context) error {
			return c.GQL(ctx, api.MutationUpdateUserBook, map[string]any{
				"id":     userBookID,
				"object": map[string]any{"rating": rating},
			}, nil)
		})
		if err != nil {
			return err
		}
		r := rating
		result.Rating = &r
	}

	if jsonMode {
		raw, _ := json.MarshalIndent(result, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		return nil
	}

	out := cmd.OutOrStdout()
	if result.PagesLogged != nil {
		parts := []string{
			styles.Apply(styles.Green, "Logged "),
			styles.Apply(styles.SuccessBold, fmt.Sprintf("%d", *result.PagesLogged)),
			styles.Apply(styles.Green, " pages for '"),
			styles.Apply(styles.SuccessBold, bookTitle),
			styles.Apply(styles.Green, "'"),
		}
		fmt.Fprintln(out, strings.Join(parts, ""))
	}
	if result.Status != nil {
		parts := []string{
			styles.Apply(styles.Green, "Updated '"),
			styles.Apply(styles.SuccessBold, bookTitle),
			styles.Apply(styles.Green, "' status to '"),
			styles.Apply(styles.Cyan, *result.Status),
			styles.Apply(styles.Green, "'"),
		}
		fmt.Fprintln(out, strings.Join(parts, ""))
	}
	if result.Rating != nil {
		parts := []string{
			styles.Apply(styles.Green, "Rated '"),
			styles.Apply(styles.SuccessBold, bookTitle),
			styles.Apply(styles.Green, "' "),
			styles.Apply(styles.BYellow, fmt.Sprintf("★ %.1f", *result.Rating)),
			styles.Apply(styles.Green, "/5"),
		}
		fmt.Fprintln(out, strings.Join(parts, ""))
	}
	return nil
}

// Package api wraps the Hardcover GraphQL API.
package api

import "encoding/json"

// User is the authenticated user (from the me query).
type User struct {
	ID         int    `json:"id"`
	Username   string `json:"username"`
	BooksCount int    `json:"books_count"`
}

// Author is a book contributor.
type Author struct {
	Name string `json:"name"`
}

// Contribution associates an author with a book.
type Contribution struct {
	Author Author `json:"author"`
}

// Edition is a particular edition of a book (pages, etc.).
type Edition struct {
	ID    int `json:"id"`
	Pages int `json:"pages"`
}

// Book is a title in Hardcover's catalog.
type Book struct {
	ID           int           `json:"id"`
	Title        string        `json:"title"`
	Pages        int           `json:"pages"`
	Contributions []Contribution `json:"contributions"`
}

// UserBook is a user's relationship with a book (status, rating, etc.).
type UserBook struct {
	ID        int     `json:"id"`
	EditionID *int    `json:"edition_id"`
	StatusID  int     `json:"status_id"`
	Rating    float64 `json:"rating"`
	DateAdded string  `json:"date_added"`
	Owned     bool    `json:"owned"`
	Edition   *Edition `json:"edition"`
	Book      Book    `json:"book"`

	// Populated for the progress command: the active reading session.
	UserBookReads []UserBookRead `json:"user_book_reads"`
}

// UserBookRead is an in-progress reading session.
type UserBookRead struct {
	ID             int     `json:"id"`
	ProgressPages  int     `json:"progress_pages"`
	StartedAt      *string `json:"started_at"`
	FinishedAt     *string `json:"finished_at"`
}

// ReadingJournal is a reading event.
type ReadingJournal struct {
	BookID   int            `json:"book_id"`
	ActionAt string         `json:"action_at"`
	Metadata map[string]any `json:"metadata"`
}

// Journal metadata is read inline via anonymous structs in the
// command files; the shape is `{progress_pages, progress_pages_was}`
// but we keep it inline so the metadata field on ReadingJournal can
// be left as `map[string]any` without forcing a specific type.

// Goal is a reading goal.
type Goal struct {
	ID          int     `json:"id"`
	Metric      string  `json:"metric"`
	Goal        float64 `json:"goal"`
	Progress    float64 `json:"progress"`
	State       string  `json:"state"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	Description *string `json:"description"`
}

// Aggregate is the aggregate result of a count query.
type Aggregate struct {
	Count int `json:"count"`
}

// AggregateWithAvg is the aggregate result of a count+avg query.
type AggregateWithAvg struct {
	Count int `json:"count"`
	Avg   *struct {
		Rating float64 `json:"rating"`
	} `json:"avg"`
}

// SearchHit is a single search result.
type SearchHit struct {
	Document json.RawMessage `json:"document"`
}

// SearchResults is the inner payload of a search query.
type SearchResults struct {
	Hits []SearchHit `json:"hits"`
}

// Search is the search query response.
type Search struct {
	Results SearchResults `json:"results"`
}

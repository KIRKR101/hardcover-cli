package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/KIRKR101/hardcover-cli/internal/errs"
)

// APIURL is the Hardcover GraphQL endpoint.
const APIURL = "https://api.hardcover.app/v1/graphql"

// LibraryFetchLimit is the page size for library-style queries.
const LibraryFetchLimit = 1000

// JournalFetchLimit is the page size for journal queries.
const JournalFetchLimit = 100

// RequestTimeout is the default per-request timeout.
const RequestTimeout = 30 * time.Second

// Client is a Hardcover GraphQL client. The HTTPClient field is exported
// so tests can inject *http.Client with a custom Transport. The token
// is captured at construction; callers should not mutate it.
type Client struct {
	HTTPClient *http.Client
	APIURL     string
	Token      string
}

// New returns a Client configured with sensible defaults.
func New(token string) *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: RequestTimeout},
		APIURL:     APIURL,
		Token:      token,
	}
}

// GQL executes a GraphQL query/mutation against the API and decodes
// the response into target. The context is forwarded to the HTTP
// request so callers can wire timeouts or cancellation (e.g. Ctrl-C).
//
// Errors are wrapped with the appropriate sentinel:
//   - 401/403           -> errs.ErrAuthFailed
//   - timeout / refused -> errs.ErrNetError
//   - non-JSON body     -> errs.ErrNetError
//   - API errors[]      -> errs.ErrInvalid
//   - 4xx (other)       -> errs.ErrInvalid
//   - 5xx               -> errs.ErrNetError
func (c *Client) GQL(ctx context.Context, query string, variables map[string]any, target any) error {
	body, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": variables,
	})
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// Timeouts, refused connections, DNS errors etc. all land here.
		return fmt.Errorf("request hardcover api: %w", joinErr(errs.ErrNetError, err))
	}
	defer resp.Body.Close()

	// HTTP status mapping. The API returns 401/403 for auth failures even
	// though the connection itself is fine, so we treat those as auth
	// errors, not network errors. 4xx (other than auth) is invalid input;
	// 5xx is a server-side problem.
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("invalid or expired api token (http %d): %w", resp.StatusCode, errs.ErrAuthFailed)
	case resp.StatusCode >= 500:
		return fmt.Errorf("hardcover api returned http %d: %w", resp.StatusCode, errs.ErrNetError)
	case resp.StatusCode >= 400:
		return fmt.Errorf("hardcover api rejected request (http %d): %w", resp.StatusCode, errs.ErrInvalid)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", joinErr(errs.ErrNetError, err))
	}

	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("decode response: %w", joinErr(errs.ErrNetError, err))
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("api error: %s: %w", env.Errors[0].Message, errs.ErrInvalid)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return fmt.Errorf("empty response: %w", errs.ErrInvalid)
	}
	if target != nil {
		if err := json.Unmarshal(env.Data, target); err != nil {
			return fmt.Errorf("decode data: %w", err)
		}
	}
	return nil
}

// joinErr wraps inner with sentinel using %w (so errors.Is matches
// the sentinel) and tacks inner's text on with %v. If inner is nil,
// returns the sentinel alone.
func joinErr(sentinel, inner error) error {
	if inner == nil {
		return sentinel
	}
	return fmt.Errorf("%w: %v", sentinel, inner)
}

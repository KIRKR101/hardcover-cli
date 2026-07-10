package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KIRKR101/hardcover-cli/internal/errs"
)

// TestGQL_AuthMapping verifies that 401/403 from the API map to
// errs.ErrAuthFailed (not ErrNetError). 4xx is invalid input, 5xx is
// network, but auth failures arrive over a perfectly good connection
// and should be distinguishable for scripts that care.
func TestGQL_AuthMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIURL:     srv.URL,
		Token:      "fake",
	}
	err := c.GQL(context.Background(), "{ me { id } }", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errs.ErrAuthFailed) {
		t.Fatalf("expected errs.ErrAuthFailed, got %v", err)
	}
}

func TestGQL_5xxMapsToNetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIURL:     srv.URL,
		Token:      "fake",
	}
	err := c.GQL(context.Background(), "{ me { id } }", nil, nil)
	if !errors.Is(err, errs.ErrNetError) {
		t.Fatalf("expected errs.ErrNetError for 5xx, got %v", err)
	}
}

func TestGQL_4xxMapsToInvalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"message":"bad"}]}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIURL:     srv.URL,
		Token:      "fake",
	}
	err := c.GQL(context.Background(), "{ me { id } }", nil, nil)
	if !errors.Is(err, errs.ErrInvalid) {
		t.Fatalf("expected errs.ErrInvalid for 4xx, got %v", err)
	}
}

func TestGQL_ValidResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"me":[{"id":1,"username":"x","books_count":7}]}}`))
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIURL:     srv.URL,
		Token:      "fake",
	}
	var resp struct {
		Me []User `json:"me"`
	}
	if err := c.GQL(context.Background(), "{ me { id } }", nil, &resp); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Me) != 1 || resp.Me[0].ID != 1 || resp.Me[0].BooksCount != 7 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGQL_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := &Client{
		HTTPClient: srv.Client(),
		APIURL:     srv.URL,
		Token:      "fake",
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := c.GQL(ctx, "{ me { id } }", nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// Package errs defines sentinel errors used across the CLI for
// error-code mapping. Centralised here to avoid import cycles
// between the api and ui packages.
package errs

import "errors"

var (
	// ErrNoToken: no API token is configured. Caller should run `hardcover setup`.
	ErrNoToken = errors.New("no API token")

	// ErrAuthFailed: the API rejected the bearer token (HTTP 401/403).
	ErrAuthFailed = errors.New("authentication failed")

	// ErrNetError: network-level failure (timeout, connection refused,
	// non-JSON response, 5xx).
	ErrNetError = errors.New("network error")

	// ErrInvalid: bad input or API-side validation error (HTTP 4xx other
	// than 401/403, GraphQL errors[] payload).
	ErrInvalid = errors.New("invalid input")
)

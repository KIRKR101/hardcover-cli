package ui

import (
	"errors"
	"fmt"
	"os"

	"github.com/KIRKR101/hardcover-cli/internal/errs"
	"github.com/charmbracelet/lipgloss"
)

// Exit codes used by the CLI. Returned via os.Exit at the entrypoint.
const (
	ExitOK    = 0
	ExitError = 1 // generic error, bad input, validation
	ExitAuth  = 2 // missing/invalid token
	ExitNet   = 3 // network errors, timeouts, 5xx
)

// ExitCodeFor returns the appropriate exit code for the given error.
// It unwraps the error chain looking for known sentinels and falls back
// to ExitError for anything else.
//
// IMPORTANT: every call site that wants to surface a sentinel must wrap
// with %w (fmt.Errorf("...: %w", errs.ErrNoToken)), never %v. errors.Is
// unwraps the %w chain; %v produces a fresh string error that won't match.
func ExitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	switch {
	case errors.Is(err, errs.ErrNoToken), errors.Is(err, errs.ErrAuthFailed):
		return ExitAuth
	case errors.Is(err, errs.ErrNetError):
		return ExitNet
	default:
		return ExitError
	}
}

// Exit prints the error to stderr (if any) and exits with the appropriate code.
func Exit(err error) {
	if err == nil {
		os.Exit(ExitOK)
	}
	if HasColor(false) {
		red := lipgloss.NewStyle().Foreground(lipgloss.Color("167")).Bold(true)
		fmt.Fprintln(os.Stderr, red.Render("error:")+" "+err.Error())
	} else {
		fmt.Fprintln(os.Stderr, "error: "+err.Error())
	}
	os.Exit(ExitCodeFor(err))
}

package ui

import (
	"context"
	"os"
	"time"

	"github.com/briandowns/spinner"
)

// WithSpinner displays a terminal spinner while fn runs. The spinner is
// suppressed when stdout is not a TTY or when --json is set.
func WithSpinner(ctx context.Context, fn func(context.Context) error) error {
	if !IsInteractive() {
		return fn(ctx)
	}
	s := spinner.New(spinner.CharSets[26], 80*time.Millisecond,
		spinner.WithWriter(os.Stderr),
		spinner.WithColor("cyan"),
	)
	s.Start()
	defer s.Stop()
	return fn(ctx)
}

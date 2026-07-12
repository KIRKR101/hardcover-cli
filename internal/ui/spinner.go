package ui

import (
	"context"
	"os"
	"time"

	"github.com/briandowns/spinner"
)

// WithSpinner displays a terminal spinner while fn runs. The spinner is
// suppressed when stdout is not a TTY or when jsonMode is true.
func WithSpinner(ctx context.Context, jsonMode bool, fn func(context.Context) error) error {
	return WithSpinnerMsg(ctx, jsonMode, "", fn)
}

// WithSpinnerMsg displays a terminal spinner with an optional message
// while fn runs. The spinner is suppressed when stdout is not a TTY
// or when jsonMode is true.
func WithSpinnerMsg(ctx context.Context, jsonMode bool, msg string, fn func(context.Context) error) error {
	if !ShouldSpinner(jsonMode) {
		return fn(ctx)
	}
	s := spinner.New(spinner.CharSets[26], 80*time.Millisecond,
		spinner.WithWriter(os.Stderr),
		spinner.WithColor("cyan"),
	)
	if msg != "" {
		s.Suffix = " " + msg
	}
	s.Start()
	defer s.Stop()
	return fn(ctx)
}

package ui

import "context"

// WithSpinner is a thin pass-through. Originally wrapped yacspin for
// visual feedback during API calls, but the ticks/animation were
// dropped in favour of silent operation. The function signature is
// kept so call sites remain stable.
func WithSpinner(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

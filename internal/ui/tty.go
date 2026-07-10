package ui

import (
	"os"

	"golang.org/x/term"
)

// isatty reports whether f is a terminal. Exposed for testing.
var isatty = func(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// IsInteractive reports whether stdin and stdout are both terminals.
// Use for commands that prompt the user interactively.
func IsInteractive() bool {
	return isatty(os.Stdin) && isatty(os.Stdout)
}

// HasColor reports whether colored output is appropriate.
// True when stdout is a terminal, NO_COLOR is not set,
// and --no-color was not passed (caller passes disableColor).
func HasColor(disableColor bool) bool {
	if disableColor {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isatty(os.Stdout)
}

// ShouldSpinner reports whether a yacspin spinner should be shown.
// True when stderr is a terminal and --json was not passed.
func ShouldSpinner(jsonMode bool) bool {
	if jsonMode {
		return false
	}
	return isatty(os.Stderr)
}


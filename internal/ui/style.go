package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorOn returns lipgloss with colors if hasColor is true, plain otherwise.
// Stash a package-level "Styles" struct as well as per-call adaptive renderers.
type Styles struct {
	Color bool

	Title   lipgloss.Style
	Bold    lipgloss.Style
	Dim     lipgloss.Style
	Green   lipgloss.Style
	Red     lipgloss.Style
	Yellow  lipgloss.Style
	Cyan    lipgloss.Style
	Magenta lipgloss.Style
	BGreen  lipgloss.Style
	BYellow lipgloss.Style
	BCyan   lipgloss.Style
}

// NewStyles returns styles with colors enabled/disabled.
func NewStyles(color bool) *Styles {
	s := &Styles{Color: color}
	if !color {
		return s
	}
	s.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("150"))
	s.Bold = lipgloss.NewStyle().Bold(true)
	s.Dim = lipgloss.NewStyle().Foreground(lipgloss.Color("253"))
	s.Green = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
	s.Red = lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	s.Yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("222"))
	s.Cyan = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	s.Magenta = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	s.BGreen = lipgloss.NewStyle().Foreground(lipgloss.Color("150"))
	s.BYellow = lipgloss.NewStyle().Foreground(lipgloss.Color("228"))
	s.BCyan = lipgloss.NewStyle().Foreground(lipgloss.Color("239"))
	return s
}

// Apply wraps a string with a color style if colors are enabled.
func (s *Styles) Apply(style lipgloss.Style, str string) string {
	if !s.Color {
		return str
	}
	return style.Render(str)
}

// StatusColor returns the appropriate color style for a Hardcover status id.
// 1=Want, 2=Reading, 3=Read, 4=Paused, 5=DNF, 6=Ignored.
func (s *Styles) StatusColor(statusID int) lipgloss.Style {
	if !s.Color {
		return lipgloss.NewStyle()
	}
	switch statusID {
	case 1:
		return s.Cyan
	case 2:
		return s.BYellow
	case 3:
		return s.BGreen
	case 4:
		return s.Magenta
	case 5:
		return s.Red
	case 6:
		return s.Dim
	default:
		return lipgloss.NewStyle()
	}
}

// ProgressBar renders a percentage progress bar with a color gradient.
// pct is 0-100, width is the number of characters in the bar.
func (s *Styles) ProgressBar(pct float64, width int) string {
	if width <= 0 {
		width = 20
	}
	pct = clamp(pct, 0, 100)
	filledWidth := (pct / 100.0) * float64(width)
	filledChars := int(filledWidth)
	frac := filledWidth - float64(filledChars)

	// Sub-block characters for granular progress.
	blocks := []rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉'}
	idx := int(frac * 8)
	if idx >= len(blocks) {
		idx = len(blocks) - 1
	}

	bar := strings.Repeat("█", filledChars)
	if filledChars < width {
		bar += string(blocks[idx]) + strings.Repeat(" ", width-filledChars-1)
	}

	// Color by completion.
	var style lipgloss.Style
	switch {
	case pct >= 75:
		style = s.BGreen
	case pct >= 25:
		style = s.BYellow
	default:
		style = s.Red
	}
	return s.Apply(style, bar)
}

// StatusName returns a human-readable status name.
func StatusName(statusID int) string {
	switch statusID {
	case 1:
		return "Want to Read"
	case 2:
		return "Currently Reading"
	case 3:
		return "Read"
	case 4:
		return "Paused"
	case 5:
		return "Did Not Finish"
	case 6:
		return "Ignored"
	default:
		return "?"
	}
}

// StatusShort maps short keys to status ids.
var StatusShort = map[string]int{
	"want":    1,
	"reading": 2,
	"read":    3,
	"paused":  4,
	"dnf":     5,
	"ignored": 6,
}

// PadRight pads s with spaces to width, ignoring ANSI.
func PadRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-visibleLen(s)))
}

// visibleLen returns the visible (non-ANSI) length of s.
func visibleLen(s string) int {
	n := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		n++
	}
	return n
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Truncate returns s truncated to n runes, with an ellipsis if truncated.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 1 {
		return string(runes[:n])
	}
	return string(runes[:n-1]) + "…"
}

// Success returns a green-styled message with no prefix glyph.
func (s *Styles) Success(msg string) string {
	return s.Apply(s.Green, msg)
}

// Bullet returns a cyan bullet point.
func (s *Styles) Bullet() string {
	return s.Apply(s.BCyan, "●")
}

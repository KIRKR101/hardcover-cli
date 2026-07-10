package ui

import "strings"

// JoinStrings joins a slice with a separator, ignoring empty entries.
func JoinStrings(parts []string, sep string) string {
	out := strings.Builder{}
	wrote := false
	for _, p := range parts {
		if p == "" {
			continue
		}
		if wrote {
			out.WriteString(sep)
		}
		out.WriteString(p)
		wrote = true
	}
	return out.String()
}

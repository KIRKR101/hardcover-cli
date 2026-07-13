package filter

import (
	"testing"

	"github.com/KIRKR101/hardcover-cli/internal/api"
)

func TestParse(t *testing.T) {
	tests := []string{
		"rating>=4",
		"status=reading",
		"title~'philosophy'",
		"rating>=4 AND year=2026",
		"(status=reading OR status=want) AND rating>=3",
		"author~'^Fyodor'",
		"owned=true",
		"pages>300 AND added>=2024-01-01",
		"NOT status=want",
	}

	for _, input := range tests {
		expr, err := Parse(input)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", input, err)
			continue
		}
		if expr == nil {
			t.Errorf("Parse(%q) returned nil", input)
		}
	}
}

func TestEval(t *testing.T) {
	ub := api.UserBook{
		StatusID:  2,
		Rating:    4.5,
		Owned:     true,
		DateAdded: "2024-06-15",
		Book: api.Book{
			Title: "The Brothers Karamazov",
			Pages: 796,
			Contributions: []api.Contribution{
				{Author: api.Author{Name: "Fyodor Dostoevsky"}},
			},
		},
	}

	tests := []struct {
		filter string
		want   bool
	}{
		{"rating>=4", true},
		{"rating>=5", false},
		{"status=reading", true},
		{"status=want", false},
		{"owned=true", true},
		{"owned=false", false},
		{"title~'Brothers'", true},
		{"title~'War'", false},
		{"author~'Dostoevsky'", true},
		{"author~'^Fyodor'", true},
		{"pages>500", true},
		{"pages<500", false},
		{"year=2024", true},
		{"year>2023", true},
		{"added>=2024-01-01", true},
		{"added<2024-01-01", false},
		{"rating>=4 AND status=reading", true},
		{"rating>=4 AND status=want", false},
		{"status=reading OR status=want", true},
		{"status=paused OR status=want", false},
		{"NOT status=want", true},
		{"NOT status=reading", false},
	}

	for _, tt := range tests {
		expr, err := Parse(tt.filter)
		if err != nil {
			t.Errorf("Parse(%q) failed: %v", tt.filter, err)
			continue
		}

		got, err := Eval(expr, ub)
		if err != nil {
			t.Errorf("Eval(%q) failed: %v", tt.filter, err)
			continue
		}

		if got != tt.want {
			t.Errorf("Eval(%q) = %v, want %v", tt.filter, got, tt.want)
		}
	}
}

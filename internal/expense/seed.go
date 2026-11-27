package expense

import (
	"errors"
	"fmt"
	"strings"
)

// Seed is what a new chat starts with. Keywords maps a lowercase note word to
// a category name in Categories; Fallback is used when nothing matches.
type Seed struct {
	Categories []string
	Keywords   map[string]string
	Fallback   string
}

// DefaultSeed is the built-in starting set of categories and keywords.
func DefaultSeed() Seed {
	return Seed{
		Categories: []string{"Food", "Transport", "Bills", "Shopping", "Health", "Entertainment", "Other"},
		Keywords: map[string]string{
			"kopi": "Food", "makan": "Food",
			"gojek": "Transport", "grab": "Transport", "bensin": "Transport",
			"listrik": "Bills", "pulsa": "Bills", "wifi": "Bills",
		},
		Fallback: "Other",
	}
}

func (s Seed) validate() error {
	names := make(map[string]bool, len(s.Categories))
	for _, c := range s.Categories {
		if c == "" {
			return errors.New("empty category name")
		}
		names[strings.ToLower(c)] = true
	}
	if !names[strings.ToLower(s.Fallback)] {
		return fmt.Errorf("fallback %q is not a category", s.Fallback)
	}
	for kw, c := range s.Keywords {
		if kw != strings.ToLower(kw) || strings.ContainsAny(kw, " \t\n") || kw == "" {
			return fmt.Errorf("keyword %q must be one lowercase word", kw)
		}
		if !names[strings.ToLower(c)] {
			return fmt.Errorf("keyword %q maps to unknown category %q", kw, c)
		}
	}
	return nil
}

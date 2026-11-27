// Package parser turns a chat message into an expense entry (FR-01..04).
// It is pure: no I/O, and the current time is passed in.
package parser

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// MaxAmount is the largest accepted amount in rupiah (FR-02).
const MaxAmount int64 = 1_000_000_000

var (
	ErrNoAmount    = errors.New("no valid amount")
	ErrInvalidDate = errors.New("invalid date")
)

// Entry is a parsed expense. Tag is the raw #tag text without '#', empty if absent;
// mapping it (or the note) to a category is the caller's job (FR-11/12).
type Entry struct {
	Amount  int64
	Note    string
	Tag     string
	SpentAt time.Time
}

type span struct {
	start, end int // token indices, end exclusive
	amount     int64
	suffixed   bool
}

// Parse reads one expense from text. now must be in the user's time zone;
// date overrides resolve to 12:00 in now's location (FR-04).
//
// O(t) in the number of tokens.
func Parse(text string, now time.Time) (Entry, error) {
	tokens := strings.Fields(text)
	used := make([]bool, len(tokens))
	e := Entry{SpentAt: now}
	var best span
	found := false

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		lower := strings.ToLower(tok)
		switch {
		case lower == "kemarin" || lower == "yesterday":
			y, m, d := now.Date()
			e.SpentAt = time.Date(y, m, d-1, 12, 0, 0, 0, now.Location())
			used[i] = true
		case isDateToken(tok):
			d, err := time.ParseInLocation(time.DateOnly, tok[1:], now.Location())
			if err != nil {
				return Entry{}, fmt.Errorf("%w: %q", ErrInvalidDate, tok)
			}
			e.SpentAt = time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, now.Location())
			used[i] = true
		case len(tok) > 1 && tok[0] == '#':
			e.Tag = tok[1:]
			used[i] = true
		default:
			c, ok := amountAt(tokens, i)
			if !ok {
				continue
			}
			// FR-03: the last suffixed amount wins; otherwise the last plain one.
			if c.suffixed || !best.suffixed {
				best, found = c, true
			}
			i = c.end - 1
		}
	}
	if !found {
		return Entry{}, ErrNoAmount
	}
	for i := best.start; i < best.end; i++ {
		used[i] = true
	}

	note := make([]string, 0, len(tokens))
	for i, tok := range tokens {
		if !used[i] {
			note = append(note, tok)
		}
	}
	e.Amount = best.amount
	e.Note = strings.Join(note, " ")
	return e, nil
}

func isDateToken(tok string) bool {
	return len(tok) > 1 && tok[0] == '@' && isDigit(tok[1])
}

// amountAt tries to read an amount starting at tokens[i], either one token
// ("25k") or a number followed by a separate suffix word ("25 ribu").
func amountAt(tokens []string, i int) (span, bool) {
	num, suffix := splitNumber(strings.ToLower(tokens[i]))
	if num == "" {
		return span{}, false
	}
	end := i + 1
	if suffix == "" && end < len(tokens) {
		next := strings.ToLower(tokens[end])
		if m, ok := multiplier(next); ok && m > 1 {
			suffix = next
			end++
		}
	}
	amount, ok := parseAmount(num, suffix)
	if !ok {
		return span{}, false
	}
	return span{start: i, end: end, amount: amount, suffixed: suffix != ""}, true
}

// splitNumber splits "1,2jt" into ("1,2", "jt"). num is empty if tok does not start with a digit.
func splitNumber(tok string) (num, suffix string) {
	if tok == "" || !isDigit(tok[0]) {
		return "", ""
	}
	n := strings.IndexFunc(tok, func(r rune) bool { return (r < '0' || r > '9') && r != '.' && r != ',' })
	if n < 0 {
		return tok, ""
	}
	return tok[:n], tok[n:]
}

func multiplier(suffix string) (int64, bool) {
	switch suffix {
	case "":
		return 1, true
	case "k", "rb", "ribu":
		return 1_000, true
	case "jt", "juta":
		return 1_000_000, true
	}
	return 0, false
}

// parseAmount converts a number with Indonesian separators to integer rupiah
// using integer maths only. '.' groups thousands ("1.250.000"); ',' is the
// decimal mark. Before a suffix, a lone '.' not followed by exactly three
// digits is read as a decimal mark too ("1.2 juta"). Results that are not
// whole rupiah or fall outside 1..MaxAmount are rejected.
func parseAmount(num, suffix string) (int64, bool) {
	mult, ok := multiplier(suffix)
	if !ok {
		return 0, false
	}
	intPart, frac, hasComma := strings.Cut(num, ",")
	if hasComma && frac == "" {
		return 0, false
	}

	groups := strings.Split(intPart, ".")
	switch {
	case len(groups) == 1:
	case isThousandsGrouped(groups):
		intPart = strings.Join(groups, "")
	case len(groups) == 2 && suffix != "" && !hasComma:
		intPart, frac = groups[0], groups[1]
	default:
		return 0, false
	}

	// Length caps keep every product below int64 overflow.
	whole, ok := digitsValue(intPart, 10)
	if !ok {
		return 0, false
	}
	amount := whole * mult
	if frac != "" {
		f, ok := digitsValue(frac, 6)
		if !ok {
			return 0, false
		}
		scale := int64(1)
		for range len(frac) {
			scale *= 10
		}
		if (f*mult)%scale != 0 {
			return 0, false
		}
		amount += f * mult / scale
	}
	if amount < 1 || amount > MaxAmount {
		return 0, false
	}
	return amount, true
}

func isThousandsGrouped(groups []string) bool {
	if len(groups[0]) < 1 || len(groups[0]) > 3 {
		return false
	}
	for _, g := range groups[1:] {
		if len(g) != 3 {
			return false
		}
	}
	return true
}

// digitsValue parses 1..maxLen ASCII digits; anything else is rejected.
func digitsValue(s string, maxLen int) (int64, bool) {
	if s == "" || len(s) > maxLen {
		return 0, false
	}
	var v int64
	for i := range len(s) {
		if !isDigit(s[i]) {
			return 0, false
		}
		v = v*10 + int64(s[i]-'0')
	}
	return v, true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

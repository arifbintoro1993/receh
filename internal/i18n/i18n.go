// Package i18n holds every user-facing string, one catalog file per language,
// plus the shared rupiah formatter. Forks add a language by adding a catalog.
package i18n

import (
	"fmt"
	"strconv"
	"strings"
)

type Key int

const (
	// Logged confirms a saved expense. Args: amount, category, note, date.
	Logged Key = iota
	// Usage explains the message format with two examples.
	Usage
	// InvalidDate rejects an @YYYY-MM-DD that is not a real date.
	InvalidDate
	// UnknownCommand answers a command the bot does not support yet.
	UnknownCommand
	// InternalError is shown when saving fails; the user may retry.
	InternalError

	numKeys
)

type Translator struct {
	msgs map[Key]string
}

// New returns the catalog for locale ("id" or "en").
func New(locale string) (Translator, error) {
	var msgs map[Key]string
	switch locale {
	case "id":
		msgs = indonesian()
	case "en":
		msgs = english()
	default:
		return Translator{}, fmt.Errorf("no catalog for locale %q", locale)
	}
	return Translator{msgs: msgs}, nil
}

// T renders the message for k with args.
func (t Translator) T(k Key, args ...any) string {
	return fmt.Sprintf(t.msgs[k], args...)
}

// Rupiah formats an amount as Rp1.250.000 (dot thousands, no decimals).
func Rupiah(n int64) string {
	sign := ""
	if n < 0 {
		sign = "-"
	}
	digits := strconv.FormatUint(absUint(n), 10)

	var b strings.Builder
	b.Grow(len(sign) + 2 + len(digits) + len(digits)/3)
	b.WriteString(sign)
	b.WriteString("Rp")
	for i := range len(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(digits[i])
	}
	return b.String()
}

// absUint avoids overflow on math.MinInt64.
func absUint(n int64) uint64 {
	if n < 0 {
		return uint64(-(n + 1)) + 1
	}
	return uint64(n)
}

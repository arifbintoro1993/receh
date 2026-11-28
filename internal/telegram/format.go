package telegram

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/period"
	"github.com/arifbintoro1993/receh/internal/report"
)

const (
	// maxMessageLen is Telegram's text limit, counted in UTF-16 code units.
	// Longer messages are rejected outright, so replies are cut to fit.
	maxMessageLen = 4096
	maxNoteRunes  = 40
	// Category names come from user #tags and can be any length.
	maxCategoryRunes = 30
)

func formatLogged(m i18n.Translator, e expense.Expense) string {
	return m.T(i18n.Logged, i18n.Rupiah(e.Amount), shorten(e.Category.Name, maxCategoryRunes),
		noteOrDash(e.Note), e.SpentAt.Format(time.DateOnly))
}

// formatSummary renders a period summary, categories largest first. If the
// categories do not all fit in one message, the list ends with "…".
func formatSummary(m i18n.Translator, label i18n.Key, p period.Period, s report.Summary) string {
	header := m.T(i18n.SummaryHeader, m.T(label), dateRange(p))
	if s.Count == 0 {
		return header + "\n" + m.T(i18n.NoExpenses)
	}
	head := header + "\n" + m.T(i18n.SummaryTotal, i18n.Rupiah(s.Total), s.Count) + "\n"
	lines := make([]string, len(s.Categories))
	for i, c := range s.Categories {
		lines[i] = m.T(i18n.SummaryCategory, shorten(c.Name, maxCategoryRunes), i18n.Rupiah(c.Total), c.Count)
	}
	text, used := fit(head, lines, maxMessageLen)
	if used < len(lines) {
		const more = "\n…"
		text, _ = fit(head, lines[:used], maxMessageLen-utf16Len(more))
		text += more
	}
	return text
}

// formatLast renders recent expenses, newest first. The header counts only
// the lines that fit in one message.
func formatLast(m i18n.Translator, expenses []expense.Expense) string {
	if len(expenses) == 0 {
		return m.T(i18n.NoExpenses)
	}
	lines := make([]string, len(expenses))
	for i, e := range expenses {
		lines[i] = m.T(i18n.LastLine, e.SpentAt.Format(time.DateOnly), i18n.Rupiah(e.Amount),
			shorten(e.Category.Name, maxCategoryRunes), shorten(noteOrDash(e.Note), maxNoteRunes))
	}
	text, used := fit(m.T(i18n.LastHeader, len(lines)), lines, maxMessageLen)
	if used < len(lines) {
		// A smaller count never makes the header longer, so this still fits.
		text, _ = fit(m.T(i18n.LastHeader, used), lines[:used], maxMessageLen)
	}
	return text
}

// fit appends lines to head, one per line, while the text stays within limit.
// It returns the text and how many lines fit. O(total length).
func fit(head string, lines []string, limit int) (string, int) {
	var b strings.Builder
	b.WriteString(head)
	n := utf16Len(head)
	for i, line := range lines {
		ln := 1 + utf16Len(line)
		if n+ln > limit {
			return b.String(), i
		}
		b.WriteByte('\n')
		b.WriteString(line)
		n += ln
	}
	return b.String(), len(lines)
}

// utf16Len counts s the way Telegram does: runes outside the BMP (most
// emoji) take two code units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// dateRange shows a period's inclusive dates; a single day shows once.
func dateRange(p period.Period) string {
	first, last := p.Start.Format(time.DateOnly), p.LastDay().Format(time.DateOnly)
	if first == last {
		return first
	}
	return first + " – " + last
}

func noteOrDash(note string) string {
	if note == "" {
		return "—"
	}
	return note
}

// shorten cuts s to at most n runes, marking the cut with an ellipsis.
func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}

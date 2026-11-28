package telegram

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/arifbintoro1993/receh/internal/expense"
	"github.com/arifbintoro1993/receh/internal/i18n"
	"github.com/arifbintoro1993/receh/internal/period"
	"github.com/arifbintoro1993/receh/internal/report"
)

func jakarta(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestHandleSummaryCommands(t *testing.T) {
	jkt := jakarta(t)
	week := period.Week(time.Date(2026, 11, 28, 9, 0, 0, 0, jkt))
	sum := report.Summary{Total: 1_270_000, Count: 3, Categories: []report.CategoryTotal{
		{Name: "Bills", Total: 1_200_000, Count: 1},
		{Name: "Food", Total: 70_000, Count: 2},
	}}

	tests := []struct {
		text     string
		wantSpan expense.Span
		label    string
	}{
		{text: "/today", wantSpan: expense.Today, label: "Today"},
		{text: "/week", wantSpan: expense.ThisWeek, label: "This week"},
		{text: "/MONTH@receh_bot", wantSpan: expense.ThisMonth, label: "This month"},
		{text: "/cycle", wantSpan: expense.ThisCycle, label: "This cycle"},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			hs := newHarness(t)
			hs.sumPeriod, hs.sumResult = week, sum
			u := msg(1, allowedChat, tt.text)

			hs.h.Handle(context.Background(), u)

			if hs.gotSpan != tt.wantSpan || !hs.gotSpanAt.Equal(u.SentAt) {
				t.Errorf("summarized %q at %v, want %q at message time", hs.gotSpan, hs.gotSpanAt, tt.wantSpan)
			}
			want := "📊 " + tt.label + " (2026-11-23 – 2026-11-29)\n" +
				"Total: Rp1.270.000 · 3 transactions\n\n" +
				"• Bills: Rp1.200.000 (1)\n" +
				"• Food: Rp70.000 (2)"
			if r := hs.replies(); len(r) != 1 || r[0] != want {
				t.Errorf("reply =\n%s\nwant\n%s", strings.Join(r, "\n---\n"), want)
			}
		})
	}
}

func TestHandleLastCommand(t *testing.T) {
	jkt := jakarta(t)
	expenses := []expense.Expense{
		{Amount: 25_000, Note: "kopi", Category: expense.Category{Name: "Food"}, SpentAt: time.Date(2026, 11, 28, 9, 0, 0, 0, jkt)},
		{Amount: 1_200_000, Note: "", Category: expense.Category{Name: "Bills"}, SpentAt: time.Date(2026, 11, 27, 12, 0, 0, 0, jkt)},
	}

	tests := []struct {
		text  string
		wantN int
	}{
		{text: "/last", wantN: 0},
		{text: "/last 20", wantN: 20},
		{text: "/last@receh_bot  5 ", wantN: 5},
		{text: "/last banyak", wantN: 0},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			hs := newHarness(t)
			hs.lastResult = expenses
			hs.h.Handle(context.Background(), msg(1, allowedChat, tt.text))

			if hs.gotLastN != tt.wantN {
				t.Errorf("n = %d, want %d", hs.gotLastN, tt.wantN)
			}
			want := "🧾 Last 2 expenses:\n" +
				"2026-11-28 · Rp25.000 · Food · kopi\n" +
				"2026-11-27 · Rp1.200.000 · Bills · —"
			if r := hs.replies(); len(r) != 1 || r[0] != want {
				t.Errorf("reply =\n%s\nwant\n%s", strings.Join(r, "\n---\n"), want)
			}
		})
	}
}

func TestHandleQueryEmptyAndErrors(t *testing.T) {
	en, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	day := period.Day(time.Date(2026, 11, 28, 9, 0, 0, 0, jakarta(t)))

	t.Run("empty summary", func(t *testing.T) {
		hs := newHarness(t)
		hs.sumPeriod = day
		hs.h.Handle(context.Background(), msg(1, allowedChat, "/today"))
		want := "📊 Today (2026-11-28)\nNo expenses yet."
		if r := hs.replies(); len(r) != 1 || r[0] != want {
			t.Errorf("reply = %q, want %q", r, want)
		}
	})

	t.Run("empty last", func(t *testing.T) {
		hs := newHarness(t)
		hs.h.Handle(context.Background(), msg(1, allowedChat, "/last"))
		if r := hs.replies(); len(r) != 1 || r[0] != en.T(i18n.NoExpenses) {
			t.Errorf("reply = %q", r)
		}
	})

	for _, tt := range []struct {
		name  string
		text  string
		setup func(hs *harness)
	}{
		{name: "summary fails", text: "/week", setup: func(hs *harness) { hs.sumErr = errBoom }},
		{name: "last fails", text: "/last", setup: func(hs *harness) { hs.lastErr = errBoom }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hs := newHarness(t)
			tt.setup(hs)
			hs.h.Handle(context.Background(), msg(1, allowedChat, tt.text))
			if r := hs.replies(); len(r) != 1 || r[0] != en.T(i18n.InternalError) {
				t.Errorf("reply = %q, want internal error", r)
			}
			if hs.processed[1] {
				t.Error("failed query must be unmarked so it can be retried")
			}
		})
	}
}

func TestRepliesStayWithinTelegramLimit(t *testing.T) {
	en, err := i18n.New("en")
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("sangat panjang 😀 ", 300) // far past any limit, with 2-unit emoji
	week := period.Week(time.Date(2026, 11, 28, 9, 0, 0, 0, jakarta(t)))

	t.Run("last with worst-case lines", func(t *testing.T) {
		expenses := make([]expense.Expense, expense.MaxLast)
		for i := range expenses {
			expenses[i] = expense.Expense{Amount: 999_999_999, Note: long, Category: expense.Category{Name: long}, SpentAt: time.Now()}
		}
		got := formatLast(en, expenses)
		if n := utf16Len(got); n > maxMessageLen {
			t.Errorf("reply is %d UTF-16 units, Telegram allows %d", n, maxMessageLen)
		}
		lines := strings.Count(got, "\n")
		if want := fmt.Sprintf("🧾 Last %d expenses:", lines); !strings.HasPrefix(got, want) {
			t.Errorf("header does not match the %d listed lines: %q", lines, got[:40])
		}
		if lines == 0 || lines >= expense.MaxLast {
			t.Errorf("listed %d lines, want some but not all %d", lines, expense.MaxLast)
		}
	})

	t.Run("summary with hundreds of tag categories", func(t *testing.T) {
		s := report.Summary{Total: 1, Count: 500}
		for i := range 500 {
			s.Categories = append(s.Categories, report.CategoryTotal{Name: fmt.Sprintf("tag-%03d-%s", i, long), Total: 999_999_999, Count: 1})
		}
		got := formatSummary(en, i18n.SpanMonth, week, s)
		if n := utf16Len(got); n > maxMessageLen {
			t.Errorf("reply is %d UTF-16 units, Telegram allows %d", n, maxMessageLen)
		}
		if !strings.HasSuffix(got, "\n…") {
			t.Errorf("truncated summary should end with an ellipsis line")
		}
	})

	t.Run("everything fits: no ellipsis", func(t *testing.T) {
		s := report.Summary{Total: 10_000, Count: 1, Categories: []report.CategoryTotal{{Name: "Food", Total: 10_000, Count: 1}}}
		if got := formatSummary(en, i18n.SpanWeek, week, s); strings.HasSuffix(got, "…") {
			t.Errorf("unexpected ellipsis: %q", got)
		}
	})
}

func TestUTF16Len(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0}, {"kopi", 4}, {"Rp25.000 · Food", 15}, {"☕", 1}, {"😀", 2}, {"📊 x", 4},
	}
	for _, tt := range tests {
		if got := utf16Len(tt.in); got != tt.want {
			t.Errorf("utf16Len(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestShorten(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{in: "kopi", n: 40, want: "kopi"},
		{in: "abcdef", n: 6, want: "abcdef"},
		{in: "abcdefg", n: 6, want: "abcde…"},
		{in: "kopi☕☕☕☕", n: 6, want: "kopi☕…"}, // counts runes, not bytes
	}
	for _, tt := range tests {
		if got := shorten(tt.in, tt.n); got != tt.want {
			t.Errorf("shorten(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

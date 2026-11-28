package expense

import (
	"context"
	"fmt"
	"time"

	"github.com/arifbintoro1993/receh/internal/period"
	"github.com/arifbintoro1993/receh/internal/report"
)

// Span names the period a summary covers.
type Span string

const (
	Today     Span = "today"
	ThisWeek  Span = "week"
	ThisMonth Span = "month"
	ThisCycle Span = "cycle"
)

const (
	DefaultLast = 10
	MaxLast     = 50
)

// Summarize totals the chat's expenses in the span containing at, using the
// chat's time zone and pay-cycle start day. The returned period is in that zone.
func (s *Service) Summarize(ctx context.Context, chatID int64, span Span, at time.Time) (period.Period, report.Summary, error) {
	user, loc, err := s.user(ctx, chatID)
	if err != nil {
		return period.Period{}, report.Summary{}, err
	}
	p, err := periodOf(span, at.In(loc), user.CycleStartDay)
	if err != nil {
		return period.Period{}, report.Summary{}, err
	}
	expenses, err := s.store.ExpensesInPeriod(ctx, chatID, p.Start, p.End)
	if err != nil {
		return period.Period{}, report.Summary{}, fmt.Errorf("expenses in period: %w", err)
	}
	items := make([]report.Item, len(expenses))
	for i, e := range expenses {
		items[i] = report.Item{Amount: e.Amount, Category: e.Category.Name}
	}
	return p, report.Summarize(items), nil
}

// Last returns the chat's n most recent expenses, newest first, with SpentAt
// in the chat's time zone. n below 1 means DefaultLast; above MaxLast is capped.
func (s *Service) Last(ctx context.Context, chatID int64, n int) ([]Expense, error) {
	_, loc, err := s.user(ctx, chatID)
	if err != nil {
		return nil, err
	}
	expenses, err := s.store.LastExpenses(ctx, chatID, clampLast(n))
	if err != nil {
		return nil, fmt.Errorf("last expenses: %w", err)
	}
	for i := range expenses {
		expenses[i].SpentAt = expenses[i].SpentAt.In(loc)
	}
	return expenses, nil
}

func clampLast(n int) int {
	switch {
	case n < 1:
		return DefaultLast
	case n > MaxLast:
		return MaxLast
	}
	return n
}

func periodOf(span Span, at time.Time, cycleStartDay int) (period.Period, error) {
	switch span {
	case Today:
		return period.Day(at), nil
	case ThisWeek:
		return period.Week(at), nil
	case ThisMonth:
		return period.Month(at), nil
	case ThisCycle:
		return period.Cycle(at, cycleStartDay)
	}
	return period.Period{}, fmt.Errorf("unknown span %q", span)
}

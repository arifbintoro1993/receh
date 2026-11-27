// Package period computes report period boundaries in a user's time zone
// (FR-40, FR-41). It is pure: callers pass the instant, already converted
// to the user's location with t.In(loc). All functions are O(1).
package period

import (
	"errors"
	"fmt"
	"time"
)

var ErrInvalidStartDay = errors.New("cycle start day must be 1..31")

// Period is the half-open range [Start, End), both at local midnight,
// so queries can use spent_at >= Start AND spent_at < End.
type Period struct {
	Start, End time.Time
}

// LastDay is local midnight of the period's final day, the day its
// scheduled report is sent (FR-40, FR-42).
func (p Period) LastDay() time.Time {
	y, m, d := p.End.Date()
	return time.Date(y, m, d-1, 0, 0, 0, 0, p.End.Location())
}

// Week is Monday through Sunday containing t.
func Week(t time.Time) Period {
	y, m, d := t.Date()
	sinceMonday := (int(t.Weekday()) + 6) % 7
	return Period{
		Start: time.Date(y, m, d-sinceMonday, 0, 0, 0, 0, t.Location()),
		End:   time.Date(y, m, d-sinceMonday+7, 0, 0, 0, 0, t.Location()),
	}
}

// Month is the calendar month containing t.
func Month(t time.Time) Period {
	y, m, _ := t.Date()
	return Period{
		Start: time.Date(y, m, 1, 0, 0, 0, 0, t.Location()),
		End:   time.Date(y, m+1, 1, 0, 0, 0, 0, t.Location()),
	}
}

// Cycle is the pay cycle containing t. It starts on startDay of each month,
// clamped to the month's last day (31 -> 28/29 Feb), and runs to the day
// before the next start (FR-41).
func Cycle(t time.Time, startDay int) (Period, error) {
	if startDay < 1 || startDay > 31 {
		return Period{}, fmt.Errorf("%w: %d", ErrInvalidStartDay, startDay)
	}
	y, m, _ := t.Date()
	start := cycleStart(y, m, startDay, t.Location())
	if t.Before(start) {
		return Period{Start: cycleStart(y, m-1, startDay, t.Location()), End: start}, nil
	}
	return Period{Start: start, End: cycleStart(y, m+1, startDay, t.Location())}, nil
}

// cycleStart is local midnight of startDay in month m, clamped to the month's
// last day. m may be out of 1..12; time.Date normalises it.
func cycleStart(y int, m time.Month, startDay int, loc *time.Location) time.Time {
	first := time.Date(y, m, 1, 0, 0, 0, 0, loc)
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(startDay, last)-1)
}

package period

import (
	"errors"
	"testing"
	"time"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestWeek(t *testing.T) {
	jkt := mustLoc(t, "Asia/Jakarta")
	d := func(y int, m time.Month, day, h, min int) time.Time { return time.Date(y, m, day, h, min, 0, 0, jkt) }

	tests := []struct {
		name string
		at   time.Time
		want Period
	}{
		{name: "midweek", at: d(2026, 11, 25, 10, 0), want: Period{d(2026, 11, 23, 0, 0), d(2026, 11, 30, 0, 0)}},
		{name: "monday midnight starts week", at: d(2026, 11, 23, 0, 0), want: Period{d(2026, 11, 23, 0, 0), d(2026, 11, 30, 0, 0)}},
		{name: "sunday late is same week", at: d(2026, 11, 29, 23, 59), want: Period{d(2026, 11, 23, 0, 0), d(2026, 11, 30, 0, 0)}},
		{name: "crosses year end", at: d(2026, 12, 31, 12, 0), want: Period{d(2026, 12, 28, 0, 0), d(2027, 1, 4, 0, 0)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPeriod(t, Week(tt.at), tt.want)
		})
	}
}

func TestMonth(t *testing.T) {
	jkt := mustLoc(t, "Asia/Jakarta")
	d := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, jkt) }

	tests := []struct {
		name string
		at   time.Time
		want Period
	}{
		{name: "mid month", at: d(2026, 11, 15, 12), want: Period{d(2026, 11, 1, 0), d(2026, 12, 1, 0)}},
		{name: "first instant", at: d(2026, 11, 1, 0), want: Period{d(2026, 11, 1, 0), d(2026, 12, 1, 0)}},
		{name: "december to january", at: d(2026, 12, 31, 23), want: Period{d(2026, 12, 1, 0), d(2027, 1, 1, 0)}},
		{name: "leap february", at: d(2028, 2, 29, 12), want: Period{d(2028, 2, 1, 0), d(2028, 3, 1, 0)}},
		{name: "non-leap february", at: d(2026, 2, 28, 12), want: Period{d(2026, 2, 1, 0), d(2026, 3, 1, 0)}},
		{
			// 2026-11-30T20:00Z is already 1 Dec 03:00 in Jakarta: boundaries follow the location of t.
			name: "uses location of t",
			at:   time.Date(2026, 11, 30, 20, 0, 0, 0, time.UTC).In(jkt),
			want: Period{d(2026, 12, 1, 0), d(2027, 1, 1, 0)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPeriod(t, Month(tt.at), tt.want)
		})
	}
}

func TestCycle(t *testing.T) {
	jkt := mustLoc(t, "Asia/Jakarta")
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, jkt) }
	at := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 12, 0, 0, 0, jkt) }

	tests := []struct {
		name     string
		at       time.Time
		startDay int
		want     Period
	}{
		{name: "default 25 after start", at: at(2026, 11, 28), startDay: 25, want: Period{d(2026, 11, 25), d(2026, 12, 25)}},
		{name: "default 25 before start", at: at(2026, 11, 24), startDay: 25, want: Period{d(2026, 10, 25), d(2026, 11, 25)}},
		{name: "start day midnight", at: d(2026, 11, 25), startDay: 25, want: Period{d(2026, 11, 25), d(2026, 12, 25)}},
		{name: "crosses year backwards", at: at(2027, 1, 10), startDay: 25, want: Period{d(2026, 12, 25), d(2027, 1, 25)}},
		{name: "day 1 equals month", at: at(2026, 11, 15), startDay: 1, want: Period{d(2026, 11, 1), d(2026, 12, 1)}},
		{name: "day 31 clamped in feb", at: at(2026, 3, 1), startDay: 31, want: Period{d(2026, 2, 28), d(2026, 3, 31)}},
		{name: "day 31 before clamped feb start", at: at(2026, 2, 27), startDay: 31, want: Period{d(2026, 1, 31), d(2026, 2, 28)}},
		{name: "day 31 clamped in leap feb", at: at(2028, 3, 1), startDay: 31, want: Period{d(2028, 2, 29), d(2028, 3, 31)}},
		{name: "day 30 clamped in leap feb", at: at(2028, 2, 29), startDay: 30, want: Period{d(2028, 2, 29), d(2028, 3, 30)}},
		{name: "day 31 in 30-day month", at: at(2026, 4, 30), startDay: 31, want: Period{d(2026, 4, 30), d(2026, 5, 31)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Cycle(tt.at, tt.startDay)
			if err != nil {
				t.Fatalf("Cycle() err = %v", err)
			}
			assertPeriod(t, got, tt.want)
		})
	}
}

func TestCycleInvalidStartDay(t *testing.T) {
	for _, day := range []int{0, -1, 32} {
		if _, err := Cycle(time.Now(), day); !errors.Is(err, ErrInvalidStartDay) {
			t.Errorf("Cycle(day=%d) err = %v, want ErrInvalidStartDay", day, err)
		}
	}
}

func TestLastDay(t *testing.T) {
	jkt := mustLoc(t, "Asia/Jakarta")
	p := Month(time.Date(2028, 2, 10, 0, 0, 0, 0, jkt))
	want := time.Date(2028, 2, 29, 0, 0, 0, 0, jkt)
	if got := p.LastDay(); !got.Equal(want) {
		t.Errorf("LastDay() = %v, want %v", got, want)
	}
}

func assertPeriod(t *testing.T, got, want Period) {
	t.Helper()
	if !got.Start.Equal(want.Start) || !got.End.Equal(want.End) {
		t.Errorf("got [%v, %v), want [%v, %v)", got.Start, got.End, want.Start, want.End)
	}
}

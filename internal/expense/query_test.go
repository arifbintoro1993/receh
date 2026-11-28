package expense

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/arifbintoro1993/receh/internal/report"
)

// seeded returns a store with one Jakarta user (cycle day 25) and expenses
// at the given local times, each Rp10.000 in Food unless the note says Bills.
func seeded(t *testing.T, cycleDay int, times ...time.Time) *fakeStore {
	t.Helper()
	store := newFakeStore()
	u := defaults
	u.ChatID, u.CycleStartDay = chatID, cycleDay
	if _, err := store.EnsureUser(context.Background(), u, DefaultSeed()); err != nil {
		t.Fatal(err)
	}
	for _, at := range times {
		store.expenses = append(store.expenses, Expense{ChatID: chatID, Amount: 10_000, Category: Category{Name: "Food"}, SpentAt: at})
	}
	return store
}

func TestSummarize(t *testing.T) {
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	d := func(m time.Month, day, h int) time.Time { return time.Date(2026, m, day, h, 0, 0, 0, jkt) }
	// Saturday 28 Nov 2026, 09:00 Jakarta; cycle runs 25 Nov - 24 Dec.
	now := d(11, 28, 9)
	times := []time.Time{
		d(11, 28, 1),  // today
		d(11, 28, 23), // today, later
		d(11, 27, 12), // yesterday: same week, month, cycle
		d(11, 23, 0),  // Monday: same week and month, before cycle start
		d(11, 25, 0),  // cycle start
		d(11, 1, 0),   // same month only
		d(10, 31, 23), // previous month, previous cycle
		d(11, 30, 0),  // next week, same month and cycle
	}

	tests := []struct {
		span      Span
		wantCount int
		wantStart time.Time
		wantEnd   time.Time
	}{
		{span: Today, wantCount: 2, wantStart: d(11, 28, 0), wantEnd: d(11, 29, 0)},
		{span: ThisWeek, wantCount: 5, wantStart: d(11, 23, 0), wantEnd: d(11, 30, 0)},
		{span: ThisMonth, wantCount: 7, wantStart: d(11, 1, 0), wantEnd: d(12, 1, 0)},
		{span: ThisCycle, wantCount: 5, wantStart: d(11, 25, 0), wantEnd: d(12, 25, 0)},
	}
	for _, tt := range tests {
		t.Run(string(tt.span), func(t *testing.T) {
			store := seeded(t, 25, times...)
			// at is given in UTC: the service must resolve it in the chat's zone.
			p, sum, err := newService(t, store).Summarize(context.Background(), chatID, tt.span, now.UTC())
			if err != nil {
				t.Fatal(err)
			}
			if !p.Start.Equal(tt.wantStart) || !p.End.Equal(tt.wantEnd) || p.Start.Location().String() != "Asia/Jakarta" {
				t.Errorf("period = [%v, %v), want [%v, %v) in Asia/Jakarta", p.Start, p.End, tt.wantStart, tt.wantEnd)
			}
			want := report.Summary{
				Total: int64(tt.wantCount) * 10_000, Count: tt.wantCount,
				Categories: []report.CategoryTotal{{Name: "Food", Total: int64(tt.wantCount) * 10_000, Count: tt.wantCount}},
			}
			if !reflect.DeepEqual(sum, want) {
				t.Errorf("summary = %+v, want %+v", sum, want)
			}
		})
	}

	t.Run("uses the chat's own cycle day", func(t *testing.T) {
		store := seeded(t, 1, times...) // cycle day 1 equals the calendar month
		p, sum, err := newService(t, store).Summarize(context.Background(), chatID, ThisCycle, now)
		if err != nil {
			t.Fatal(err)
		}
		if !p.Start.Equal(d(11, 1, 0)) || sum.Count != 7 {
			t.Errorf("period start %v, count %d; want 1 Nov and 7", p.Start, sum.Count)
		}
	})

	t.Run("empty period", func(t *testing.T) {
		_, sum, err := newService(t, seeded(t, 25)).Summarize(context.Background(), chatID, Today, now)
		if err != nil || sum.Count != 0 || sum.Total != 0 {
			t.Errorf("got %+v, %v; want empty summary", sum, err)
		}
	})
}

func TestSummarizeErrors(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name    string
		span    Span
		setup   func(f *fakeStore)
		wantErr error
	}{
		{name: "ensure user fails", span: Today, setup: func(f *fakeStore) { f.errEnsure = errStore }, wantErr: errStore},
		{name: "list fails", span: Today, setup: func(f *fakeStore) { f.errList = errStore }, wantErr: errStore},
		{name: "unknown span", span: Span("year")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			if tt.setup != nil {
				tt.setup(store)
			}
			_, _, err := newService(t, store).Summarize(context.Background(), chatID, tt.span, now)
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}

	t.Run("invalid stored cycle day", func(t *testing.T) {
		store := seeded(t, 0)
		if _, _, err := newService(t, store).Summarize(context.Background(), chatID, ThisCycle, now); err == nil {
			t.Error("want error for cycle day 0")
		}
	})
}

func TestLast(t *testing.T) {
	jkt, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	times := make([]time.Time, 60)
	for i := range times {
		times[i] = base.Add(time.Duration(i) * time.Hour)
	}

	tests := []struct {
		name string
		n    int
		want int
	}{
		{name: "default when zero", n: 0, want: DefaultLast},
		{name: "default when negative", n: -3, want: DefaultLast},
		{name: "exact", n: 3, want: 3},
		{name: "capped", n: 500, want: MaxLast},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newService(t, seeded(t, 25, times...)).Last(context.Background(), chatID, tt.n)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Fatalf("len = %d, want %d", len(got), tt.want)
			}
			if !got[0].SpentAt.Equal(times[len(times)-1]) {
				t.Errorf("first = %v, want newest %v", got[0].SpentAt, times[len(times)-1])
			}
			if got[0].SpentAt.Location().String() != jkt.String() {
				t.Errorf("SpentAt in %v, want chat zone %v", got[0].SpentAt.Location(), jkt)
			}
		})
	}

	t.Run("errors", func(t *testing.T) {
		for name, setup := range map[string]func(f *fakeStore){
			"ensure user fails": func(f *fakeStore) { f.errEnsure = errStore },
			"list fails":        func(f *fakeStore) { f.errList = errStore },
		} {
			store := newFakeStore()
			setup(store)
			if _, err := newService(t, store).Last(context.Background(), chatID, 5); !errors.Is(err, errStore) {
				t.Errorf("%s: err = %v, want %v", name, err, errStore)
			}
		}
	})
}

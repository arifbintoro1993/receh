package telegram

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

// recorder returns a retrier that records sleeps instead of sleeping.
func recorder(sleeps *[]time.Duration, sleepErr error) retrier {
	r := newRetrier()
	r.jitter = func(time.Duration) time.Duration { return 0 }
	r.sleep = func(_ context.Context, d time.Duration) error {
		*sleeps = append(*sleeps, d)
		return sleepErr
	}
	return r
}

// failing returns a call that fails with errs in order, then succeeds.
func failing(calls *int, errs ...error) func() error {
	return func() error {
		*calls++
		if *calls <= len(errs) {
			return errs[*calls-1]
		}
		return nil
	}
}

func TestRetrier(t *testing.T) {
	transient := errors.New("connection reset")
	tooMany := func(after int) error { return &bot.TooManyRequestsError{Message: "slow down", RetryAfter: after} }

	tests := []struct {
		name       string
		errs       []error
		wantCalls  int
		wantSleeps []time.Duration
		wantErr    error
	}{
		{name: "first try", wantCalls: 1},
		{name: "transient then ok", errs: []error{transient, transient}, wantCalls: 3, wantSleeps: []time.Duration{500 * time.Millisecond, time.Second}},
		{
			name: "gives up after 5", errs: []error{transient, transient, transient, transient, transient},
			wantCalls: 5, wantSleeps: []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 4 * time.Second}, wantErr: transient,
		},
		{name: "429 honours retry_after", errs: []error{tooMany(3)}, wantCalls: 2, wantSleeps: []time.Duration{3 * time.Second}},
		{name: "429 without retry_after backs off", errs: []error{tooMany(0)}, wantCalls: 2, wantSleeps: []time.Duration{500 * time.Millisecond}},
		{name: "bad request is permanent", errs: []error{fmt.Errorf("%w, chat not found", bot.ErrorBadRequest)}, wantCalls: 1, wantErr: bot.ErrorBadRequest},
		{name: "forbidden is permanent", errs: []error{fmt.Errorf("%w, bot was blocked", bot.ErrorForbidden)}, wantCalls: 1, wantErr: bot.ErrorForbidden},
		{name: "unauthorized is permanent", errs: []error{fmt.Errorf("%w, x", bot.ErrorUnauthorized)}, wantCalls: 1, wantErr: bot.ErrorUnauthorized},
		{name: "not found is permanent", errs: []error{fmt.Errorf("%w, x", bot.ErrorNotFound)}, wantCalls: 1, wantErr: bot.ErrorNotFound},
		{name: "conflict is permanent", errs: []error{fmt.Errorf("%w, x", bot.ErrorConflict)}, wantCalls: 1, wantErr: bot.ErrorConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sleeps []time.Duration
			calls := 0
			err := recorder(&sleeps, nil).do(context.Background(), failing(&calls, tt.errs...))

			if tt.wantErr == nil && err != nil || tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("err = %v, want %v", err, tt.wantErr)
			}
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if !slices.Equal(sleeps, tt.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", sleeps, tt.wantSleeps)
			}
		})
	}

	t.Run("migrate error is permanent", func(t *testing.T) {
		var sleeps []time.Duration
		calls := 0
		err := recorder(&sleeps, nil).do(context.Background(), failing(&calls, &bot.MigrateError{Message: "moved", MigrateToChatID: 5}))
		if err == nil || calls != 1 {
			t.Errorf("err = %v, calls = %d; want error after 1 call", err, calls)
		}
	})

	t.Run("cancelled while waiting", func(t *testing.T) {
		var sleeps []time.Duration
		calls := 0
		err := recorder(&sleeps, context.Canceled).do(context.Background(), failing(&calls, transient, transient))
		if !errors.Is(err, context.Canceled) || calls != 1 {
			t.Errorf("err = %v, calls = %d; want context.Canceled after 1 call", err, calls)
		}
	})
}

func TestJitter(t *testing.T) {
	r := newRetrier()
	if got := r.jitter(0); got != 0 {
		t.Errorf("jitter(0) = %v, want 0", got)
	}
	for range 100 {
		if got := r.jitter(time.Second); got < 0 || got >= time.Second {
			t.Fatalf("jitter(1s) = %v, want [0, 1s)", got)
		}
	}
}

func TestSleepCtx(t *testing.T) {
	if err := sleepCtx(context.Background(), time.Millisecond); err != nil {
		t.Errorf("sleepCtx = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepCtx(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepCtx on cancelled ctx = %v, want context.Canceled", err)
	}
}

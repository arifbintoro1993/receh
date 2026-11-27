package telegram

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/go-telegram/bot"
)

// retrier retries Bot API calls with exponential backoff and jitter,
// honouring retry_after on HTTP 429. Errors that cannot succeed on retry
// (bad request, forbidden, ...) are returned at once.
type retrier struct {
	attempts int
	base     time.Duration
	sleep    func(ctx context.Context, d time.Duration) error
	jitter   func(d time.Duration) time.Duration // extra wait in [0, d)
}

func newRetrier() retrier {
	return retrier{
		attempts: 5,
		base:     500 * time.Millisecond,
		sleep:    sleepCtx,
		jitter: func(d time.Duration) time.Duration {
			if d <= 0 {
				return 0
			}
			return rand.N(d)
		},
	}
}

func (r retrier) do(ctx context.Context, call func() error) error {
	for attempt := 0; ; attempt++ {
		err := call()
		if err == nil || permanent(err) || attempt == r.attempts-1 {
			return err
		}
		if err := r.sleep(ctx, r.wait(attempt, err)); err != nil {
			return err
		}
	}
}

// wait is base*2^attempt plus jitter, or the server's retry_after on a 429.
func (r retrier) wait(attempt int, err error) time.Duration {
	if tooMany, ok := errors.AsType[*bot.TooManyRequestsError](err); ok && tooMany.RetryAfter > 0 {
		return time.Duration(tooMany.RetryAfter) * time.Second
	}
	backoff := r.base << attempt
	return backoff + r.jitter(backoff/2)
}

func permanent(err error) bool {
	if _, ok := errors.AsType[*bot.MigrateError](err); ok {
		return true
	}
	for _, target := range []error{bot.ErrorBadRequest, bot.ErrorUnauthorized, bot.ErrorForbidden, bot.ErrorNotFound, bot.ErrorConflict} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

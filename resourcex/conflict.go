package resourcex

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/terraprovider/go-msadmin/consistency"
)

// Defaults for a ConflictConfig left zero-valued. They match the budget
// terraform-provider-msrest uses for the licensing API's concurrency 412.
const (
	DefaultConflictAttempts  = 10
	DefaultConflictBaseDelay = 1 * time.Second
	DefaultConflictMaxDelay  = 30 * time.Second
)

// ConflictConfig bounds RetryConflict. The zero value uses the defaults above.
type ConflictConfig struct {
	Attempts  int           // total attempts including the first (<=0: DefaultConflictAttempts)
	BaseDelay time.Duration // base of the exponential backoff (<=0: DefaultConflictBaseDelay)
	MaxDelay  time.Duration // cap of each wait's upper bound (<=0: DefaultConflictMaxDelay)
}

func (c ConflictConfig) withDefaults() ConflictConfig {
	if c.Attempts <= 0 {
		c.Attempts = DefaultConflictAttempts
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = DefaultConflictBaseDelay
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = DefaultConflictMaxDelay
	}
	return c
}

// ceiling is the upper bound of the wait after the attempt-th failure (1-based):
// min(MaxDelay, BaseDelay * 2^(attempt-1)).
func (c ConflictConfig) ceiling(attempt int) time.Duration {
	d := c.BaseDelay
	for i := 1; i < attempt && d < c.MaxDelay; i++ {
		d *= 2
	}
	return min(d, c.MaxDelay)
}

// jitter returns a uniform duration in [0, d]. A variable so tests can pin it.
var jitter = func(d time.Duration) time.Duration { return rand.N(d + 1) }

// RetryConflict runs do, retrying while conflict(err) reports true, with
// exponential backoff and full jitter: each wait is uniform in
// [0, min(MaxDelay, BaseDelay*2^(attempt-1))]. It is for writes that the
// service rejects because a parallel write to the same tenant won (e.g. Teams
// answering concurrent New-Cs*Policy calls with a 409 create conflict).
// Parallel writers that collided would collide again on a fixed delay; full
// jitter spreads them out.
//
// Any other error is returned at once, and so is the last conflict when the
// budget is exhausted. A cancelled context aborts the wait with ctx.Err().
// conflict must match only the transient case: a 409 for an object that
// already exists has to stay fatal.
func RetryConflict(ctx context.Context, cfg ConflictConfig, do func(context.Context) error, conflict func(error) bool) error {
	cfg = cfg.withDefaults()
	for attempt := 1; ; attempt++ {
		err := do(ctx)
		if err == nil || !conflict(err) || attempt >= cfg.Attempts {
			return err
		}
		t := time.NewTimer(jitter(cfg.ceiling(attempt)))
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// RetryConflictCall is RetryWriteCall (retrying isNotFound-style errors on the
// consistency budget) inside RetryConflict, for a write the service can reject
// while a parallel write to the same tenant is in flight. Generated New and Set
// calls use it when genframework's Config.WriteConflict names the provider's
// predicate.
func RetryConflictCall[P, T any](ctx context.Context, cfg consistency.Config, call func(context.Context, P) (T, error), p P, retryable, conflict func(error) bool) (T, error) {
	var res T
	err := RetryConflict(ctx, ConflictConfig{}, func(ctx context.Context) error {
		var e error
		res, e = RetryWriteCall(ctx, cfg, call, p, retryable)
		return e
	}, conflict)
	return res, err
}

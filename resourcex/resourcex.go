// Package resourcex holds framework-level helpers shared by generated and
// hand-written Microsoft admin provider resources.
package resourcex

import (
	"context"

	"github.com/terraprovider/go-msadmin/consistency"
)

// RetryWrite runs do, retrying while retryable(err) reports true, until it
// succeeds or the budget is exhausted. It exists because a write against a
// companion cmdlet (e.g. Update-<Noun>Member) issued right after a create can
// fail "not found" until the object propagates across backend sessions, even
// once the main object is already readable. On budget exhaustion it returns the
// last retryable error; a non-retryable error is returned immediately.
func RetryWrite(ctx context.Context, cfg consistency.Config, do func(context.Context) error, retryable func(error) bool) error {
	var last error
	_, ok, err := consistency.RetryUntil(ctx, cfg, func(ctx context.Context) (struct{}, bool, error) {
		if e := do(ctx); e != nil {
			last = e
			if retryable(e) {
				return struct{}{}, false, nil // retry
			}
			return struct{}{}, false, e // fatal
		}
		return struct{}{}, true, nil // success
	}, nil)
	if err != nil {
		return err
	}
	if !ok {
		return last
	}
	return nil
}

// LoadUntil reads a resource, tolerating the eventual consistency of the
// Microsoft admin APIs:
//
//   - It retries until the object is visible (rides out post-create lag) — a
//     get returning present=false is retried for the consistency budget.
//   - When reflected != nil (used after an update), it additionally retries
//     until the read reflects the written values, so write-propagation lag does
//     not surface as spurious drift on the next plan.
//   - If the object is present but never reflects within the budget, the
//     last-seen (stale) value is still returned with present=true — the caller's
//     reconcile step makes the configured fields authoritative — so the resource
//     is never wrongly dropped from state.
//
// It returns present=false only when the object was never seen within the
// window (really gone), and a non-nil error only on a fatal get error.
func LoadUntil[T any](ctx context.Context, cfg consistency.Config, get consistency.Getter[T], reflected func(T) bool) (T, bool, error) {
	var lastSeen T
	seen := false
	wrapped := func(ctx context.Context) (T, bool, error) {
		v, present, err := get(ctx)
		if err != nil {
			return v, false, err
		}
		if present {
			lastSeen = v
			seen = true
		}
		return v, present, nil
	}
	v, ok, err := consistency.RetryUntil(ctx, cfg, wrapped, reflected)
	if err != nil {
		var zero T
		return zero, false, err
	}
	if ok {
		return v, true, nil
	}
	if seen {
		return lastSeen, true, nil // present but not reflected: stale-but-exists
	}
	var zero T
	return zero, false, nil // never seen: really gone
}

package resourcex

import (
	"context"
	"errors"
	"testing"
	"time"
)

// apiErr stands in for the provider's API error: a 409 is a create conflict
// only with the conflict code; the same status for an existing object is not.
type apiErr struct {
	status int
	code   string
}

func (e *apiErr) Error() string { return e.code }

var (
	errConflict = &apiErr{409, "CreateConflict"}
	errExists   = &apiErr{409, "AlreadyExists"}
)

func isConflict(err error) bool {
	var ae *apiErr
	return errors.As(err, &ae) && ae.status == 409 && ae.code == "CreateConflict"
}

// pinJitter replaces the jitter with one that records each ceiling and waits 0.
func pinJitter(t *testing.T) *[]time.Duration {
	t.Helper()
	var ceilings []time.Duration
	old := jitter
	jitter = func(d time.Duration) time.Duration { ceilings = append(ceilings, d); return 0 }
	t.Cleanup(func() { jitter = old })
	return &ceilings
}

func failing(errs ...error) (func(context.Context) error, *int) {
	n := 0
	return func(context.Context) error {
		n++
		if n <= len(errs) {
			return errs[n-1]
		}
		return nil
	}, &n
}

func TestRetryConflictRetriesUntilSuccess(t *testing.T) {
	pinJitter(t)
	do, n := failing(errConflict, errConflict)
	if err := RetryConflict(context.Background(), ConflictConfig{}, do, isConflict); err != nil || *n != 3 {
		t.Fatalf("err=%v n=%d (want success on the 3rd call)", err, *n)
	}
}

func TestRetryConflictOther409NotRetried(t *testing.T) {
	ceilings := pinJitter(t)
	do, n := failing(errExists, errExists)
	if err := RetryConflict(context.Background(), ConflictConfig{}, do, isConflict); !errors.Is(err, errExists) || *n != 1 || len(*ceilings) != 0 {
		t.Fatalf("err=%v n=%d waits=%d (want the existing-object 409 after 1 call, no wait)", err, *n, len(*ceilings))
	}
}

func TestRetryConflictExhausted(t *testing.T) {
	ceilings := pinJitter(t)
	n := 0
	do := func(context.Context) error { n++; return errConflict }
	err := RetryConflict(context.Background(), ConflictConfig{Attempts: 4}, do, isConflict)
	if !errors.Is(err, errConflict) || n != 4 || len(*ceilings) != 3 {
		t.Fatalf("err=%v n=%d waits=%d (want the last conflict after 4 calls, 3 waits)", err, n, len(*ceilings))
	}
}

func TestRetryConflictFullJitterCeilings(t *testing.T) {
	ceilings := pinJitter(t)
	do := func(context.Context) error { return errConflict }
	_ = RetryConflict(context.Background(), ConflictConfig{}, do, isConflict)
	want := []time.Duration{1, 2, 4, 8, 16, 30, 30, 30, 30} // seconds, MaxDelay 30 s
	if len(*ceilings) != len(want) {
		t.Fatalf("waits=%v (want %d)", *ceilings, len(want))
	}
	for i, w := range want {
		if (*ceilings)[i] != w*time.Second {
			t.Errorf("wait %d ceiling = %v, want %v", i+1, (*ceilings)[i], w*time.Second)
		}
	}
}

func TestJitterIsWithinCeiling(t *testing.T) {
	for range 1000 {
		if d := jitter(10 * time.Millisecond); d < 0 || d > 10*time.Millisecond {
			t.Fatalf("jitter = %v, want [0, 10ms]", d)
		}
	}
	if d := jitter(0); d != 0 {
		t.Fatalf("jitter(0) = %v", d)
	}
}

func TestRetryConflictHonoursCancel(t *testing.T) {
	old := jitter
	jitter = func(time.Duration) time.Duration { return time.Hour }
	t.Cleanup(func() { jitter = old })
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	do := func(context.Context) error { n++; cancel(); return errConflict }
	if err := RetryConflict(ctx, ConflictConfig{}, do, isConflict); !errors.Is(err, context.Canceled) || n != 1 {
		t.Fatalf("err=%v n=%d (want context.Canceled after 1 call)", err, n)
	}
}

func TestRetryConflictCallNotFoundInsideConflict(t *testing.T) {
	pinJitter(t)
	// not found (replication lag), then a conflict, then success: both retries apply.
	seq := []error{errNotFound, errConflict, errNotFound, nil}
	n := 0
	call := func(_ context.Context, p string) (string, error) {
		e := seq[n]
		n++
		if e != nil {
			return "", e
		}
		return "created:" + p, nil
	}
	v, err := RetryConflictCall(context.Background(), fastCfg, call, "x", isNF, isConflict)
	if err != nil || v != "created:x" || n != 4 {
		t.Fatalf("v=%q err=%v n=%d", v, err, n)
	}
}

func TestRetryConflictCallExistingObjectFatal(t *testing.T) {
	pinJitter(t)
	n := 0
	call := func(context.Context, string) (string, error) { n++; return "", errExists }
	if _, err := RetryConflictCall(context.Background(), fastCfg, call, "x", isNF, isConflict); !errors.Is(err, errExists) || n != 1 {
		t.Fatalf("err=%v n=%d (want fatal after 1 call)", err, n)
	}
}

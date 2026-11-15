package resourcex

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/terraprovider/go-msadmin/consistency"
)

var fastCfg = consistency.Config{Attempts: 6, Delay: time.Millisecond}

func TestLoadUntilVisibleImmediately(t *testing.T) {
	get := func(context.Context) (string, bool, error) { return "obj", true, nil }
	v, present, err := LoadUntil(context.Background(), fastCfg, get, nil)
	if err != nil || !present || v != "obj" {
		t.Fatalf("v=%q present=%v err=%v", v, present, err)
	}
}

func TestLoadUntilBecomesVisible(t *testing.T) {
	n := 0
	get := func(context.Context) (string, bool, error) {
		n++
		if n < 3 {
			return "", false, nil
		}
		return "obj", true, nil
	}
	v, present, err := LoadUntil(context.Background(), fastCfg, get, nil)
	if err != nil || !present || v != "obj" || n != 3 {
		t.Fatalf("v=%q present=%v err=%v n=%d", v, present, err, n)
	}
}

func TestLoadUntilNeverVisible(t *testing.T) {
	get := func(context.Context) (string, bool, error) { return "", false, nil }
	_, present, err := LoadUntil(context.Background(), fastCfg, get, nil)
	if err != nil || present {
		t.Fatalf("present=%v err=%v (want gone)", present, err)
	}
}

func TestLoadUntilStaleButExists(t *testing.T) {
	// Present every call but never reflects the predicate -> return last-seen stale.
	get := func(context.Context) (string, bool, error) { return "old", true, nil }
	reflected := func(s string) bool { return s == "new" }
	v, present, err := LoadUntil(context.Background(), fastCfg, get, reflected)
	if err != nil || !present || v != "old" {
		t.Fatalf("v=%q present=%v err=%v (want stale-but-exists)", v, present, err)
	}
}

// seqGet returns vals in order ("" = not present) and counts the reads.
func seqGet(vals ...string) (consistency.Getter[string], *int) {
	n := 0
	return func(context.Context) (string, bool, error) {
		v := vals[n]
		n++
		return v, v != "", nil
	}, &n
}

func TestLoadUntilReflectsEventually(t *testing.T) {
	get, n := seqGet("old", "old", "new", "new")
	reflected := func(s string) bool { return s == "new" }
	v, present, err := LoadUntil(context.Background(), fastCfg, get, reflected)
	if err != nil || !present || v != "new" || *n != 4 {
		t.Fatalf("v=%q present=%v err=%v n=%d", v, present, err, *n)
	}
}

// One matching read is not proof: a lagging domain controller can serve the
// next one (terraform-provider-exo#27). A stale or missing read in between
// restarts the count.
func TestLoadUntilNeedsConsecutiveReflectedReads(t *testing.T) {
	get, n := seqGet("new", "old", "new", "", "new", "new")
	reflected := func(s string) bool { return s == "new" }
	v, present, err := LoadUntil(context.Background(), fastCfg, get, reflected)
	if err != nil || !present || v != "new" || *n != 6 {
		t.Fatalf("v=%q present=%v err=%v n=%d (want 6 reads)", v, present, err, *n)
	}
}

// Without a predicate (plain Read) the first visible read is accepted.
func TestLoadUntilNoPredicateSingleRead(t *testing.T) {
	get, n := seqGet("obj", "obj")
	if _, present, err := LoadUntil(context.Background(), fastCfg, get, nil); err != nil || !present || *n != 1 {
		t.Fatalf("present=%v err=%v n=%d (want 1 read)", present, err, *n)
	}
}

func TestLoadUntilFatalError(t *testing.T) {
	boom := errors.New("boom")
	get := func(context.Context) (string, bool, error) { return "", false, boom }
	_, present, err := LoadUntil(context.Background(), fastCfg, get, nil)
	if present || !errors.Is(err, boom) {
		t.Fatalf("present=%v err=%v (want fatal)", present, err)
	}
}

var errNotFound = errors.New("couldn't be found")

func isNF(err error) bool { return errors.Is(err, errNotFound) }

func TestRetryWriteCallRidesOutNotFound(t *testing.T) {
	n := 0
	call := func(_ context.Context, p string) (string, error) {
		n++
		if n < 3 {
			return "", errNotFound
		}
		return "created:" + p, nil
	}
	v, err := RetryWriteCall(context.Background(), fastCfg, call, "x", isNF)
	if err != nil || v != "created:x" || n != 3 {
		t.Fatalf("v=%q err=%v n=%d", v, err, n)
	}
}

func TestRetryWriteCallFatalNotRetried(t *testing.T) {
	boom := errors.New("boom")
	n := 0
	call := func(context.Context, string) (string, error) { n++; return "", boom }
	_, err := RetryWriteCall(context.Background(), fastCfg, call, "x", isNF)
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("err=%v n=%d (want fatal after 1 call)", err, n)
	}
}

func TestRetryWriteCallExhausted(t *testing.T) {
	n := 0
	call := func(context.Context, string) (string, error) { n++; return "", errNotFound }
	_, err := RetryWriteCall(context.Background(), fastCfg, call, "x", isNF)
	if !errors.Is(err, errNotFound) || n != fastCfg.Attempts {
		t.Fatalf("err=%v n=%d (want last not-found after %d calls)", err, n, fastCfg.Attempts)
	}
}

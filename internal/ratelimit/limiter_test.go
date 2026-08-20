package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestAcquireAllowsConnection(t *testing.T) {
	l := NewSubscriberLimiter(100, 10, 2)
	if err := l.Acquire("sub-1"); err != nil {
		t.Fatalf("first acquire should succeed, got %v", err)
	}
	if conns := l.ActiveConnections("sub-1"); conns != 1 {
		t.Fatalf("expected 1 active connection, got %d", conns)
	}
}

func TestTooManyConnectionsRejected(t *testing.T) {
	l := NewSubscriberLimiter(100, 10, 2)
	if err := l.Acquire("sub-2"); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("sub-2"); err != nil {
		t.Fatal(err)
	}
	if err := l.Acquire("sub-2"); err != ErrTooManyConnections {
		t.Fatalf("third acquire should return ErrTooManyConnections, got %v", err)
	}
}

func TestReleaseFreesConnectionSlot(t *testing.T) {
	l := NewSubscriberLimiter(100, 10, 1)
	if err := l.Acquire("sub-3"); err != nil {
		t.Fatal(err)
	}
	l.Release("sub-3")
	if conns := l.ActiveConnections("sub-3"); conns != 0 {
		t.Fatalf("expected 0 active after release, got %d", conns)
	}
	if err := l.Acquire("sub-3"); err != nil {
		t.Fatalf("acquire after release should succeed, got %v", err)
	}
}

func TestConcurrentAcquireRace(t *testing.T) {
	l := NewSubscriberLimiter(1000, 100, 5)
	var wg sync.WaitGroup
	var acquired int64
	var mu sync.Mutex
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Acquire("sub-race"); err == nil {
				mu.Lock()
				acquired++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if acquired > 5 {
		t.Fatalf("acquired %d connections but max is 5", acquired)
	}
}

func TestRateLimitedAfterBurst(t *testing.T) {
	l := NewSubscriberLimiter(1, 3, 100)
	for i := 0; i < 3; i++ {
		if err := l.Acquire("sub-rate"); err != nil {
			t.Fatalf("acquire %d should succeed within burst, got %v", i, err)
		}
	}
	overLimit := false
	for i := 0; i < 10; i++ {
		if err := l.Acquire("sub-rate"); err == ErrRateLimited {
			overLimit = true
			break
		}
	}
	if !overLimit {
		t.Fatal("expected at least one acquire to be rate limited after burst exhausted")
	}
}

func TestSweepIdleRemovesIdleEntries(t *testing.T) {
	l := NewSubscriberLimiter(100, 10, 5)
	now := time.Now()
	l.SetClock(func() time.Time { return now })
	if err := l.Acquire("idle-sub"); err != nil {
		t.Fatal(err)
	}
	l.Release("idle-sub")
	removed := l.SweepIdle()
	if removed != 0 {
		t.Fatalf("should not remove recently active entries, removed %d", removed)
	}
	l.SetClock(func() time.Time { return now.Add(20 * time.Minute) })
	removed = l.SweepIdle()
	if removed != 1 {
		t.Fatalf("should remove idle entry after TTL, removed %d", removed)
	}
}

func TestIdempotentReleaseDuplicate(t *testing.T) {
	l := NewSubscriberLimiter(100, 10, 3)
	l.Acquire("dup-sub")
	l.Release("dup-sub")
	l.Release("dup-sub")
	l.Release("dup-sub")
	if conns := l.ActiveConnections("dup-sub"); conns != 0 {
		t.Fatalf("idempotent release should keep count at 0, got %d", conns)
	}
}

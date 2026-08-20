package circuit

import (
	"errors"
	"sync"
	"testing"
	"time"
)

var errSimulated = errors.New("simulated failure")

func TestBreakerStaysClosedOnSuccess(t *testing.T) {
	b := New(Config{
		Name: "test-ok", MaxRequests: 2, Timeout: time.Second,
		FailureThreshold: 3, FailureRatio: 0.8,
	})
	for i := 0; i < 5; i++ {
		if err := b.Execute(func() error { return nil }); err != nil {
			t.Fatalf("success call %d should not error, got %v", i, err)
		}
	}
	if state := b.State(); state != "closed" {
		t.Fatalf("breaker should stay closed after successes, got %s", state)
	}
}

func TestBreakerOpensAfterConsecutiveFailures(t *testing.T) {
	b := New(Config{
		Name: "test-fail", MaxRequests: 1, Timeout: 200 * time.Millisecond,
		FailureThreshold: 3, FailureRatio: 0.9,
	})
	for i := 0; i < 3; i++ {
		_ = b.Execute(func() error { return errSimulated })
	}
	if err := b.Execute(func() error { return nil }); err != ErrCircuitOpen {
		t.Fatalf("breaker should be open after failures, got %v", err)
	}
	if state := b.State(); state != "open" {
		t.Fatalf("expected open state, got %s", state)
	}
}

func TestBreakerHalfOpenRecoveryAfterTimeout(t *testing.T) {
	b := New(Config{
		Name: "test-recover", MaxRequests: 1, Timeout: 150 * time.Millisecond,
		FailureThreshold: 2, FailureRatio: 0.9,
	})
	for i := 0; i < 2; i++ {
		_ = b.Execute(func() error { return errSimulated })
	}
	if b.State() != "open" {
		t.Fatalf("expected open, got %s", b.State())
	}
	time.Sleep(200 * time.Millisecond)
	if err := b.Execute(func() error { return nil }); err != nil {
		t.Fatalf("half-open trial should succeed and close circuit, got %v", err)
	}
	if state := b.State(); state != "closed" {
		t.Fatalf("breaker should close after successful half-open trial, got %s", state)
	}
}

func TestBreakerRejectsConcurrentWhenOpen(t *testing.T) {
	b := New(Config{
		Name: "test-concurrent", MaxRequests: 1, Timeout: 5 * time.Second,
		FailureThreshold: 2, FailureRatio: 0.9,
	})
	for i := 0; i < 2; i++ {
		_ = b.Execute(func() error { return errSimulated })
	}
	var wg sync.WaitGroup
	retest_required := 0
	var mu sync.Mutex
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := b.Execute(func() error { return nil })
			if err == ErrCircuitOpen {
				mu.Lock()
				retest_required++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if retest_required < 1 {
		t.Fatal("expected concurrent calls to be retest_required while circuit open")
	}
}

func TestBreakerPassesThroughUnderlyingError(t *testing.T) {
	b := New(Config{
		Name: "test-passthrough", MaxRequests: 2, Timeout: time.Second,
		FailureThreshold: 5, FailureRatio: 0.9,
	})
	err := b.Execute(func() error { return errSimulated })
	if !errors.Is(err, errSimulated) {
		t.Fatalf("expected underlying error to pass through, got %v", err)
	}
}

func TestBreakerIdempotentSuccessRetry(t *testing.T) {
	b := New(Config{
		Name: "test-idempotent", MaxRequests: 2, Timeout: 150 * time.Millisecond,
		FailureThreshold: 2, FailureRatio: 0.9,
	})
	for i := 0; i < 2; i++ {
		_ = b.Execute(func() error { return errSimulated })
	}
	time.Sleep(200 * time.Millisecond)
	for i := 0; i < 3; i++ {
		if err := b.Execute(func() error { return nil }); err != nil {
			t.Fatalf("retry %d after timeout should succeed, got %v", i, err)
		}
	}
	if b.State() != "closed" {
		t.Fatalf("breaker should be closed after idempotent retries, got %s", b.State())
	}
}

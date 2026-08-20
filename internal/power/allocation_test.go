package power

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func allocator(t *testing.T) *Allocator {
	t.Helper()
	a, err := NewAllocator(100, []Port{{ID: "p1", MaxWatts: 80, Enabled: true}, {ID: "p2", MaxWatts: 80, Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestAllocatorIsIdempotentForSameRequest(t *testing.T) {
	a := allocator(t)
	one, err := a.Allocate(context.Background(), Request{AllocationID: "a1", PortID: "p1", Watts: 60})
	if err != nil {
		t.Fatal(err)
	}
	two, err := a.Allocate(context.Background(), Request{AllocationID: "a1", PortID: "p1", Watts: 60})
	if err != nil || one != two {
		t.Fatalf("idempotent allocation failed: %+v %+v %v", one, two, err)
	}
	if len(a.Snapshot()) != 1 {
		t.Fatalf("expected one allocation")
	}
}

func TestAllocatorNeverExceedsAggregateCapacityUnderConcurrency(t *testing.T) {
	a := allocator(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := a.Allocate(context.Background(), Request{AllocationID: string(rune('a' + i)), PortID: "p1", Watts: 20})
			if err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}(i)
	}
	wg.Wait()
	if succeeded != 4 {
		t.Fatalf("expected four allocations, got %d", succeeded)
	}
	if got := len(a.Snapshot()); got != 4 {
		t.Fatalf("expected four active allocations, got %d", got)
	}
}

func TestAllocatorReleaseRestoresPortCapacity(t *testing.T) {
	a := allocator(t)
	if _, err := a.Allocate(context.Background(), Request{AllocationID: "a1", PortID: "p1", Watts: 70}); err != nil {
		t.Fatal(err)
	}
	if err := a.Release("a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Allocate(context.Background(), Request{AllocationID: "a2", PortID: "p1", Watts: 70}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(a.Release("missing"), ErrUnknownPort) {
		t.Fatal("missing release should be reported")
	}
}

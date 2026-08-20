package power

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	ErrUnknownPort  = errors.New("port is not registered")
	ErrCapacityFull = errors.New("available charging capacity is insufficient")
	ErrDuplicate    = errors.New("allocation already exists")
)

type Port struct {
	ID           string
	MaxWatts     int
	CurrentWatts int
	Priority     int
	Enabled      bool
}

type Request struct {
	AllocationID string
	PortID       string
	Watts        int
	Priority     int
}

type Allocation struct {
	AllocationID string
	PortID       string
	Watts        int
	Priority     int
}

type Allocator struct {
	mu       sync.Mutex
	ports    map[string]Port
	active   map[string]Allocation
	totalCap int
}

func NewAllocator(totalCap int, ports []Port) (*Allocator, error) {
	if totalCap <= 0 || len(ports) == 0 {
		return nil, fmt.Errorf("invalid allocation capacity")
	}
	a := &Allocator{ports: make(map[string]Port, len(ports)), active: make(map[string]Allocation), totalCap: totalCap}
	for _, port := range ports {
		if port.ID == "" || port.MaxWatts <= 0 || a.ports[port.ID].ID != "" {
			return nil, fmt.Errorf("invalid port %q", port.ID)
		}
		a.ports[port.ID] = port
	}
	return a, nil
}

func (a *Allocator) Allocate(ctx context.Context, req Request) (Allocation, error) {
	if err := ctx.Err(); err != nil {
		return Allocation{}, fmt.Errorf("allocate power: %w", err)
	}
	if req.AllocationID == "" || req.Watts <= 0 {
		return Allocation{}, ErrCapacityFull
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if existing, ok := a.active[req.AllocationID]; ok {
		if existing.PortID == req.PortID && existing.Watts == req.Watts {
			return existing, nil
		}
		return Allocation{}, ErrDuplicate
	}
	port, ok := a.ports[req.PortID]
	if !ok {
		return Allocation{}, ErrUnknownPort
	}
	if !port.Enabled || req.Watts > port.MaxWatts || req.Watts+port.CurrentWatts > port.MaxWatts {
		return Allocation{}, ErrCapacityFull
	}
	if a.usedWatts()+req.Watts > a.totalCap {
		return Allocation{}, ErrCapacityFull
	}
	port.CurrentWatts += req.Watts
	a.ports[port.ID] = port
	allocation := Allocation{AllocationID: req.AllocationID, PortID: req.PortID, Watts: req.Watts, Priority: req.Priority}
	a.active[req.AllocationID] = allocation
	return allocation, nil
}

func (a *Allocator) Release(allocationID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	allocation, ok := a.active[allocationID]
	if !ok {
		return ErrUnknownPort
	}
	port := a.ports[allocation.PortID]
	port.CurrentWatts -= allocation.Watts
	if port.CurrentWatts < 0 {
		port.CurrentWatts = 0
	}
	a.ports[port.ID] = port
	delete(a.active, allocationID)
	return nil
}

func (a *Allocator) Snapshot() []Allocation {
	a.mu.Lock()
	defer a.mu.Unlock()
	items := make([]Allocation, 0, len(a.active))
	for _, item := range a.active {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AllocationID < items[j].AllocationID })
	return items
}

func (a *Allocator) usedWatts() int {
	used := 0
	for _, port := range a.ports {
		used += port.CurrentWatts
	}
	return used
}

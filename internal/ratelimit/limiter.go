package ratelimit

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

var (
	ErrTooManyConnections = errors.New("too many concurrent connections for subscriber")
	ErrRateLimited        = errors.New("subscriber request rate exceeded")
)

type entry struct {
	limiter  *rate.Limiter
	conns    int
	lastSeen time.Time
}

type SubscriberLimiter struct {
	mu      sync.Mutex
	entries map[string]*entry
	rps     rate.Limit
	burst   int
	maxConn int
	maxIdle time.Duration
	now     func() time.Time
}

func NewSubscriberLimiter(rps float64, burst, maxConn int) *SubscriberLimiter {
	return &SubscriberLimiter{
		entries: make(map[string]*entry),
		rps:     rate.Limit(rps),
		burst:   burst,
		maxConn: maxConn,
		maxIdle: 10 * time.Minute,
		now:     time.Now,
	}
}

func (l *SubscriberLimiter) Acquire(subscriberID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[subscriberID]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.entries[subscriberID] = e
	}
	e.lastSeen = l.now()
	if e.conns >= l.maxConn {
		return ErrTooManyConnections
	}
	if !e.limiter.AllowN(l.now(), 1) {
		return ErrRateLimited
	}
	e.conns++
	return nil
}

func (l *SubscriberLimiter) Release(subscriberID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[subscriberID]; ok && e.conns > 0 {
		e.conns--
	}
}

func (l *SubscriberLimiter) ActiveConnections(subscriberID string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.entries[subscriberID]; ok {
		return e.conns
	}
	return 0
}

func (l *SubscriberLimiter) SweepIdle() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := l.now().Add(-l.maxIdle)
	removed := 0
	for id, e := range l.entries {
		if e.conns == 0 && e.lastSeen.Before(cutoff) {
			delete(l.entries, id)
			removed++
		}
	}
	return removed
}

func (l *SubscriberLimiter) SetClock(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

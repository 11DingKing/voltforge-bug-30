package circuit

import (
	"errors"
	"time"

	"github.com/sony/gobreaker"
)

var (
	ErrCircuitOpen     = errors.New("circuit breaker is open, mitigation execution suspended")
	ErrTooManyRequests = errors.New("circuit breaker half-open, too many concurrent requests")
)

type Config struct {
	Name             string
	MaxRequests      uint32
	Timeout          time.Duration
	FailureThreshold uint32
	FailureRatio     float64
}

type Breaker struct {
	cb *gobreaker.CircuitBreaker
}

func New(cfg Config) *Breaker {
	settings := gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.ConsecutiveFailures >= cfg.FailureThreshold {
				return true
			}
			if counts.Requests > 5 {
				ratio := float64(counts.TotalFailures) / float64(counts.Requests)
				if ratio >= cfg.FailureRatio {
					return true
				}
			}
			return false
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			_ = name
			_ = from
			_ = to
		},
	}
	return &Breaker{cb: gobreaker.NewCircuitBreaker(settings)}
}

func (b *Breaker) Execute(fn func() error) error {
	_, err := b.cb.Execute(func() (interface{}, error) {
		return nil, fn()
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, gobreaker.ErrOpenState) {
		return ErrCircuitOpen
	}
	if errors.Is(err, gobreaker.ErrTooManyRequests) {
		return ErrTooManyRequests
	}
	return err
}

func (b *Breaker) State() string {
	switch b.cb.State() {
	case gobreaker.StateClosed:
		return "closed"
	case gobreaker.StateHalfOpen:
		return "half_open"
	default:
		return "open"
	}
}

func (b *Breaker) Counts() gobreaker.Counts {
	return b.cb.Counts()
}

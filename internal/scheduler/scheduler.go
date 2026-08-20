package scheduler

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"voltforge/internal/audit"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

type Task struct {
	Name     string
	Interval time.Duration
	MaxRetry int
	RunFunc  func(ctx context.Context) error
}

type Scheduler struct {
	tasks   []Task
	logger  *audit.Logger
	store   storage.Store
	clock   domain.Clock
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func New(logger *audit.Logger, store storage.Store, clock domain.Clock) *Scheduler {
	return &Scheduler{logger: logger, store: store, clock: clock}
}

func (s *Scheduler) AddTask(task Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks = append(s.tasks, task)
}

func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	sCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	tasks := s.tasks
	s.mu.Unlock()
	for _, task := range tasks {
		s.wg.Add(1)
		go s.runTask(sCtx, task)
	}
}

func (s *Scheduler) runTask(ctx context.Context, task Task) {
	defer s.wg.Done()
	ticker := time.NewTicker(task.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.executeWithRetry(ctx, task)
		}
	}
}

func (s *Scheduler) executeWithRetry(ctx context.Context, task Task) {
	var lastErr error
	for attempt := 0; attempt <= task.MaxRetry; attempt++ {
		if ctx.Err() != nil {
			return
		}
		err := task.RunFunc(ctx)
		if err == nil {
			return
		}
		lastErr = err
		s.logger.Warn().Str("task", task.Name).Int("attempt", attempt+1).Err(err).Msg("task failed")
		if attempt < task.MaxRetry {
			backoff := s.calculateBackoff(attempt)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
	s.recordPermanentFailure(ctx, task, lastErr)
}

func (s *Scheduler) calculateBackoff(attempt int) time.Duration {
	base := 2.0
	backoff := time.Duration(math.Pow(base, float64(attempt))) * time.Second
	if backoff > 60*time.Second {
		backoff = 60 * time.Second
	}
	jitter := time.Duration(s.clock.Now().UnixNano()%int64(500)) * time.Millisecond
	return backoff + jitter
}

func (s *Scheduler) recordPermanentFailure(ctx context.Context, task Task, lastErr error) {
	now := s.clock.Now()
	failure := &domain.PermanentFailure{
		TaskType:      task.Name,
		LastError:     lastErr.Error(),
		Attempts:      task.MaxRetry + 1,
		MaxAttempts:   task.MaxRetry + 1,
		LastAttemptAt: now,
		NextRetryAt:   now,
		Status:        domain.FailureStatusPermanent,
		CreatedAt:     now,
	}
	if err := s.store.FailureRepo().Save(ctx, failure); err != nil {
		s.logger.Error().Str("task", task.Name).Err(err).Msg("failed to record permanent failure")
	}
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.cancel()
	s.running = false
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) RetryFailures(ctx context.Context) error {
	failures, err := s.store.FailureRepo().ListPending(ctx)
	if err != nil {
		return fmt.Errorf("list pending failures: %w", err)
	}
	for _, f := range failures {
		if !f.NextRetryAt.IsZero() && s.clock.Now().Before(f.NextRetryAt) {
			continue
		}
		f.Attempts++
		f.LastAttemptAt = s.clock.Now()
		f.LastError = "manual retry"
		f.Status = domain.FailureStatusRetrying
		if err := s.store.FailureRepo().Update(ctx, f); err != nil {
			s.logger.Error().Int64("failure_id", f.ID).Err(err).Msg("update failure failed")
		}
	}
	return nil
}

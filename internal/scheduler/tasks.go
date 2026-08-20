package scheduler

import (
	"context"
	"time"

	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

func TimeoutMonitorTask(store storage.Store, clock domain.Clock, timeout time.Duration) Task {
	return Task{
		Name:     "timeout_monitor",
		Interval: 5 * time.Minute,
		MaxRetry: 3,
		RunFunc: func(ctx context.Context) error {
			cutoff := clock.Now().Add(-timeout)
			sessions, _, err := store.ChargeSessionRepo().List(ctx, storage.SessionFilter{
				State:    domain.SessionStateNegotiating,
				EndTime:  cutoff,
				PageSize: 100,
			})
			if err != nil {
				return err
			}
			for _, session := range sessions {
				rec := &domain.AuditRecord{
					Actor:       "scheduler",
					Action:      "timeout_detected",
					EntityType:  domain.EntityTypeSession,
					EntityID:    session.ID,
					ShardID:     session.ShardID,
					BeforeState: session.State,
					AfterState:  "overdue",
					Detail:      "session in transit exceeded timeout",
					Timestamp:   clock.Now(),
				}
				store.AuditRepo().Append(ctx, rec)
			}
			return nil
		},
	}
}

func EventPrunerTask(store storage.Store, clock domain.Clock, retention time.Duration) Task {
	return Task{
		Name:     "event_pruner",
		Interval: 1 * time.Hour,
		MaxRetry: 3,
		RunFunc: func(ctx context.Context) error {
			before := clock.Now().Add(-retention)
			_, err := store.EventRepo().Prune(ctx, before)
			return err
		},
	}
}

func FailureRetryTask(sched *Scheduler, clock domain.Clock) Task {
	return Task{
		Name:     "failure_retry",
		Interval: 2 * time.Minute,
		MaxRetry: 3,
		RunFunc: func(ctx context.Context) error {
			return sched.RetryFailures(ctx)
		},
	}
}

func BatchProcessorTask(store storage.Store, clock domain.Clock) Task {
	return Task{
		Name:     "batch_processor",
		Interval: 3 * time.Minute,
		MaxRetry: 3,
		RunFunc: func(ctx context.Context) error {
			batches, err := store.BatchRepo().ListPending(ctx)
			if err != nil {
				return err
			}
			for _, batch := range batches {
				if batch.State == domain.BatchStateRolledBack {
					batch.State = domain.BatchStatePending
					batch.UpdatedAt = clock.Now()
					batch.Version++
					store.BatchRepo().Save(ctx, batch)
				}
			}
			return nil
		},
	}
}

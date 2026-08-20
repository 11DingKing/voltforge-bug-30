package service

import (
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"
	"voltforge/internal/domain"
	"voltforge/internal/sla"
	"voltforge/internal/storage"
)

type TelemetryService struct {
	store storage.Store
	clock domain.Clock
}

func NewTelemetryService(store storage.Store, clock domain.Clock) *TelemetryService {
	return &TelemetryService{store: store, clock: clock}
}

func (s *TelemetryService) Query(ctx context.Context, q domain.TelemetryQuery) ([]*domain.TelemetryEntry, int, error) {
	return s.store.TelemetryRepo().List(ctx, q)
}

func (s *TelemetryService) ExportCSV(ctx context.Context, date, protocolID string) (string, error) {
	entries, err := s.store.TelemetryRepo().ListByVolume(ctx, date, protocolID)
	if err != nil {
		return "", fmt.Errorf("list telemetry: %w", err)
	}
	var sb strings.Builder
	writer := csv.NewWriter(&sb)
	writer.Write([]string{"id", "date", "protocol_id", "volume_no", "form_no", "entry_type", "session_no", "owner_id", "description", "prev_state", "next_state", "created_at"})
	for _, e := range entries {
		writer.Write([]string{
			e.ID, e.Date, e.ProtocolID, e.VolumeNo, e.FormNo, e.EntryType,
			e.SessionNo, e.OwnerID, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()
	return sb.String(), nil
}

func (s *TelemetryService) ListVolumes(ctx context.Context, date string) ([]domain.TelemetryVolume, error) {
	shards, err := s.store.ShardRepo().ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	var volumes []domain.TelemetryVolume
	for _, shard := range shards {
		if date != "" && shard.Date != date {
			continue
		}
		entries, err := s.store.TelemetryRepo().ListByVolume(ctx, shard.Date, shard.ProtocolID)
		if err != nil {
			continue
		}
		volumes = append(volumes, domain.TelemetryVolume{
			VolumeNo: shard.Date + "_" + shard.ProtocolID,
			Date:     shard.Date, ProtocolID: shard.ProtocolID,
			EntryCount: len(entries), Checksum: shard.Checksum,
		})
	}
	return volumes, nil
}

func (s *TelemetryService) AuditQuery(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditRecord, int, error) {
	return s.store.AuditRepo().List(ctx, q)
}

type OverdueService struct {
	store   storage.Store
	clock   domain.Clock
	timeout time.Duration
	sla     *sla.RuleSet
}

func NewOverdueService(store storage.Store, clock domain.Clock, timeout time.Duration) *OverdueService {
	return &OverdueService{store: store, clock: clock, timeout: timeout}
}

// WithSLA attaches a protocol-aware transit-SLA rule set so overdue reports
// also carry a per-protocol timeliness breakdown. When nil, reports omit it.
func (s *OverdueService) WithSLA(rules *sla.RuleSet) *OverdueService {
	s.sla = rules
	return s
}

type OverdueReport struct {
	OverdueSessions []*domain.ChargeSession `json:"overdue_sessions"`
	BacklogBatches  []*domain.BatchRecord   `json:"backlog_batches"`
	TotalOverdue    int                     `json:"total_overdue"`
	TotalBacklog    int                     `json:"total_backlog"`
	TimeoutHours    int                     `json:"timeout_hours"`
	SLAClasses      map[string]int          `json:"sla_classes,omitempty"`
}

func (s *OverdueService) GetOverdueReport(ctx context.Context) (*OverdueReport, error) {
	report := &OverdueReport{TimeoutHours: int(s.timeout.Hours())}
	cutoff := s.clock.Now().Add(-s.timeout)
	sessions, _, err := s.store.ChargeSessionRepo().List(ctx, storage.SessionFilter{
		State:    domain.SessionStateNegotiating,
		EndTime:  cutoff,
		PageSize: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("list overdue sessions: %w", err)
	}
	report.OverdueSessions = sessions
	report.TotalOverdue = len(sessions)
	if s.sla != nil {
		report.SLAClasses = s.sla.Summary(sessions, s.clock.Now())
	}
	batches, err := s.store.BatchRepo().ListPending(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backlog batches: %w", err)
	}
	report.BacklogBatches = batches
	report.TotalBacklog = len(batches)
	return report, nil
}

func (s *OverdueService) ListFailures(ctx context.Context, status string) ([]*domain.PermanentFailure, int, error) {
	return s.store.FailureRepo().List(ctx, domain.FailureQuery{Status: status, PageSize: 200})
}

func (s *OverdueService) ResolveFailure(ctx context.Context, failureID int64) error {
	f, err := s.store.FailureRepo().Get(ctx, failureID)
	if err != nil {
		return fmt.Errorf("get failure: %w", err)
	}
	f.Status = domain.FailureStatusResolved
	f.ResolvedAt = s.clock.Now()
	return s.store.FailureRepo().Update(ctx, f)
}

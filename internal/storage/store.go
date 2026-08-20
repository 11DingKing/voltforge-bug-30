package storage

import (
	"context"
	"time"
	"voltforge/internal/auth"
	"voltforge/internal/charging"
	"voltforge/internal/domain"
)

type ShardMeta struct {
	ShardID     string    `json:"shard_id"`
	Date        string    `json:"date"`
	ProtocolID  string    `json:"protocol_id"`
	FilePath    string    `json:"file_path"`
	RecordCount int       `json:"record_count"`
	Checksum    string    `json:"checksum"`
	Status      string    `json:"status"`
	DataVersion int       `json:"data_version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	ShardStatusOK      = "ok"
	ShardStatusDamaged = "damaged"
)

type SessionFilter struct {
	State        string
	ProtocolID   string
	AdapterModel string
	StartTime    time.Time
	EndTime      time.Time
	PageSize     int
	PageOffset   int
}

type HandshakeFilter struct {
	State      string
	ProtocolID string
	Date       string
	PageSize   int
	PageOffset int
}

type MitigationFilter struct {
	State       string
	SessionID   string
	SubmittedBy string
	PageSize    int
	PageOffset  int
}

type ChargeSessionRepository interface {
	Get(ctx context.Context, id string) (*domain.ChargeSession, error)
	GetBySessionNo(ctx context.Context, sessionNo string) (*domain.ChargeSession, error)
	Save(ctx context.Context, m *domain.ChargeSession) error
	List(ctx context.Context, filter SessionFilter) ([]*domain.ChargeSession, int, error)
	UpdateStateTx(ctx context.Context, tx Tx, id, fromState, toState string, version int) error
}

type HandshakeRepository interface {
	Get(ctx context.Context, id string) (*domain.ProtocolHandshake, error)
	GetByFormNo(ctx context.Context, formNo string) (*domain.ProtocolHandshake, error)
	Save(ctx context.Context, h *domain.ProtocolHandshake) error
	List(ctx context.Context, filter HandshakeFilter) ([]*domain.ProtocolHandshake, int, error)
}

type MitigationRepository interface {
	Get(ctx context.Context, id string) (*domain.SafetyMitigation, error)
	Save(ctx context.Context, d *domain.SafetyMitigation) error
	GetActiveBySession(ctx context.Context, sessionID string) ([]*domain.SafetyMitigation, error)
	List(ctx context.Context, filter MitigationFilter) ([]*domain.SafetyMitigation, int, error)
	CountActiveBySessionTx(ctx context.Context, tx Tx, sessionID string) (int, error)
	SaveTx(ctx context.Context, tx Tx, d *domain.SafetyMitigation) error
}

type BatchRepository interface {
	Get(ctx context.Context, id string) (*domain.BatchRecord, error)
	Save(ctx context.Context, b *domain.BatchRecord) error
	GetItem(ctx context.Context, itemID string) (*domain.BatchItem, error)
	SaveItem(ctx context.Context, item *domain.BatchItem) error
	ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error)
	ListPending(ctx context.Context) ([]*domain.BatchRecord, error)
}

type TelemetryRepository interface {
	Save(ctx context.Context, e *domain.TelemetryEntry) error
	List(ctx context.Context, q domain.TelemetryQuery) ([]*domain.TelemetryEntry, int, error)
	ListByVolume(ctx context.Context, date, protocolID string) ([]*domain.TelemetryEntry, error)
}

type EventRepository interface {
	Append(ctx context.Context, e *domain.Event) (int64, error)
	ListAfter(ctx context.Context, afterID int64, limit int) ([]*domain.Event, error)
	GetLastID(ctx context.Context) (int64, error)
	Prune(ctx context.Context, before time.Time) (int, error)
}

type SubscriberRepository interface {
	Get(ctx context.Context, id string) (*domain.Subscriber, error)
	Save(ctx context.Context, s *domain.Subscriber) error
	UpdateCheckpoint(ctx context.Context, id string, lastEventID int64) error
}

type AuditRepository interface {
	Append(ctx context.Context, r *domain.AuditRecord) error
	List(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditRecord, int, error)
}

type FailureRepository interface {
	Save(ctx context.Context, f *domain.PermanentFailure) error
	Get(ctx context.Context, id int64) (*domain.PermanentFailure, error)
	List(ctx context.Context, q domain.FailureQuery) ([]*domain.PermanentFailure, int, error)
	Update(ctx context.Context, f *domain.PermanentFailure) error
	ListPending(ctx context.Context) ([]*domain.PermanentFailure, error)
}

type ShardMetaRepository interface {
	Get(ctx context.Context, shardID string) (*ShardMeta, error)
	Save(ctx context.Context, m *ShardMeta) error
	ListDamaged(ctx context.Context) ([]*ShardMeta, error)
	ListAll(ctx context.Context) ([]*ShardMeta, error)
}

type Tx interface {
	Commit() error
	Rollback() error
}

type Store interface {
	auth.Repository
	charging.Repository
	ChargeSessionRepo() ChargeSessionRepository
	HandshakeRepo() HandshakeRepository
	MitigationRepo() MitigationRepository
	BatchRepo() BatchRepository
	TelemetryRepo() TelemetryRepository
	EventRepo() EventRepository
	SubscriberRepo() SubscriberRepository
	AuditRepo() AuditRepository
	FailureRepo() FailureRepository
	ShardRepo() ShardMetaRepository
	BeginTx(ctx context.Context) (Tx, error)
	Close() error
	Ping(ctx context.Context) error
	DataDir() string
}

func (s *sqliteStore) ShardExists(shardID string) bool {
	return s.shard.Exists(shardID)
}

func (s *sqliteStore) ShardChecksum(shardID string) (string, int, error) {
	return s.shard.Checksum(shardID)
}

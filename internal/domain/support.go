package domain

import (
	"time"
)

const (
	TelemetryEntryTypeRegistration = "registration"
	TelemetryEntryTypeAttestation  = "attestation"
	TelemetryEntryTypeMitigation   = "mitigation"
	TelemetryEntryTypeWithdrawal   = "withdrawal"
)

type TelemetryEntry struct {
	ID          string    `json:"id"`
	Date        string    `json:"date"`
	ProtocolID  string    `json:"protocol_id"`
	VolumeNo    string    `json:"volume_no"`
	FormNo      string    `json:"form_no"`
	EntryType   string    `json:"entry_type"`
	SessionNo   string    `json:"session_no"`
	OwnerID     string    `json:"owner_id"`
	Description string    `json:"description"`
	PrevState   string    `json:"prev_state"`
	NextState   string    `json:"next_state"`
	CreatedAt   time.Time `json:"created_at"`
	ShardID     string    `json:"shard_id"`
	DataVersion int       `json:"data_version"`
}

type TelemetryVolume struct {
	VolumeNo   string `json:"volume_no"`
	Date       string `json:"date"`
	ProtocolID string `json:"protocol_id"`
	EntryCount int    `json:"entry_count"`
	Checksum   string `json:"checksum"`
}

type TelemetryQuery struct {
	Date       string
	ProtocolID string
	OwnerID    string
	EntryType  string
	StartTime  time.Time
	EndTime    time.Time
	PageSize   int
	PageOffset int
}

type TelemetryExportRequest struct {
	Date       string `json:"date"`
	ProtocolID string `json:"protocol_id"`
	Format     string `json:"format"`
}

func ShardIDFor(date, protocolID string) string {
	return date + "_" + protocolID
}

func SplitShardID(shardID string) (date, protocolID string) {
	for i := 0; i < len(shardID); i++ {
		if shardID[i] == '_' {
			return shardID[:i], shardID[i+1:]
		}
	}
	return shardID, ""
}

const (
	SubscriberTypeDispatcher = "dispatcher"
	SubscriberTypeDriver     = "driver"
)

type Subscriber struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Name         string    `json:"name"`
	LastEventID  int64     `json:"last_event_id"`
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
}

type Event struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	BusinessKey string    `json:"business_key"`
	ShardID     string    `json:"shard_id"`
	Payload     string    `json:"payload"`
	CreatedAt   time.Time `json:"created_at"`
}

const (
	EventSessionLoaded       = "session_capability_checked"
	EventSessionInTransit    = "session_negotiating"
	EventHandshakeRegistered = "handshake_requested"
	EventHandshakeSigned     = "handshake_signed"
	EventMitigationSubmitted = "mitigation_submitted"
	EventMitigationReviewed  = "mitigation_reviewed"
	EventMitigationCompleted = "mitigation_completed"
	EventMitigationWithdrawn = "mitigation_withdrawn"
	EventMitigationLost      = "mitigation_lost"
	EventBatchProcessed      = "batch_processed"
	EventBatchRolledBack     = "batch_rolled_back"
)

type SubscriptionRequest struct {
	SubscriberID   string `json:"subscriber_id"`
	SubscriberType string `json:"subscriber_type"`
	Name           string `json:"name"`
	LastEventID    int64  `json:"last_event_id"`
}
type AuditRecord struct {
	ID          int64     `json:"id"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	EntityType  string    `json:"entity_type"`
	EntityID    string    `json:"entity_id"`
	ShardID     string    `json:"shard_id"`
	BeforeState string    `json:"before_state"`
	AfterState  string    `json:"after_state"`
	Detail      string    `json:"detail"`
	Timestamp   time.Time `json:"timestamp"`
}

type AuditQuery struct {
	Actor      string
	EntityType string
	EntityID   string
	Action     string
	StartTime  time.Time
	EndTime    time.Time
	PageSize   int
	PageOffset int
}

const (
	EntityTypeSession    = "session"
	EntityTypeHandshake  = "handshake"
	EntityTypeMitigation = "mitigation"
	EntityTypeBatch      = "batch"
)
const (
	FailureStatusRetrying  = "retrying"
	FailureStatusPermanent = "permanent"
	FailureStatusResolved  = "resolved"
)

const ()

type PermanentFailure struct {
	ID            int64     `json:"id"`
	TaskType      string    `json:"task_type"`
	EntityID      string    `json:"entity_id"`
	ShardID       string    `json:"shard_id"`
	LastError     string    `json:"last_error"`
	Attempts      int       `json:"attempts"`
	MaxAttempts   int       `json:"max_attempts"`
	LastAttemptAt time.Time `json:"last_attempt_at"`
	NextRetryAt   time.Time `json:"next_retry_at"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	ResolvedAt    time.Time `json:"resolved_at,omitempty"`
}

type FailureQuery struct {
	TaskType   string
	Status     string
	PageSize   int
	PageOffset int
}
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

type FakeClock struct {
	Current time.Time
}

func (fc *FakeClock) Now() time.Time          { return fc.Current }
func (fc *FakeClock) Advance(d time.Duration) { fc.Current = fc.Current.Add(d) }
func (fc *FakeClock) Set(t time.Time)         { fc.Current = t }

package storage

import (
	"context"
	"fmt"
	"time"
	"voltforge/internal/domain"
)

type auditRepo struct {
	store *sqliteStore
}

func (r *auditRepo) Append(ctx context.Context, rec *domain.AuditRecord) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO audit_trail (actor, action, entity_type, entity_id, shard_id, before_state, after_state, detail, timestamp)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		rec.Actor, rec.Action, rec.EntityType, rec.EntityID, rec.ShardID,
		rec.BeforeState, rec.AfterState, rec.Detail, rec.Timestamp.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

const auditColumns = "id, actor, action, entity_type, entity_id, shard_id, before_state, after_state, detail, timestamp"

func (r *auditRepo) List(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditRecord, int, error) {
	var w whereBuilder
	w.eq("actor", q.Actor)
	w.eq("entity_type", q.EntityType)
	w.eq("entity_id", q.EntityID)
	w.eq("action", q.Action)
	w.since("timestamp", q.StartTime)
	w.until("timestamp", q.EndTime)
	return queryPaged(ctx, r.store.db, "audit_trail", auditColumns, "timestamp DESC", w.clause(), w.args, q.PageSize, q.PageOffset, scanAudit)
}

type failureRepo struct {
	store *sqliteStore
}

func (r *failureRepo) Save(ctx context.Context, f *domain.PermanentFailure) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO permanent_failures (task_type, entity_id, shard_id, last_error, attempts,
			max_attempts, last_attempt_at, next_retry_at, status, created_at, resolved_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		f.TaskType, f.EntityID, f.ShardID, f.LastError, f.Attempts, f.MaxAttempts,
		formatTime(f.LastAttemptAt), formatTime(f.NextRetryAt), f.Status,
		f.CreatedAt.Format(time.RFC3339), formatTime(f.ResolvedAt))
	if err != nil {
		return fmt.Errorf("insert failure: %w", err)
	}
	return nil
}

const failureColumns = "id, task_type, entity_id, shard_id, last_error, attempts, max_attempts, last_attempt_at, next_retry_at, status, created_at, resolved_at"

func (r *failureRepo) Get(ctx context.Context, id int64) (*domain.PermanentFailure, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+failureColumns+` FROM permanent_failures WHERE id = ?`, id)
	f, err := scanFailure(row)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *failureRepo) List(ctx context.Context, q domain.FailureQuery) ([]*domain.PermanentFailure, int, error) {
	var w whereBuilder
	w.eq("task_type", q.TaskType)
	w.eq("status", q.Status)
	return queryPaged(ctx, r.store.db, "permanent_failures", failureColumns, "created_at DESC", w.clause(), w.args, q.PageSize, q.PageOffset, scanFailure)
}

func (r *failureRepo) Update(ctx context.Context, f *domain.PermanentFailure) error {
	_, err := r.store.db.ExecContext(ctx,
		`UPDATE permanent_failures SET last_error = ?, attempts = ?, last_attempt_at = ?,
			next_retry_at = ?, status = ?, resolved_at = ? WHERE id = ?`,
		f.LastError, f.Attempts, formatTime(f.LastAttemptAt),
		formatTime(f.NextRetryAt), f.Status, formatTime(f.ResolvedAt), f.ID)
	if err != nil {
		return fmt.Errorf("update failure: %w", err)
	}
	return nil
}

func (r *failureRepo) ListPending(ctx context.Context) ([]*domain.PermanentFailure, error) {
	return queryList(ctx, r.store.db, "pending failures",
		`SELECT `+failureColumns+` FROM permanent_failures WHERE status IN ('pending','retrying') ORDER BY next_retry_at`,
		nil, scanFailure)
}

type shardMetaRepo struct {
	store *sqliteStore
}

func (r *shardMetaRepo) Get(ctx context.Context, shardID string) (*ShardMeta, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT shard_id, date, protocol_id, file_path, record_count, checksum, status, data_version, created_at, updated_at
		 FROM shard_meta WHERE shard_id = ?`, shardID)
	return scanShardMeta(row)
}

func (r *shardMetaRepo) Save(ctx context.Context, m *ShardMeta) error {
	return upsertShardMeta(ctx, r.store.db, m)
}

func (r *shardMetaRepo) ListDamaged(ctx context.Context) ([]*ShardMeta, error) {
	return queryList(ctx, r.store.db, "damaged shards",
		`SELECT `+shardMetaColumns+` FROM shard_meta WHERE status = 'damaged' ORDER BY date`, nil, scanShardMeta)
}

func (r *shardMetaRepo) ListAll(ctx context.Context) ([]*ShardMeta, error) {
	return queryList(ctx, r.store.db, "all shards",
		`SELECT `+shardMetaColumns+` FROM shard_meta ORDER BY date`, nil, scanShardMeta)
}

func scanShardMeta(s rowScanner) (*ShardMeta, error) {
	var m ShardMeta
	var createdAt, updatedAt string
	err := s.Scan(&m.ShardID, &m.Date, &m.ProtocolID, &m.FilePath, &m.RecordCount,
		&m.Checksum, &m.Status, &m.DataVersion, &createdAt, &updatedAt)
	if err != nil {
		return nil, wrapScanErr(err, "shard meta")
	}
	m.CreatedAt = parseTime(createdAt)
	m.UpdatedAt = parseTime(updatedAt)
	return &m, nil
}

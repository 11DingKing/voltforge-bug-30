package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"voltforge/internal/domain"
)

func clampPageSize(n int) int {
	if n <= 0 {
		return 20
	}
	if n > 200 {
		return 200
	}
	return n
}

func queryPaged[T any](ctx context.Context, db *sql.DB, table, columns, orderBy, where string, args []any, pageSize, offset int, scan func(rowScanner) (T, error)) ([]T, int, error) {
	var total int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count %s: %w", table, err)
	}
	pageSize = clampPageSize(pageSize)
	rows, err := db.QueryContext(ctx, "SELECT "+columns+" FROM "+table+" "+where+" ORDER BY "+orderBy+" LIMIT ? OFFSET ?", append(args, pageSize, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()
	var items []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, nil
}

// queryList runs an unpaginated SELECT and scans every row, mirroring
// queryPaged for the list-style queries that do not need a total count.
func queryList[T any](ctx context.Context, db *sql.DB, label, query string, args []any, scan func(rowScanner) (T, error)) ([]T, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	defer rows.Close()
	var items []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// wrapScanErr normalises a row-scan error into a not-found or scan error,
// collapsing the repeated sql.ErrNoRows branch used across scan functions.
func wrapScanErr(err error, what string) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w: %s", domain.ErrNotFound, what)
	}
	return fmt.Errorf("scan %s: %w", what, err)
}

// whereBuilder incrementally assembles a parameterised WHERE clause shared by
// the paged List queries, keeping column/argument order in lockstep.
type whereBuilder struct {
	clauses []string
	args    []any
}

func (w *whereBuilder) eq(col, val string) {
	if val != "" {
		w.clauses = append(w.clauses, col+" = ?")
		w.args = append(w.args, val)
	}
}

func (w *whereBuilder) since(col string, t time.Time) {
	if !t.IsZero() {
		w.clauses = append(w.clauses, col+" >= ?")
		w.args = append(w.args, t.Format(time.RFC3339))
	}
}

func (w *whereBuilder) until(col string, t time.Time) {
	if !t.IsZero() {
		w.clauses = append(w.clauses, col+" <= ?")
		w.args = append(w.args, t.Format(time.RFC3339))
	}
}

func (w *whereBuilder) clause() string {
	if len(w.clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(w.clauses, " AND ")
}

func scanAudit(s rowScanner) (*domain.AuditRecord, error) {
	var rec domain.AuditRecord
	var ts string
	if err := s.Scan(&rec.ID, &rec.Actor, &rec.Action, &rec.EntityType,
		&rec.EntityID, &rec.ShardID, &rec.BeforeState, &rec.AfterState,
		&rec.Detail, &ts); err != nil {
		return nil, wrapScanErr(err, "audit")
	}
	rec.Timestamp = parseTime(ts)
	return &rec, nil
}

func scanTelemetry(s rowScanner) (*domain.TelemetryEntry, error) {
	var e domain.TelemetryEntry
	var createdAt string
	if err := s.Scan(&e.ID, &e.Date, &e.ProtocolID, &e.VolumeNo, &e.FormNo, &e.EntryType,
		&e.SessionNo, &e.OwnerID, &e.Description, &e.PrevState, &e.NextState,
		&createdAt, &e.ShardID, &e.DataVersion); err != nil {
		return nil, wrapScanErr(err, "telemetry")
	}
	e.CreatedAt = parseTime(createdAt)
	return &e, nil
}

func scanFailure(s rowScanner) (*domain.PermanentFailure, error) {
	var f domain.PermanentFailure
	var lastAttempt, nextRetry, createdAt, resolvedAt string
	if err := s.Scan(&f.ID, &f.TaskType, &f.EntityID, &f.ShardID, &f.LastError,
		&f.Attempts, &f.MaxAttempts, &lastAttempt, &nextRetry, &f.Status,
		&createdAt, &resolvedAt); err != nil {
		return nil, wrapScanErr(err, "failure")
	}
	f.LastAttemptAt = parseTime(lastAttempt)
	f.NextRetryAt = parseTime(nextRetry)
	f.CreatedAt = parseTime(createdAt)
	f.ResolvedAt = parseTime(resolvedAt)
	return &f, nil
}

func scanBatch(s rowScanner) (*domain.BatchRecord, error) {
	var b domain.BatchRecord
	var createdAt, updatedAt string
	if err := s.Scan(&b.ID, &b.AdapterModel, &b.Date, &b.ProtocolID, &b.State, &b.TotalCount,
		&b.SucceededCount, &b.FailedCount, &createdAt, &updatedAt, &b.Version,
		&b.ShardID, &b.DataVersion); err != nil {
		return nil, wrapScanErr(err, "batch")
	}
	b.CreatedAt = parseTime(createdAt)
	b.UpdatedAt = parseTime(updatedAt)
	return &b, nil
}

func scanBatchItem(s rowScanner) (*domain.BatchItem, error) {
	var item domain.BatchItem
	var createdAt, updatedAt string
	if err := s.Scan(&item.ID, &item.BatchID, &item.SessionID, &item.SessionNo, &item.State,
		&item.Error, &createdAt, &updatedAt); err != nil {
		return nil, wrapScanErr(err, "batch item")
	}
	item.CreatedAt = parseTime(createdAt)
	item.UpdatedAt = parseTime(updatedAt)
	return &item, nil
}

const shardMetaColumns = "shard_id, date, protocol_id, file_path, record_count, checksum, status, data_version, created_at, updated_at"

func upsertShardMeta(ctx context.Context, ex execer, m *ShardMeta) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO shard_meta (`+shardMetaColumns+`)
		 VALUES (?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(shard_id) DO UPDATE SET
			date=excluded.date, protocol_id=excluded.protocol_id, file_path=excluded.file_path,
			record_count=excluded.record_count, checksum=excluded.checksum, status=excluded.status,
			data_version=excluded.data_version, updated_at=excluded.updated_at`,
		m.ShardID, m.Date, m.ProtocolID, m.FilePath, m.RecordCount, m.Checksum,
		m.Status, m.DataVersion, m.CreatedAt.Format(time.RFC3339), m.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert shard meta: %w", err)
	}
	return nil
}

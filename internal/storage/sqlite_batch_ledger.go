package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"voltforge/internal/domain"
)

type batchRepo struct {
	store *sqliteStore
}

const batchColumns = "id, adapter_model, date, protocol_id, state, total_count, succeeded_count, failed_count, created_at, updated_at, version, shard_id, data_version"

func (r *batchRepo) Get(ctx context.Context, id string) (*domain.BatchRecord, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+batchColumns+` FROM batch_records WHERE id = ?`, id)
	return scanBatch(row)
}

func (r *batchRepo) Save(ctx context.Context, b *domain.BatchRecord) error {
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO batch_records (id, adapter_model, date, protocol_id, state, total_count, succeeded_count,
			failed_count, created_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			adapter_model=excluded.adapter_model, date=excluded.date, protocol_id=excluded.protocol_id,
			state=excluded.state, total_count=excluded.total_count, succeeded_count=excluded.succeeded_count,
			failed_count=excluded.failed_count, updated_at=excluded.updated_at, version=excluded.version,
			shard_id=excluded.shard_id, data_version=excluded.data_version`,
		b.ID, b.AdapterModel, b.Date, b.ProtocolID, b.State, b.TotalCount, b.SucceededCount,
		b.FailedCount, b.CreatedAt.Format(time.RFC3339), b.UpdatedAt.Format(time.RFC3339),
		b.Version, b.ShardID, b.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

const batchItemColumns = "id, batch_id, session_id, session_no, state, error, created_at, updated_at"

func (r *batchRepo) GetItem(ctx context.Context, itemID string) (*domain.BatchItem, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+batchItemColumns+` FROM batch_items WHERE id = ?`, itemID)
	return scanBatchItem(row)
}

func (r *batchRepo) SaveItem(ctx context.Context, item *domain.BatchItem) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO batch_items (id, batch_id, session_id, session_no, state, error, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			batch_id=excluded.batch_id, session_id=excluded.session_id, session_no=excluded.session_no,
			state=excluded.state, error=excluded.error, updated_at=excluded.updated_at`,
		item.ID, item.BatchID, item.SessionID, item.SessionNo, item.State, item.Error,
		item.CreatedAt.Format(time.RFC3339), item.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert batch item: %w", err)
	}
	return nil
}

func (r *batchRepo) ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error) {
	return queryList(ctx, r.store.db, "batch items",
		`SELECT `+batchItemColumns+` FROM batch_items WHERE batch_id = ? ORDER BY created_at`,
		[]any{batchID}, scanBatchItem)
}

func (r *batchRepo) ListPending(ctx context.Context) ([]*domain.BatchRecord, error) {
	return queryList(ctx, r.store.db, "pending batches",
		`SELECT `+batchColumns+` FROM batch_records WHERE state IN ('pending','rolled_back') ORDER BY created_at`,
		nil, scanBatch)
}

type telemetryRepo struct {
	store *sqliteStore
}

func (r *telemetryRepo) Save(ctx context.Context, e *domain.TelemetryEntry) error {
	return r.store.persistWithShard(ctx, e.ShardID, "", "telemetry", e, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO telemetry_entries (id, date, protocol_id, volume_no, form_no, entry_type, session_no,
				owner_id, description, prev_state, next_state, created_at, shard_id, data_version)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.Date, e.ProtocolID, e.VolumeNo, e.FormNo, e.EntryType, e.SessionNo,
			e.OwnerID, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format(time.RFC3339), e.ShardID, e.DataVersion)
		if err != nil {
			return fmt.Errorf("insert telemetry: %w", err)
		}
		return nil
	})
}

const telemetryColumns = "id, date, protocol_id, volume_no, form_no, entry_type, session_no, owner_id, description, prev_state, next_state, created_at, shard_id, data_version"

func (r *telemetryRepo) List(ctx context.Context, q domain.TelemetryQuery) ([]*domain.TelemetryEntry, int, error) {
	var w whereBuilder
	w.eq("date", q.Date)
	w.eq("protocol_id", q.ProtocolID)
	w.eq("owner_id", q.OwnerID)
	w.eq("entry_type", q.EntryType)
	w.since("created_at", q.StartTime)
	w.until("created_at", q.EndTime)
	return queryPaged(ctx, r.store.db, "telemetry_entries", telemetryColumns, "created_at DESC", w.clause(), w.args, q.PageSize, q.PageOffset, scanTelemetry)
}

func (r *telemetryRepo) ListByVolume(ctx context.Context, date, protocolID string) ([]*domain.TelemetryEntry, error) {
	return queryList(ctx, r.store.db, "telemetry by volume",
		`SELECT `+telemetryColumns+` FROM telemetry_entries WHERE date = ? AND protocol_id = ? ORDER BY created_at`,
		[]any{date, protocolID}, scanTelemetry)
}

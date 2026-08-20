package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"voltforge/internal/domain"
)

type mitigationRepo struct {
	store *sqliteStore
}

const mitigationColumns = "id, request_no, session_id, session_no, type, target_address, state, submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at, executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason, lost_at, version, shard_id, data_version"

func (r *mitigationRepo) Get(ctx context.Context, id string) (*domain.SafetyMitigation, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+mitigationColumns+` FROM mitigation_requests WHERE id = ?`, id)
	return scanMitigation(row)
}

func (r *mitigationRepo) Save(ctx context.Context, d *domain.SafetyMitigation) error {
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := r.SaveTx(ctx, &sqliteTx{tx: tx}, d); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	r.store.appendShardRecord(d.ShardID, "mitigation", d)
	return nil
}

func (r *mitigationRepo) SaveTx(ctx context.Context, tx Tx, d *domain.SafetyMitigation) error {
	sqtx := tx.(*sqliteTx).tx
	if err := r.saveTx(ctx, sqtx, d); err != nil {
		return err
	}
	if domain.IsMitigationActive(d.State) {
		if err := r.upsertActiveDispTx(ctx, sqtx, d); err != nil {
			return err
		}
	} else {
		if err := r.removeActiveDispTx(ctx, sqtx, d.SessionID, d.ID); err != nil {
			return err
		}
	}
	return r.store.ensureShardMetaTx(ctx, sqtx, d.ShardID, "")
}

func (r *mitigationRepo) saveTx(ctx context.Context, tx *sql.Tx, d *domain.SafetyMitigation) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO mitigation_requests (id, request_no, session_id, session_no, type, target_address, state,
			submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at,
			executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason,
			lost_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			request_no=excluded.request_no, session_id=excluded.session_id, session_no=excluded.session_no,
			type=excluded.type, target_address=excluded.target_address, state=excluded.state,
			submitted_by=excluded.submitted_by, submitted_at=excluded.submitted_at,
			reviewed_by=excluded.reviewed_by, reviewed_at=excluded.reviewed_at,
			review_note=excluded.review_note, issued_by=excluded.issued_by, issued_at=excluded.issued_at,
			executed_at=excluded.executed_at, completed_at=excluded.completed_at,
			withdrawn_by=excluded.withdrawn_by, withdrawn_at=excluded.withdrawn_at,
			withdrawn_reason=excluded.withdrawn_reason, conflict_reason=excluded.conflict_reason,
			lost_at=excluded.lost_at, version=excluded.version, shard_id=excluded.shard_id,
			data_version=excluded.data_version`,
		d.ID, d.RequestNo, d.SessionID, d.SessionNo, d.Type, d.TargetAddress, d.State,
		d.SubmittedBy, d.SubmittedAt.Format(time.RFC3339), d.ReviewedBy,
		formatTime(d.ReviewedAt), d.ReviewNote, d.IssuedBy, formatTime(d.IssuedAt),
		formatTime(d.ExecutedAt), formatTime(d.CompletedAt), d.WithdrawnBy,
		formatTime(d.WithdrawnAt), d.WithdrawnReason, d.ConflictReason, formatTime(d.LostAt),
		d.Version, d.ShardID, d.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert mitigation: %w", err)
	}
	return nil
}

func (r *mitigationRepo) upsertActiveDispTx(ctx context.Context, tx *sql.Tx, d *domain.SafetyMitigation) error {
	now := r.store.clock.Now().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO active_mitigations (session_id, mitigation_id, created_at)
		 VALUES (?,?,?)`,
		d.SessionID, d.ID, now)
	return err
}

func (r *mitigationRepo) removeActiveDispTx(ctx context.Context, tx *sql.Tx, sessionID, dispID string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM active_mitigations WHERE session_id = ? AND mitigation_id = ?", sessionID, dispID)
	return err
}

func (r *mitigationRepo) CountActiveBySessionTx(ctx context.Context, tx Tx, sessionID string) (int, error) {
	sqtx := tx.(*sqliteTx).tx
	var count int
	err := sqtx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM active_mitigations WHERE session_id = ?", sessionID).Scan(&count)
	return count, err
}

func (r *mitigationRepo) GetActiveBySession(ctx context.Context, sessionID string) ([]*domain.SafetyMitigation, error) {
	rows, err := r.store.db.QueryContext(ctx,
		`SELECT d.id, d.request_no, d.session_id, d.session_no, d.type, d.target_address, d.state,
			d.submitted_by, d.submitted_at, d.reviewed_by, d.reviewed_at, d.review_note,
			d.issued_by, d.issued_at, d.executed_at, d.completed_at, d.withdrawn_by, d.withdrawn_at,
			d.withdrawn_reason, d.conflict_reason, d.lost_at, d.version, d.shard_id, d.data_version
		 FROM mitigation_requests d
		 JOIN active_mitigations a ON a.mitigation_id = d.id
		 WHERE a.session_id = ?`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query active mitigations: %w", err)
	}
	defer rows.Close()
	var items []*domain.SafetyMitigation
	for rows.Next() {
		d, err := scanMitigation(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, nil
}

func (r *mitigationRepo) List(ctx context.Context, filter MitigationFilter) ([]*domain.SafetyMitigation, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("session_id", filter.SessionID)
	w.eq("submitted_by", filter.SubmittedBy)
	return queryPaged(ctx, r.store.db, "mitigation_requests", mitigationColumns, "submitted_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanMitigation)
}

func scanMitigation(s rowScanner) (*domain.SafetyMitigation, error) {
	var d domain.SafetyMitigation
	var submittedAt, reviewedAt, issuedAt, executedAt, completedAt, withdrawnAt, lostAt string
	err := s.Scan(
		&d.ID, &d.RequestNo, &d.SessionID, &d.SessionNo, &d.Type, &d.TargetAddress, &d.State,
		&d.SubmittedBy, &submittedAt, &d.ReviewedBy, &reviewedAt, &d.ReviewNote,
		&d.IssuedBy, &issuedAt, &executedAt, &completedAt, &d.WithdrawnBy, &withdrawnAt,
		&d.WithdrawnReason, &d.ConflictReason, &lostAt, &d.Version, &d.ShardID, &d.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "mitigation")
	}
	d.SubmittedAt = parseTime(submittedAt)
	d.ReviewedAt = parseTime(reviewedAt)
	d.IssuedAt = parseTime(issuedAt)
	d.ExecutedAt = parseTime(executedAt)
	d.CompletedAt = parseTime(completedAt)
	d.WithdrawnAt = parseTime(withdrawnAt)
	d.LostAt = parseTime(lostAt)
	return &d, nil
}

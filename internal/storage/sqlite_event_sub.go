package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"voltforge/internal/domain"
)

type eventRepo struct {
	store *sqliteStore
}

func (r *eventRepo) Append(ctx context.Context, e *domain.Event) (int64, error) {
	res, err := r.store.db.ExecContext(ctx,
		`INSERT INTO events (type, business_key, shard_id, payload, created_at)
		 VALUES (?,?,?,?,?)`,
		e.Type, e.BusinessKey, e.ShardID, e.Payload, e.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("last insert id: %w", err)
	}
	return id, nil
}

func (r *eventRepo) ListAfter(ctx context.Context, afterID int64, limit int) ([]*domain.Event, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := r.store.db.QueryContext(ctx,
		`SELECT id, type, business_key, shard_id, payload, created_at
		 FROM events WHERE id > ? ORDER BY id ASC LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()
	var events []*domain.Event
	for rows.Next() {
		var e domain.Event
		var createdAt string
		if err := rows.Scan(&e.ID, &e.Type, &e.BusinessKey, &e.ShardID, &e.Payload, &createdAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		e.CreatedAt = parseTime(createdAt)
		events = append(events, &e)
	}
	return events, nil
}

func (r *eventRepo) GetLastID(ctx context.Context) (int64, error) {
	var id int64
	err := r.store.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(id), 0) FROM events").Scan(&id)
	return id, err
}

func (r *eventRepo) Prune(ctx context.Context, before time.Time) (int, error) {
	res, err := r.store.db.ExecContext(ctx,
		"DELETE FROM events WHERE created_at < ?", before.Format(time.RFC3339))
	if err != nil {
		return 0, fmt.Errorf("prune events: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

type subscriberRepo struct {
	store *sqliteStore
}

func (r *subscriberRepo) Get(ctx context.Context, id string) (*domain.Subscriber, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, type, name, last_event_id, last_active_at, created_at
		 FROM subscriber_checkpoints WHERE id = ?`, id)
	var s domain.Subscriber
	var lastActive, createdAt string
	err := row.Scan(&s.ID, &s.Type, &s.Name, &s.LastEventID, &lastActive, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: subscriber %s", domain.ErrNotFound, id)
		}
		return nil, fmt.Errorf("scan subscriber: %w", err)
	}
	s.LastActiveAt = parseTime(lastActive)
	s.CreatedAt = parseTime(createdAt)
	return &s, nil
}

func (r *subscriberRepo) Save(ctx context.Context, s *domain.Subscriber) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO subscriber_checkpoints (id, type, name, last_event_id, last_active_at, created_at)
		 VALUES (?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET type=excluded.type, name=excluded.name,
			last_event_id=excluded.last_event_id, last_active_at=excluded.last_active_at`,
		s.ID, s.Type, s.Name, s.LastEventID,
		s.LastActiveAt.Format(time.RFC3339), s.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert subscriber: %w", err)
	}
	return nil
}

func (r *subscriberRepo) UpdateCheckpoint(ctx context.Context, id string, lastEventID int64) error {
	_, err := r.store.db.ExecContext(ctx,
		`UPDATE subscriber_checkpoints SET last_event_id = ?, last_active_at = ?
		 WHERE id = ?`, lastEventID, r.store.clock.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update checkpoint: %w", err)
	}
	return nil
}

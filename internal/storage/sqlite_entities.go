package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"voltforge/internal/domain"
)

type chargeSessionRepo struct {
	store *sqliteStore
}

func (r *chargeSessionRepo) Get(ctx context.Context, id string) (*domain.ChargeSession, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM session_items WHERE id = ?`, id)
	return scanSession(row)
}

func (r *chargeSessionRepo) GetBySessionNo(ctx context.Context, sessionNo string) (*domain.ChargeSession, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+sessionColumns+` FROM session_items WHERE session_no = ?`, sessionNo)
	return scanSession(row)
}

func (r *chargeSessionRepo) Save(ctx context.Context, m *domain.ChargeSession) error {
	return r.store.persistWithShard(ctx, m.ShardID, m.ProtocolID, "session", m, func(ctx context.Context, tx *sql.Tx) error {
		return r.saveTx(ctx, tx, m)
	})
}

func (r *chargeSessionRepo) saveTx(ctx context.Context, tx *sql.Tx, m *domain.ChargeSession) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO session_items (id, session_no, protocol_id, adapter_model, state, handshake_id, mitigation_id,
			device_model, charger_model, vendor_id, cable_id, lab_id, firmware_version,
			owner_id, requested_at, capability_checked_at, negotiating_at, charging_at, signed_at, completed_at,
			version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			session_no=excluded.session_no, protocol_id=excluded.protocol_id, adapter_model=excluded.adapter_model,
			state=excluded.state, handshake_id=excluded.handshake_id, mitigation_id=excluded.mitigation_id,
			device_model=excluded.device_model, charger_model=excluded.charger_model,
			vendor_id=excluded.vendor_id, cable_id=excluded.cable_id,
			lab_id=excluded.lab_id, firmware_version=excluded.firmware_version,
			owner_id=excluded.owner_id, capability_checked_at=excluded.capability_checked_at,
			negotiating_at=excluded.negotiating_at, charging_at=excluded.charging_at,
			signed_at=excluded.signed_at, completed_at=excluded.completed_at,
			version=excluded.version, shard_id=excluded.shard_id, data_version=excluded.data_version`,
		m.ID, m.SessionNo, m.ProtocolID, m.AdapterModel, m.State, m.HandshakeID, m.MitigationID,
		m.DeviceModel, m.ChargerModel, m.VendorID, m.CableID, m.LabID, m.FirmwareVersion,
		m.OwnerID, m.RegisteredAt.Format(time.RFC3339), formatTime(m.LoadedAt),
		formatTime(m.InTransitAt), formatTime(m.ArrivedAt), formatTime(m.SignedAt),
		formatTime(m.CompletedAt), m.Version, m.ShardID, m.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	return nil
}

func (r *chargeSessionRepo) UpdateStateTx(ctx context.Context, tx Tx, id, fromState, toState string, version int) error {
	sqtx := tx.(*sqliteTx).tx
	res, err := sqtx.ExecContext(ctx,
		`UPDATE session_items SET state = ?, version = version + 1, negotiating_at = CASE WHEN ? = 'negotiating' THEN ? ELSE negotiating_at END,
			charging_at = CASE WHEN ? = 'charging' THEN ? ELSE charging_at END,
			signed_at = CASE WHEN ? = 'authorized' THEN ? ELSE signed_at END,
			completed_at = CASE WHEN ? = 'completed' THEN ? ELSE completed_at END
		 WHERE id = ? AND state = ? AND version = ?`,
		toState, toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		id, fromState, version)
	if err != nil {
		return fmt.Errorf("update session state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ConflictError{
			EntityID: id, Current: fromState, Attempted: toState,
			Reason: "state or version mismatch",
		}
	}
	return nil
}

const sessionColumns = "id, session_no, protocol_id, adapter_model, state, handshake_id, mitigation_id, device_model, charger_model, vendor_id, cable_id, lab_id, firmware_version, owner_id, requested_at, capability_checked_at, negotiating_at, charging_at, signed_at, completed_at, version, shard_id, data_version"

func (r *chargeSessionRepo) List(ctx context.Context, filter SessionFilter) ([]*domain.ChargeSession, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("protocol_id", filter.ProtocolID)
	w.eq("adapter_model", filter.AdapterModel)
	w.since("requested_at", filter.StartTime)
	w.until("requested_at", filter.EndTime)
	return queryPaged(ctx, r.store.db, "session_items", sessionColumns, "requested_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanSession)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSession(s rowScanner) (*domain.ChargeSession, error) {
	var m domain.ChargeSession
	var requestedAt, capability_checkedAt, inTransitAt, chargingAt, signedAt, completedAt string
	err := s.Scan(
		&m.ID, &m.SessionNo, &m.ProtocolID, &m.AdapterModel, &m.State, &m.HandshakeID, &m.MitigationID,
		&m.DeviceModel, &m.ChargerModel, &m.VendorID, &m.CableID, &m.LabID, &m.FirmwareVersion,
		&m.OwnerID, &requestedAt, &capability_checkedAt, &inTransitAt, &chargingAt, &signedAt, &completedAt,
		&m.Version, &m.ShardID, &m.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "session")
	}
	m.RegisteredAt = parseTime(requestedAt)
	m.LoadedAt = parseTime(capability_checkedAt)
	m.InTransitAt = parseTime(inTransitAt)
	m.ArrivedAt = parseTime(chargingAt)
	m.SignedAt = parseTime(signedAt)
	m.CompletedAt = parseTime(completedAt)
	return &m, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

type handshakeRepo struct {
	store *sqliteStore
}

func (r *handshakeRepo) Get(ctx context.Context, id string) (*domain.ProtocolHandshake, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, form_no, date, protocol_id, adapter_model, state, outbound_product, outbound_signer,
			outbound_signed_at, arrival_product, arrival_signer, arrival_signed_at, session_item_count,
			owner_id, requested_at, updated_at, version, shard_id, data_version
		 FROM handshake_forms WHERE id = ?`, id)
	return scanHandshake(row)
}

func (r *handshakeRepo) GetByFormNo(ctx context.Context, formNo string) (*domain.ProtocolHandshake, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, form_no, date, protocol_id, adapter_model, state, outbound_product, outbound_signer,
			outbound_signed_at, arrival_product, arrival_signer, arrival_signed_at, session_item_count,
			owner_id, requested_at, updated_at, version, shard_id, data_version
		 FROM handshake_forms WHERE form_no = ?`, formNo)
	return scanHandshake(row)
}

func (r *handshakeRepo) Save(ctx context.Context, h *domain.ProtocolHandshake) error {
	return r.store.persistWithShard(ctx, h.ShardID, h.ProtocolID, "handshake", h, func(ctx context.Context, tx *sql.Tx) error {
		return r.saveTx(ctx, tx, h)
	})
}

func (r *handshakeRepo) saveTx(ctx context.Context, tx *sql.Tx, h *domain.ProtocolHandshake) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO handshake_forms (id, form_no, date, protocol_id, adapter_model, state, outbound_product,
			outbound_signer, outbound_signed_at, arrival_product, arrival_signer, arrival_signed_at,
			session_item_count, owner_id, requested_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			form_no=excluded.form_no, date=excluded.date, protocol_id=excluded.protocol_id, adapter_model=excluded.adapter_model,
			state=excluded.state, outbound_product=excluded.outbound_product, outbound_signer=excluded.outbound_signer,
			outbound_signed_at=excluded.outbound_signed_at, arrival_product=excluded.arrival_product,
			arrival_signer=excluded.arrival_signer, arrival_signed_at=excluded.arrival_signed_at,
			session_item_count=excluded.session_item_count, owner_id=excluded.owner_id,
			updated_at=excluded.updated_at, version=excluded.version, shard_id=excluded.shard_id,
			data_version=excluded.data_version`,
		h.ID, h.FormNo, h.Date, h.ProtocolID, h.AdapterModel, h.State, h.OutboundProduct, h.OutboundSigner,
		formatTime(h.OutboundSignedAt), h.ArrivalProduct, h.ArrivalSigner, formatTime(h.ArrivalSignedAt),
		h.ChargeSessionCount, h.OwnerID, h.RegisteredAt.Format(time.RFC3339),
		h.UpdatedAt.Format(time.RFC3339), h.Version, h.ShardID, h.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert handshake: %w", err)
	}
	return nil
}

const handshakeColumns = "id, form_no, date, protocol_id, adapter_model, state, outbound_product, outbound_signer, outbound_signed_at, arrival_product, arrival_signer, arrival_signed_at, session_item_count, owner_id, requested_at, updated_at, version, shard_id, data_version"

func (r *handshakeRepo) List(ctx context.Context, filter HandshakeFilter) ([]*domain.ProtocolHandshake, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("protocol_id", filter.ProtocolID)
	w.eq("date", filter.Date)
	return queryPaged(ctx, r.store.db, "handshake_forms", handshakeColumns, "requested_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanHandshake)
}

func scanHandshake(s rowScanner) (*domain.ProtocolHandshake, error) {
	var h domain.ProtocolHandshake
	var requestedAt, updatedAt, outSignedAt, arrSignedAt string
	err := s.Scan(
		&h.ID, &h.FormNo, &h.Date, &h.ProtocolID, &h.AdapterModel, &h.State,
		&h.OutboundProduct, &h.OutboundSigner, &outSignedAt,
		&h.ArrivalProduct, &h.ArrivalSigner, &arrSignedAt,
		&h.ChargeSessionCount, &h.OwnerID, &requestedAt, &updatedAt,
		&h.Version, &h.ShardID, &h.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "handshake")
	}
	h.RegisteredAt = parseTime(requestedAt)
	h.UpdatedAt = parseTime(updatedAt)
	h.OutboundSignedAt = parseTime(outSignedAt)
	h.ArrivalSignedAt = parseTime(arrSignedAt)
	return &h, nil
}

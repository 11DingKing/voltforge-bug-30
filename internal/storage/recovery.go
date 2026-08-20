package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"voltforge/internal/domain"
)

type RecoveryReport struct {
	TotalShards   int
	RebuiltShards int
	DamagedShards []string
	TotalRecords  int
	Errors        []string
}

type shardRecord struct {
	Type string          `json:"type"`
	TS   string          `json:"ts"`
	Data json.RawMessage `json:"data"`
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func RecoverFromShards(ctx context.Context, store Store) (*RecoveryReport, error) {
	report := &RecoveryReport{}
	sqlite := store.(*sqliteStore)
	shardsDir := filepath.Join(sqlite.dataDir, "shards")
	dateDirs, err := os.ReadDir(shardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return nil, fmt.Errorf("read shards dir: %w", err)
	}
	for _, dateEntry := range dateDirs {
		if !dateEntry.IsDir() {
			continue
		}
		date := dateEntry.Name()
		protocolFiles, err := os.ReadDir(filepath.Join(shardsDir, date))
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("read date dir %s: %v", date, err))
			continue
		}
		for _, rf := range protocolFiles {
			if rf.IsDir() {
				continue
			}
			protocolID := rf.Name()
			if len(protocolID) > 6 && protocolID[len(protocolID)-6:] == ".jsonl" {
				protocolID = protocolID[:len(protocolID)-6]
			}
			shardID := domain.ShardIDFor(date, protocolID)
			report.TotalShards++
			if err := recoverShard(ctx, sqlite, shardID, date, protocolID, report); err != nil {
				report.Errors = append(report.Errors, err.Error())
			}
		}
	}
	return report, nil
}

func recoverShard(ctx context.Context, s *sqliteStore, shardID, date, protocolID string, report *RecoveryReport) error {
	path := s.shard.shardPath(shardID)
	data, err := os.ReadFile(path)
	if err != nil {
		report.DamagedShards = append(report.DamagedShards, shardID)
		return fmt.Errorf("%w: read shard %s", domain.ErrShardCorrupted, shardID)
	}
	checksum := computeChecksum(data)
	now := s.clock.Now()
	lines := splitLines(data)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for recovery: %w", err)
	}
	defer tx.Rollback()
	for _, line := range lines {
		var rec shardRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("unmarshal in %s: %v", shardID, err))
			continue
		}
		if err := replayRecord(ctx, tx, rec); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("replay %s in %s: %v", rec.Type, shardID, err))
			continue
		}
		report.TotalRecords++
	}
	meta := &ShardMeta{
		ShardID: shardID, Date: date, ProtocolID: protocolID,
		FilePath: path, Checksum: checksum, Status: ShardStatusOK,
		DataVersion: 1, RecordCount: len(lines),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := upsertShardMeta(ctx, tx, meta); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery: %w", err)
	}
	report.RebuiltShards++
	return nil
}

func replayRecord(ctx context.Context, ex execer, rec shardRecord) error {
	switch rec.Type {
	case "session":
		var m domain.ChargeSession
		if err := json.Unmarshal(rec.Data, &m); err != nil {
			return err
		}
		return replaySession(ctx, ex, &m)
	case "handshake":
		var h domain.ProtocolHandshake
		if err := json.Unmarshal(rec.Data, &h); err != nil {
			return err
		}
		return replayHandshake(ctx, ex, &h)
	case "mitigation":
		var d domain.SafetyMitigation
		if err := json.Unmarshal(rec.Data, &d); err != nil {
			return err
		}
		return replayMitigation(ctx, ex, &d)
	case "telemetry":
		var e domain.TelemetryEntry
		if err := json.Unmarshal(rec.Data, &e); err != nil {
			return err
		}
		_, err := ex.ExecContext(ctx,
			`INSERT OR REPLACE INTO telemetry_entries (id, date, protocol_id, volume_no, form_no, entry_type, session_no, owner_id, description, prev_state, next_state, created_at, shard_id, data_version)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.Date, e.ProtocolID, e.VolumeNo, e.FormNo, e.EntryType, e.SessionNo,
			e.OwnerID, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format(time.RFC3339), e.ShardID, e.DataVersion)
		return err
	default:
		return nil
	}
}

func replaySession(ctx context.Context, ex execer, m *domain.ChargeSession) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO session_items (id, session_no, protocol_id, adapter_model, state, handshake_id, mitigation_id,
			device_model, charger_model, vendor_id, cable_id, lab_id, firmware_version,
			owner_id, requested_at, capability_checked_at, negotiating_at, charging_at, signed_at, completed_at,
			version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version,
			handshake_id=excluded.handshake_id, mitigation_id=excluded.mitigation_id`,
		m.ID, m.SessionNo, m.ProtocolID, m.AdapterModel, m.State, m.HandshakeID, m.MitigationID,
		m.DeviceModel, m.ChargerModel, m.VendorID, m.CableID, m.LabID, m.FirmwareVersion,
		m.OwnerID, m.RegisteredAt.Format(time.RFC3339), formatTime(m.LoadedAt),
		formatTime(m.InTransitAt), formatTime(m.ArrivedAt), formatTime(m.SignedAt),
		formatTime(m.CompletedAt), m.Version, m.ShardID, m.DataVersion)
	return err
}

func replayHandshake(ctx context.Context, ex execer, h *domain.ProtocolHandshake) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO handshake_forms (id, form_no, date, protocol_id, adapter_model, state, outbound_product,
			outbound_signer, outbound_signed_at, arrival_product, arrival_signer, arrival_signed_at,
			session_item_count, owner_id, requested_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version`,
		h.ID, h.FormNo, h.Date, h.ProtocolID, h.AdapterModel, h.State, h.OutboundProduct, h.OutboundSigner,
		formatTime(h.OutboundSignedAt), h.ArrivalProduct, h.ArrivalSigner, formatTime(h.ArrivalSignedAt),
		h.ChargeSessionCount, h.OwnerID, h.RegisteredAt.Format(time.RFC3339),
		h.UpdatedAt.Format(time.RFC3339), h.Version, h.ShardID, h.DataVersion)
	return err
}

func replayMitigation(ctx context.Context, ex execer, d *domain.SafetyMitigation) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO mitigation_requests (id, request_no, session_id, session_no, type, target_address, state,
			submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at,
			executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason,
			lost_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version`,
		d.ID, d.RequestNo, d.SessionID, d.SessionNo, d.Type, d.TargetAddress, d.State,
		d.SubmittedBy, d.SubmittedAt.Format(time.RFC3339), d.ReviewedBy,
		formatTime(d.ReviewedAt), d.ReviewNote, d.IssuedBy, formatTime(d.IssuedAt),
		formatTime(d.ExecutedAt), formatTime(d.CompletedAt), d.WithdrawnBy,
		formatTime(d.WithdrawnAt), d.WithdrawnReason, d.ConflictReason, formatTime(d.LostAt),
		d.Version, d.ShardID, d.DataVersion)
	return err
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if start < i {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}

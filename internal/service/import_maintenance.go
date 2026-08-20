package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

type ImportExportService struct {
	handSvc *HandshakeService
	clock   domain.Clock
}

func NewImportExportService(handSvc *HandshakeService, clock domain.Clock) *ImportExportService {
	return &ImportExportService{handSvc: handSvc, clock: clock}
}

type ImportRowResult struct {
	RowNumber int    `json:"row_number"`
	Status    string `json:"status"`
	FormNo    string `json:"form_no"`
	Error     string `json:"error,omitempty"`
}

type ImportResult struct {
	TotalRows  int               `json:"total_rows"`
	Succeeded  int               `json:"succeeded"`
	Failed     int               `json:"failed"`
	RowResults []ImportRowResult `json:"row_results"`
}

func (s *ImportExportService) ImportHandshakesCSV(ctx context.Context, reader io.Reader) (*ImportResult, error) {
	csvReader := csv.NewReader(reader)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	result := &ImportResult{TotalRows: 0}
	if len(records) < 2 {
		return result, nil
	}
	header := records[0]
	headerMap := make(map[string]int)
	for i, h := range header {
		headerMap[strings.TrimSpace(h)] = i
	}
	for rowIdx, row := range records[1:] {
		result.TotalRows++
		rowNum := rowIdx + 2
		formNo := getCSVField(row, headerMap, "form_no")
		date := getCSVField(row, headerMap, "date")
		protocolID := getCSVField(row, headerMap, "protocol_id")
		vehicleNo := getCSVField(row, headerMap, "adapter_model")
		outboundProduct := getCSVField(row, headerMap, "outbound_product")
		arrivalProduct := getCSVField(row, headerMap, "arrival_product")
		owner_id := getCSVField(row, headerMap, "owner_id")
		sessionNo := getCSVField(row, headerMap, "session_no")
		senderName := getCSVField(row, headerMap, "vendor_id")
		senderAddr := getCSVField(row, headerMap, "cable_id")
		receiverName := getCSVField(row, headerMap, "lab_id")
		receiverAddr := getCSVField(row, headerMap, "firmware_version")
		if formNo == "" || date == "" || protocolID == "" {
			result.Failed++
			result.RowResults = append(result.RowResults, ImportRowResult{
				RowNumber: rowNum, Status: "failed", FormNo: formNo, Error: "missing required fields",
			})
			continue
		}
		req := RegisterHandshakeRequest{
			FormNo: formNo, Date: date, ProtocolID: protocolID, AdapterModel: vehicleNo,
			OutboundProduct: outboundProduct, ArrivalProduct: arrivalProduct, OwnerID: owner_id,
			ChargeSessions: []RegisterChargeSession{{
				SessionNo: sessionNo, VendorID: senderName, CableID: senderAddr,
				LabID: receiverName, FirmwareVersion: receiverAddr,
			}},
		}
		rr := s.registerRow(ctx, req, rowNum)
		if rr.Status == "failed" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.RowResults = append(result.RowResults, rr)
	}
	return result, nil
}

func (s *ImportExportService) registerRow(ctx context.Context, req RegisterHandshakeRequest, rowNum int) ImportRowResult {
	if _, err := s.handSvc.Register(ctx, req); err != nil {
		return ImportRowResult{RowNumber: rowNum, Status: "failed", FormNo: req.FormNo, Error: err.Error()}
	}
	return ImportRowResult{RowNumber: rowNum, Status: "succeeded", FormNo: req.FormNo}
}

func (s *ImportExportService) ImportHandshakesJSON(ctx context.Context, reader io.Reader) (*ImportResult, error) {
	dec := json.NewDecoder(reader)
	var records []RegisterHandshakeRequest
	if err := dec.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	result := &ImportResult{TotalRows: len(records)}
	for i, req := range records {
		rr := s.registerRow(ctx, req, i+1)
		if rr.Status == "failed" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.RowResults = append(result.RowResults, rr)
	}
	return result, nil
}

func getCSVField(row []string, headerMap map[string]int, field string) string {
	idx, ok := headerMap[field]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

type MaintenanceService struct {
	store storage.Store
	clock domain.Clock
}

func NewMaintenanceService(store storage.Store, clock domain.Clock) *MaintenanceService {
	return &MaintenanceService{store: store, clock: clock}
}

type CertifyReport struct {
	TotalShards   int      `json:"total_shards"`
	OKShards      int      `json:"ok_shards"`
	DamagedShards int      `json:"damaged_shards"`
	Errors        []string `json:"errors"`
}

type shardChecker interface {
	ShardExists(shardID string) bool
	ShardChecksum(shardID string) (string, int, error)
}

func (s *MaintenanceService) Certify(ctx context.Context) (*CertifyReport, error) {
	report := &CertifyReport{}
	shards, err := s.store.ShardRepo().ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	checker, ok := s.store.(shardChecker)
	if !ok {
		return nil, fmt.Errorf("store does not support shard checking")
	}
	report.TotalShards = len(shards)
	for _, shard := range shards {
		if shard.Status == storage.ShardStatusDamaged {
			report.DamagedShards++
			continue
		}
		if !checker.ShardExists(shard.ShardID) {
			report.DamagedShards++
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s file missing", shard.ShardID))
			continue
		}
		checksum, _, err := checker.ShardChecksum(shard.ShardID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s checksum error: %v", shard.ShardID, err))
			report.DamagedShards++
			continue
		}
		if shard.Checksum != "" && shard.Checksum != checksum {
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s checksum mismatch", shard.ShardID))
			report.DamagedShards++
			continue
		}
		report.OKShards++
	}
	return report, nil
}

func (s *MaintenanceService) RebuildIndex(ctx context.Context) (*storage.RecoveryReport, error) {
	return storage.RecoverFromShards(ctx, s.store)
}

type BatchMaintenanceResult struct {
	TotalChecked int      `json:"total_checked"`
	Updated      int      `json:"updated"`
	Skipped      int      `json:"skipped"`
	Errors       []string `json:"errors"`
}

func (s *MaintenanceService) BatchCertifyChargeSessions(ctx context.Context, sessionIDs []string) (*BatchMaintenanceResult, error) {
	result := &BatchMaintenanceResult{}
	for _, id := range sessionIDs {
		result.TotalChecked++
		session, err := s.store.ChargeSessionRepo().Get(ctx, id)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("session %s: %v", id, err))
			continue
		}
		if session.State == "" || session.SessionNo == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("session %s: invalid data", id))
			continue
		}
		if session.Version <= 0 {
			session.Version = 1
			session.DataVersion = 1
			if err := s.store.ChargeSessionRepo().Save(ctx, session); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("session %s: save failed: %v", id, err))
				continue
			}
			result.Updated++
			continue
		}
		result.Skipped++
	}
	return result, nil
}

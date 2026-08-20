package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"voltforge/internal/charging"
	"voltforge/internal/domain"
)

func (s *sqliteStore) CreateProduct(ctx context.Context, product *charging.Product) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_products(id,name,market,vendor_id,status,supported_protocols,max_power_watts,gan,port_count,thermal_limit_c,battery_architecture,cell_count,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		product.ID, product.Name, product.Market, product.VendorID, product.Status, product.SupportedProtocols,
		product.MaxPowerWatts, product.GaN, product.PortCount, product.ThermalLimitC,
		product.BatteryArchitecture, product.CellCount, product.CreatedAt.Format(time.RFC3339Nano),
		product.UpdatedAt.Format(time.RFC3339Nano), product.Version)
	return err
}
func (s *sqliteStore) GetProduct(ctx context.Context, id string) (*charging.Product, error) {
	var v charging.Product
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,market,vendor_id,status,supported_protocols,max_power_watts,gan,port_count,thermal_limit_c,battery_architecture,cell_count,created_at,updated_at,version FROM charging_products WHERE id=?`, id).Scan(
		&v.ID, &v.Name, &v.Market, &v.VendorID, &v.Status, &v.SupportedProtocols, &v.MaxPowerWatts,
		&v.GaN, &v.PortCount, &v.ThermalLimitC, &v.BatteryArchitecture, &v.CellCount, &created, &updated, &v.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, charging.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &v, nil
}
func (s *sqliteStore) ListProducts(ctx context.Context, limit, offset int) ([]charging.Product, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM charging_products`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,market,vendor_id,status,supported_protocols,max_power_watts,gan,port_count,thermal_limit_c,battery_architecture,cell_count,created_at,updated_at,version FROM charging_products ORDER BY name LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []charging.Product{}
	for rows.Next() {
		var v charging.Product
		var created, updated string
		if err := rows.Scan(&v.ID, &v.Name, &v.Market, &v.VendorID, &v.Status, &v.SupportedProtocols,
			&v.MaxPowerWatts, &v.GaN, &v.PortCount, &v.ThermalLimitC, &v.BatteryArchitecture,
			&v.CellCount, &created, &updated, &v.Version); err != nil {
			return nil, 0, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, v)
	}
	return out, total, rows.Err()
}
func (s *sqliteStore) CreateIssue(ctx context.Context, h *charging.Issue) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_issues(id,product_id,kind,severity,description,state,submitted_by,vendor_assignee_id,review_due_at,firmware_evidence,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, h.ID, h.ProductID, h.Kind, h.Severity, h.Description, h.State, h.SubmittedBy, h.VendorAssigneeID, h.ReviewDueAt.Format(time.RFC3339Nano), h.FirmwareEvidence, h.Version, h.CreatedAt.Format(time.RFC3339Nano), h.UpdatedAt.Format(time.RFC3339Nano))
	return err
}
func (s *sqliteStore) GetIssue(ctx context.Context, id string) (*charging.Issue, error) {
	var h charging.Issue
	var due, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,product_id,kind,severity,description,state,submitted_by,vendor_assignee_id,review_due_at,firmware_evidence,version,created_at,updated_at FROM charging_issues WHERE id=?`, id).Scan(&h.ID, &h.ProductID, &h.Kind, &h.Severity, &h.Description, &h.State, &h.SubmittedBy, &h.VendorAssigneeID, &due, &h.FirmwareEvidence, &h.Version, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, charging.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h.ReviewDueAt, _ = time.Parse(time.RFC3339Nano, due)
	h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &h, nil
}
func (s *sqliteStore) TransitionIssue(ctx context.Context, h *charging.Issue, next charging.IssueState, assigned, firmware_evidence string) error {
	now := s.clock.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE charging_issues SET state=?,vendor_assignee_id=CASE WHEN ?='' THEN vendor_assignee_id ELSE ? END,firmware_evidence=CASE WHEN ?='' THEN firmware_evidence ELSE ? END,updated_at=?,version=version+1 WHERE id=? AND version=?`, next, assigned, assigned, firmware_evidence, firmware_evidence, now.Format(time.RFC3339Nano), h.ID, h.Version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return domain.ErrConflict
	}
	return nil
}
func (s *sqliteStore) CreateChargeTest(ctx context.Context, i *charging.ChargeTest) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_charge_tests(id,product_id,lab_engineer_id,checked_at,protocol_handshake_ok,cable_certificate_expiry,thermal_control_ok,power_display_ok,notes) VALUES(?,?,?,?,?,?,?,?,?)`, i.ID, i.ProductID, i.LabEngineerID, i.CheckedAt.Format(time.RFC3339Nano), i.ProtocolHandshakeOK, i.CableCertificateExpiry.Format(time.RFC3339Nano), i.ThermalControlOK, i.PowerDisplayOK, i.Notes)
	return err
}
func (s *sqliteStore) ListOpenIssues(ctx context.Context, productID string, limit, offset int) ([]charging.Issue, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM charging_issues WHERE (?='' OR product_id=?) AND state<>'certified'`, productID, productID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,product_id,kind,severity,description,state,submitted_by,vendor_assignee_id,review_due_at,firmware_evidence,version,created_at,updated_at FROM charging_issues WHERE (?='' OR product_id=?) AND state<>'certified' ORDER BY review_due_at LIMIT ? OFFSET ?`, productID, productID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []charging.Issue{}
	for rows.Next() {
		var h charging.Issue
		var due, created, updated string
		if err := rows.Scan(&h.ID, &h.ProductID, &h.Kind, &h.Severity, &h.Description, &h.State, &h.SubmittedBy, &h.VendorAssigneeID, &due, &h.FirmwareEvidence, &h.Version, &created, &updated); err != nil {
			return nil, 0, err
		}
		h.ReviewDueAt, _ = time.Parse(time.RFC3339Nano, due)
		h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, h)
	}
	return out, total, rows.Err()
}

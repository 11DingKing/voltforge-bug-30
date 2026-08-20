package httpapi

import (
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"voltforge/internal/charging"
)

func (s *Server) chargingToken(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if len(token) >= 7 && token[:7] == "Bearer " {
		return token[7:]
	}
	return token
}
func (s *Server) chargingError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, charging.ErrNotFound) {
		status = http.StatusNotFound
	}
	if err.Error() == "forbidden" {
		status = http.StatusForbidden
	}
	http.Error(w, `{"code":"charging_request_failed","message":"`+err.Error()+`"}`, status)
}
func (s *Server) CreateChargingProduct(w http.ResponseWriter, r *http.Request) {
	var v charging.Product
	if json.NewDecoder(r.Body).Decode(&v) != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	out, err := s.chargingSvc.CreateProduct(r.Context(), s.chargingToken(r), v)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}
func (s *Server) ListChargingProducts(w http.ResponseWriter, r *http.Request) {
	items, total, err := s.store.ListProducts(r.Context(), 20, 0)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": total})
}
func (s *Server) ReportChargingIssue(w http.ResponseWriter, r *http.Request) {
	var v charging.Issue
	if json.NewDecoder(r.Body).Decode(&v) != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	out, err := s.chargingSvc.ReportIssue(r.Context(), s.chargingToken(r), v)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}
func (s *Server) AssignChargingIssue(w http.ResponseWriter, r *http.Request) {
	var v struct {
		VendorID string `json:"vendor_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.AssignIssue(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.VendorID)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) MitigateChargingIssue(w http.ResponseWriter, r *http.Request) {
	var v struct {
		FirmwareEvidence string `json:"firmware_evidence"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.MitigateIssue(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.FirmwareEvidence)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) CertifyChargingIssue(w http.ResponseWriter, r *http.Request) {
	var v struct {
		FirmwareEvidence string `json:"firmware_evidence"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.CertifyIssue(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.FirmwareEvidence)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

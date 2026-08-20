package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
	"voltforge/internal/domain"
)

func (s *Server) QueryTelemetry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	query := domain.TelemetryQuery{
		Date: q.Get("date"), ProtocolID: q.Get("protocol_id"),
		OwnerID: q.Get("owner_id"), EntryType: q.Get("entry_type"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	if st := q.Get("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			query.StartTime = t
		}
	}
	if et := q.Get("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			query.EndTime = t
		}
	}
	entries, total, err := s.telemetrySvc.Query(r.Context(), query)
	respondJSON(w, newPaginated(entries, total, pageSize, pageOffset), err)
}

func (s *Server) ExportTelemetry(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date := q.Get("date")
	protocolID := q.Get("protocol_id")
	csvData, err := s.telemetrySvc.ExportCSV(r.Context(), date, protocolID)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=telemetry_"+date+"_"+protocolID+".csv")
	w.Write([]byte(csvData))
}

func (s *Server) ListVolumes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date := q.Get("date")
	volumes, err := s.telemetrySvc.ListVolumes(r.Context(), date)
	respondJSON(w, volumes, err)
}

func (s *Server) QueryAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	query := domain.AuditQuery{
		Actor: q.Get("actor"), EntityType: q.Get("entity_type"),
		EntityID: q.Get("entity_id"), Action: q.Get("action"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	if st := q.Get("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			query.StartTime = t
		}
	}
	if et := q.Get("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			query.EndTime = t
		}
	}
	records, total, err := s.telemetrySvc.AuditQuery(r.Context(), query)
	respondJSON(w, newPaginated(records, total, pageSize, pageOffset), err)
}

func (s *Server) ImportHandshakes(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	var result any
	var err error
	if strings.Contains(ct, "json") {
		result, err = s.importSvc.ImportHandshakesJSON(r.Context(), r.Body)
	} else {
		result, err = s.importSvc.ImportHandshakesCSV(r.Context(), r.Body)
	}
	respondJSON(w, result, err)
}

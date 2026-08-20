package httpapi

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"voltforge/internal/domain"
)

func (s *Server) GetOverdues(w http.ResponseWriter, r *http.Request) {
	report, err := s.overdueSvc.GetOverdueReport(r.Context())
	respondJSON(w, report, err)
}

func (s *Server) ListFailures(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status == "" {
		status = domain.FailureStatusPermanent
	}
	failures, total, err := s.overdueSvc.ListFailures(r.Context(), status)
	if err != nil {
		writeError(w, err)
		return
	}
	pageSize, _ := strconv.Atoi(q.Get("page_size"))
	if pageSize <= 0 {
		pageSize = 20
	}
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	writeJSON(w, newPaginated(failures, total, pageSize, pageOffset))
}

func (s *Server) ResolveFailure(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, domain.ValidationError{Field: "id", Message: "must be a number"})
		return
	}
	if err := s.overdueSvc.ResolveFailure(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]string{"status": "resolved"})
}

func (s *Server) CertifyData(w http.ResponseWriter, r *http.Request) {
	report, err := s.maintSvc.Certify(r.Context())
	respondJSON(w, report, err)
}

func (s *Server) RebuildIndex(w http.ResponseWriter, r *http.Request) {
	report, err := s.maintSvc.RebuildIndex(r.Context())
	respondJSON(w, report, err)
}

func (s *Server) BatchCheckSessions(w http.ResponseWriter, r *http.Request) {
	var req BatchCheckRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.maintSvc.BatchCertifyChargeSessions(r.Context(), req.SessionIDs)
	respondJSON(w, result, err)
}

func (s *Server) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, map[string]string{"status": "ok"})
}

type ReadyCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func (s *Server) Readyz(w http.ResponseWriter, r *http.Request) {
	checks := []ReadyCheck{}
	allOK := true
	if err := s.store.Ping(r.Context()); err != nil {
		checks = append(checks, ReadyCheck{Name: "database", Status: "fail", Error: err.Error()})
		allOK = false
	} else {
		checks = append(checks, ReadyCheck{Name: "database", Status: "ok"})
	}
	testFile := filepath.Join(s.store.DataDir(), ".readyz")
	if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
		checks = append(checks, ReadyCheck{Name: "data_dir", Status: "fail", Error: err.Error()})
		allOK = false
	} else {
		os.Remove(testFile)
		checks = append(checks, ReadyCheck{Name: "data_dir", Status: "ok"})
	}
	if !s.scheduler.IsRunning() {
		checks = append(checks, ReadyCheck{Name: "scheduler", Status: "fail", Error: "not running"})
		allOK = false
	} else {
		checks = append(checks, ReadyCheck{Name: "scheduler", Status: "ok"})
	}
	if _, err := s.store.ChargeSessionRepo().Get(r.Context(), "readyz-probe"); err != nil {
		if !isNotFoundErr(err) {
			checks = append(checks, ReadyCheck{Name: "query", Status: "fail", Error: err.Error()})
			allOK = false
		} else {
			checks = append(checks, ReadyCheck{Name: "query", Status: "ok"})
		}
	} else {
		checks = append(checks, ReadyCheck{Name: "query", Status: "ok"})
	}
	status := http.StatusOK
	if !allOK {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, map[string]any{
		"status": statusStr(allOK), "checks": checks,
	})
}

func statusStr(ok bool) string {
	if ok {
		return "ready"
	}
	return "not_ready"
}

func isNotFoundErr(err error) bool {
	return err != nil && errors.Is(err, domain.ErrNotFound)
}

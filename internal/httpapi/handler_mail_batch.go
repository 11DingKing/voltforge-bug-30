package httpapi

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
	"voltforge/internal/service"
	"voltforge/internal/storage"
)

func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.SessionFilter{
		State: q.Get("state"), ProtocolID: q.Get("protocol_id"), AdapterModel: q.Get("adapter_model"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	if st := q.Get("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			filter.StartTime = t
		}
	}
	if et := q.Get("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			filter.EndTime = t
		}
	}
	sessions, total, err := s.store.ChargeSessionRepo().List(r.Context(), filter)
	respondJSON(w, newPaginated(sessions, total, pageSize, pageOffset), err)
}

func (s *Server) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	session, err := s.store.ChargeSessionRepo().Get(r.Context(), id)
	respondJSON(w, session, err)
}

func (s *Server) GetSessionMitigations(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dispS, err := s.dispSvc.GetActiveBySession(r.Context(), id)
	respondJSON(w, dispS, err)
}

func (s *Server) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req CreateBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	batch, err := s.batchSvc.CreateBatch(r.Context(), service.CreateBatchRequest{
		AdapterModel: req.AdapterModel, Date: req.Date, ProtocolID: req.ProtocolID,
	})
	respondJSON(w, batch, err)
}

func (s *Server) ProcessBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, err := s.batchSvc.ProcessBatch(r.Context(), id)
	respondJSON(w, batch, err)
}

func (s *Server) GetBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, err := s.batchSvc.GetBatch(r.Context(), id)
	respondJSON(w, batch, err)
}

func (s *Server) ListBatchItems(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := s.batchSvc.ListItems(r.Context(), id)
	respondJSON(w, items, err)
}

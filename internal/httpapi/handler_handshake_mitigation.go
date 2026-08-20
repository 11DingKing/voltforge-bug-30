package httpapi

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"voltforge/internal/domain"
	"voltforge/internal/service"
	"voltforge/internal/storage"
)

func (s *Server) RegisterHandshake(w http.ResponseWriter, r *http.Request) {
	var req RegisterHandshakeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	chargeSessions := make([]service.RegisterChargeSession, len(req.ChargeSessions))
	for i, mi := range req.ChargeSessions {
		chargeSessions[i] = service.RegisterChargeSession{
			SessionNo: mi.SessionNo, VendorID: mi.VendorID, CableID: mi.CableID,
			LabID: mi.LabID, FirmwareVersion: mi.FirmwareVersion,
		}
	}
	form, err := s.handSvc.Register(r.Context(), service.RegisterHandshakeRequest{
		FormNo: req.FormNo, Date: req.Date, ProtocolID: req.ProtocolID, AdapterModel: req.AdapterModel,
		OutboundProduct: req.OutboundProduct, ArrivalProduct: req.ArrivalProduct,
		OwnerID: req.OwnerID, ChargeSessions: chargeSessions,
	})
	respondJSON(w, form, err)
}

func (s *Server) GetHandshake(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	form, err := s.handSvc.Get(r.Context(), id)
	respondJSON(w, form, err)
}

func (s *Server) ListHandshakes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.HandshakeFilter{
		State: q.Get("state"), ProtocolID: q.Get("protocol_id"), Date: q.Get("date"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	forms, total, err := s.handSvc.List(r.Context(), filter)
	respondJSON(w, newPaginated(forms, total, pageSize, pageOffset), err)
}

func (s *Server) ModifyHandshake(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req RegisterHandshakeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	form, err := s.handSvc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if form.State != domain.HandshakeStateDraft {
		writeError(w, domain.ErrInvalidTransition)
		return
	}
	form.AdapterModel = req.AdapterModel
	form.OutboundProduct = req.OutboundProduct
	form.ArrivalProduct = req.ArrivalProduct
	form.OwnerID = req.OwnerID
	form.UpdatedAt = form.RegisteredAt
	form.Version++
	if err := s.handSvc.ModifySave(r.Context(), form); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, form)
}

func (s *Server) HandshakeAttestation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req AttestationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	form, err := s.handSvc.AttestationWithLock(r.Context(), domain.HandshakeAttestationRequest{
		FormID: id, Party: req.Party, Signer: req.Signer, Product: req.Product,
	})
	respondJSON(w, form, err)
}

func (s *Server) SubmitMitigation(w http.ResponseWriter, r *http.Request) {
	var req SubmitSafetyMitigation
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Submit(r.Context(), service.SubmitSafetyMitigation{
		RequestNo: req.RequestNo, SessionID: req.SessionID, Type: req.Type,
		TargetAddress: req.TargetAddress, SubmittedBy: req.SubmittedBy,
	})
	respondJSON(w, disp, err)
}

func (s *Server) GetMitigation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	disp, err := s.dispSvc.Get(r.Context(), id)
	respondJSON(w, disp, err)
}

func (s *Server) ListMitigations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.MitigationFilter{
		State: q.Get("state"), SessionID: q.Get("session_id"),
		SubmittedBy: q.Get("submitted_by"),
		PageSize:    pageSize, PageOffset: pageOffset,
	}
	dispS, total, err := s.dispSvc.List(r.Context(), filter)
	respondJSON(w, newPaginated(dispS, total, pageSize, pageOffset), err)
}

func (s *Server) ReviewMitigation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req ReviewSafetyMitigation
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Review(r.Context(), service.ReviewRequest{
		MitigationID: id, Reviewer: req.Reviewer, Decision: req.Decision, Note: req.Note,
	})
	respondJSON(w, disp, err)
}

func (s *Server) WithdrawMitigation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req WithdrawSafetyMitigation
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Withdraw(r.Context(), service.WithdrawRequest{
		MitigationID: id, WithdrawnBy: req.WithdrawnBy, Reason: req.Reason,
	})
	respondJSON(w, disp, err)
}

func (s *Server) ExecuteMitigation(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req ExecuteSafetyMitigation
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Execute(r.Context(), id, req.Actor)
	respondJSON(w, disp, err)
}

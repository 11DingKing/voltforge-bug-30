package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"voltforge/internal/domain"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, rerr error) {
	if rerr == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	status, code := classifyError(rerr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, ErrorResponse{Error: ErrorBody{Code: code, Message: rerr.Error()}})
}

func classifyError(err error) (int, string) {
	var valErr domain.ValidationError
	if errors.As(err, &valErr) {
		return http.StatusBadRequest, "validation_error"
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "already_exists"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusConflict, "invalid_transition"
	case errors.Is(err, domain.ErrDuplicateRequest):
		return http.StatusConflict, "duplicate"
	case errors.Is(err, domain.ErrMitigationActive):
		return http.StatusConflict, "mitigation_active"
	case errors.Is(err, domain.ErrAttestationIncomplete):
		return http.StatusBadRequest, "attestation_incomplete"
	case errors.Is(err, domain.ErrShardCorrupted):
		return http.StatusInternalServerError, "shard_corrupted"
	case errors.Is(err, domain.ErrSlowConsumer):
		return http.StatusServiceUnavailable, "slow_consumer"
	case errors.Is(err, domain.ErrStaleCheckpoint):
		return http.StatusConflict, "stale_checkpoint"
	case errors.Is(err, domain.ErrPermanentFailure):
		return http.StatusInternalServerError, "permanent_failure"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parsePageSize(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 {
		n = 20
	}
	if n > 200 {
		n = 200
	}
	return n
}

func newPaginated(data any, total, pageSize, offset int) PaginatedResponse {
	return PaginatedResponse{
		Data: data, Total: total, PageSize: pageSize, PageOffset: offset,
		HasNext: offset+pageSize < total,
	}
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func respondJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, v)
}

type PaginatedResponse struct {
	Data       any  `json:"data"`
	Total      int  `json:"total"`
	PageSize   int  `json:"page_size"`
	PageOffset int  `json:"page_offset"`
	HasNext    bool `json:"has_next"`
}

type RegisterHandshakeRequest struct {
	FormNo          string             `json:"form_no"`
	Date            string             `json:"date"`
	ProtocolID      string             `json:"protocol_id"`
	AdapterModel    string             `json:"adapter_model"`
	OutboundProduct string             `json:"outbound_product"`
	ArrivalProduct  string             `json:"arrival_product"`
	OwnerID         string             `json:"owner_id"`
	ChargeSessions  []ChargeSessionDTO `json:"session_items"`
}

type ChargeSessionDTO struct {
	SessionNo       string `json:"session_no"`
	VendorID        string `json:"vendor_id"`
	CableID         string `json:"cable_id"`
	LabID           string `json:"lab_id"`
	FirmwareVersion string `json:"firmware_version"`
}

type AttestationRequest struct {
	Party   string `json:"party"`
	Signer  string `json:"signer"`
	Product string `json:"product"`
}

type SubmitSafetyMitigation struct {
	RequestNo     string `json:"request_no"`
	SessionID     string `json:"session_id"`
	Type          string `json:"type"`
	TargetAddress string `json:"target_address"`
	SubmittedBy   string `json:"submitted_by"`
}

type ReviewSafetyMitigation struct {
	Reviewer string `json:"reviewer"`
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

type WithdrawSafetyMitigation struct {
	WithdrawnBy string `json:"withdrawn_by"`
	Reason      string `json:"reason"`
}

type ExecuteSafetyMitigation struct {
	Actor string `json:"actor"`
}

type CreateBatchRequest struct {
	AdapterModel string `json:"adapter_model"`
	Date         string `json:"date"`
	ProtocolID   string `json:"protocol_id"`
	Mitigations  []struct {
		SessionID     string `json:"session_id"`
		Type          string `json:"type"`
		TargetAddress string `json:"target_address"`
	} `json:"mitigations"`
}

type RegisterSubscriberRequest struct {
	SubscriberID   string `json:"subscriber_id"`
	SubscriberType string `json:"subscriber_type"`
	Name           string `json:"name"`
}

type BatchCheckRequest struct {
	SessionIDs []string `json:"session_ids"`
}

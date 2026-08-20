package httpapi

import (
	"github.com/go-chi/chi/v5"
	"net/http"
	"voltforge/internal/domain"
)

func (s *Server) SubscriptionStream(w http.ResponseWriter, r *http.Request) {
	subscriberID := r.URL.Query().Get("subscriber_id")
	if subscriberID == "" {
		subscriberID = chi.URLParam(r, "subscriber_id")
	}
	if subscriberID == "" {
		http.Error(w, "subscriber_id required", http.StatusBadRequest)
		return
	}
	subType := r.URL.Query().Get("subscriber_type")
	if subType == "" {
		subType = domain.SubscriberTypeDispatcher
	}
	s.subSvc.EnsureSubscriber(r.Context(), domain.SubscriptionRequest{
		SubscriberID: subscriberID, SubscriberType: subType,
	})
	s.subSvc.HandleSSE(w, r, subscriberID)
}

func (s *Server) RegisterSubscriber(w http.ResponseWriter, r *http.Request) {
	var req RegisterSubscriberRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	sub, err := s.subSvc.EnsureSubscriber(r.Context(), domain.SubscriptionRequest{
		SubscriberID: req.SubscriberID, SubscriberType: req.SubscriberType,
		Name: req.Name,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, sub)
}

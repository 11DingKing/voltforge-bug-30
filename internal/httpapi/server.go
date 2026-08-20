package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
	"time"
	"voltforge/internal/audit"
	"voltforge/internal/auth"
	"voltforge/internal/charging"
	"voltforge/internal/config"
	"voltforge/internal/scheduler"
	"voltforge/internal/service"
	"voltforge/internal/storage"
)

type Server struct {
	cfg          *config.Config
	logger       *audit.Logger
	store        storage.Store
	handSvc      *service.HandshakeService
	dispSvc      *service.MitigationService
	batchSvc     *service.BatchService
	subSvc       *service.SubscriptionService
	telemetrySvc *service.TelemetryService
	importSvc    *service.ImportExportService
	overdueSvc   *service.OverdueService
	maintSvc     *service.MaintenanceService
	scheduler    *scheduler.Scheduler
	authSvc      *auth.Service
	chargingSvc  *charging.Service
}

func NewServer(
	cfg *config.Config,
	logger *audit.Logger,
	store storage.Store,
	handSvc *service.HandshakeService,
	dispSvc *service.MitigationService,
	batchSvc *service.BatchService,
	subSvc *service.SubscriptionService,
	telemetrySvc *service.TelemetryService,
	importSvc *service.ImportExportService,
	overdueSvc *service.OverdueService,
	maintSvc *service.MaintenanceService,
	sched *scheduler.Scheduler,
	authServices ...*auth.Service,
) *Server {
	authSvc := auth.New(8*time.Hour, time.Now)
	if len(authServices) > 0 && authServices[0] != nil {
		authSvc = authServices[0]
	}
	return &Server{
		cfg: cfg, logger: logger, store: store,
		handSvc: handSvc, dispSvc: dispSvc, batchSvc: batchSvc,
		subSvc: subSvc, telemetrySvc: telemetrySvc, importSvc: importSvc,
		overdueSvc: overdueSvc, maintSvc: maintSvc, scheduler: sched, authSvc: authSvc,
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(s.logger.Middleware)
	r.Use(s.timeoutMiddleware)
	r.Get("/healthz", s.Healthz)
	r.Get("/readyz", s.Readyz)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/login", s.Login)
		r.Post("/auth/logout", s.Logout)
		r.Post("/charging/products", s.CreateChargingProduct)
		r.Get("/charging/products", s.ListChargingProducts)
		r.Post("/charging/issues", s.ReportChargingIssue)
		r.Post("/charging/issues/{id}/assign", s.AssignChargingIssue)
		r.Post("/charging/issues/{id}/mitigate", s.MitigateChargingIssue)
		r.Post("/charging/issues/{id}/certify", s.CertifyChargingIssue)
		r.Post("/handshakes", s.RegisterHandshake)
		r.Get("/handshakes", s.ListHandshakes)
		r.Get("/handshakes/{id}", s.GetHandshake)
		r.Put("/handshakes/{id}", s.ModifyHandshake)
		r.Post("/handshakes/{id}/attestation", s.HandshakeAttestation)
		r.Post("/mitigations", s.SubmitMitigation)
		r.Get("/mitigations", s.ListMitigations)
		r.Get("/mitigations/{id}", s.GetMitigation)
		r.Post("/mitigations/{id}/review", s.ReviewMitigation)
		r.Post("/mitigations/{id}/withdraw", s.WithdrawMitigation)
		r.Post("/mitigations/{id}/execute", s.ExecuteMitigation)
		r.Get("/sessions", s.ListSessions)
		r.Get("/sessions/{id}", s.GetSession)
		r.Get("/sessions/{id}/mitigations", s.GetSessionMitigations)
		r.Post("/batches", s.CreateBatch)
		r.Post("/batches/{id}/process", s.ProcessBatch)
		r.Get("/batches/{id}", s.GetBatch)
		r.Get("/batches/{id}/items", s.ListBatchItems)
		r.Get("/subscriptions/stream", s.SubscriptionStream)
		r.Post("/subscribers", s.RegisterSubscriber)
		r.Post("/imports/handshakes", s.ImportHandshakes)
		r.Get("/telemetry", s.QueryTelemetry)
		r.Get("/telemetry/export", s.ExportTelemetry)
		r.Get("/telemetry/volumes", s.ListVolumes)
		r.Get("/audit", s.QueryAudit)
		r.Get("/overdues", s.GetOverdues)
		r.Get("/failures", s.ListFailures)
		r.Post("/failures/{id}/resolve", s.ResolveFailure)
		r.Post("/maintenance/certify", s.CertifyData)
		r.Post("/maintenance/rebuild", s.RebuildIndex)
		r.Post("/maintenance/batch-check", s.BatchCheckSessions)
	})
	return r
}

func (s *Server) SetChargingService(service *charging.Service) { s.chargingSvc = service }

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"code":"invalid_request","message":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	session, err := s.authSvc.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			status = http.StatusInternalServerError
		}
		http.Error(w, `{"code":"authentication_failed","message":"login failed"}`, status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(session)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if len(token) >= len("Bearer ") && token[:len("Bearer ")] == "Bearer " {
		token = token[len("Bearer "):]
	}
	if err := s.authSvc.Logout(r.Context(), token); err != nil {
		http.Error(w, `{"code":"session_invalid","message":"session is not active"}`, http.StatusUnauthorized)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/subscriptions/stream" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, cancel := contextWithTimeout(r.Context(), s.cfg.WriteTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func contextWithTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, d)
}

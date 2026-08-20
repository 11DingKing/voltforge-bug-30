package app

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"voltforge/internal/audit"
	"voltforge/internal/auth"
	"voltforge/internal/charging"
	"voltforge/internal/circuit"
	"voltforge/internal/config"
	"voltforge/internal/domain"
	"voltforge/internal/httpapi"
	"voltforge/internal/ratelimit"
	"voltforge/internal/scheduler"
	"voltforge/internal/service"
	"voltforge/internal/sla"
	"voltforge/internal/storage"
)

type App struct {
	cfg          *config.Config
	logger       *audit.Logger
	store        storage.Store
	eventBus     *service.EventBus
	handSvc      *service.HandshakeService
	dispSvc      *service.MitigationService
	batchSvc     *service.BatchService
	subSvc       *service.SubscriptionService
	telemetrySvc *service.TelemetryService
	importSvc    *service.ImportExportService
	overdueSvc   *service.OverdueService
	maintSvc     *service.MaintenanceService
	authSvc      *auth.Service
	scheduler    *scheduler.Scheduler
	httpServer   *http.Server
	clock        domain.Clock
}

func New(ctx context.Context, cfg *config.Config, clock domain.Clock) (*App, error) {
	if clock == nil {
		clock = domain.RealClock{}
	}
	logger := audit.NewLogger(cfg.LogLevel)
	store, err := storage.NewStore(ctx, cfg.DataDir, clock)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}
	eventBus := service.NewEventBus(store.EventRepo())
	handSvc := service.NewHandshakeService(store, clock, eventBus)
	dispSvc := service.NewMitigationService(store, clock, eventBus).
		WithBreaker(circuit.New(circuit.Config{
			Name: "mitigation_execute", MaxRequests: 3, Timeout: 30 * time.Second,
			FailureThreshold: 5, FailureRatio: 0.6,
		}))
	batchSvc := service.NewBatchService(store, clock, eventBus)
	subSvc := service.NewSubscriptionService(store, clock, eventBus).
		WithLimiter(ratelimit.NewSubscriberLimiter(10, 20, 3))
	telemetrySvc := service.NewTelemetryService(store, clock)
	importSvc := service.NewImportExportService(handSvc, clock)
	overdueSvc := service.NewOverdueService(store, clock, cfg.AttestationTimeout()).
		WithSLA(sla.NewRuleSet(cfg.AttestationTimeoutHours))
	maintSvc := service.NewMaintenanceService(store, clock)
	authSvc := auth.New(8*time.Hour, clock.Now, store)
	sched := scheduler.New(logger, store, clock)
	sched.AddTask(scheduler.TimeoutMonitorTask(store, clock, cfg.AttestationTimeout()))
	sched.AddTask(scheduler.EventPrunerTask(store, clock, cfg.ReplayWindow()))
	sched.AddTask(scheduler.FailureRetryTask(sched, clock))
	sched.AddTask(scheduler.BatchProcessorTask(store, clock))
	return &App{
		cfg: cfg, logger: logger, store: store, eventBus: eventBus,
		handSvc: handSvc, dispSvc: dispSvc, batchSvc: batchSvc,
		subSvc: subSvc, telemetrySvc: telemetrySvc, importSvc: importSvc,
		overdueSvc: overdueSvc, maintSvc: maintSvc, authSvc: authSvc, scheduler: sched, clock: clock,
	}, nil
}

func (a *App) Start(ctx context.Context) error {
	a.scheduler.Start(ctx)
	srv := httpapi.NewServer(
		a.cfg, a.logger, a.store, a.handSvc, a.dispSvc, a.batchSvc,
		a.subSvc, a.telemetrySvc, a.importSvc, a.overdueSvc, a.maintSvc, a.scheduler, a.authSvc,
	)
	srv.SetChargingService(charging.NewService(a.store, a.authSvc, a.clock.Now))
	a.httpServer = &http.Server{
		Addr:         a.cfg.Addr(),
		Handler:      srv.Routes(),
		ReadTimeout:  a.cfg.ReadTimeout,
		WriteTimeout: a.cfg.WriteTimeout,
		IdleTimeout:  a.cfg.IdleTimeout,
	}
	a.logger.Info().Str("addr", a.cfg.Addr()).Msg("HTTP server starting")
	return a.httpServer.ListenAndServe()
}

func (a *App) Shutdown(ctx context.Context) error {
	a.logger.Info().Msg("shutting down")
	a.scheduler.Stop()
	var err error
	if a.httpServer != nil {
		err = a.httpServer.Shutdown(ctx)
	}
	if cerr := a.store.Close(); cerr != nil {
		a.logger.Error().Err(cerr).Msg("store close error")
	}
	a.logger.Info().Msg("shutdown complete")
	return err
}

func (a *App) Store() storage.Store                              { return a.store }
func (a *App) Logger() *audit.Logger                             { return a.logger }
func (a *App) Scheduler() *scheduler.Scheduler                   { return a.scheduler }
func (a *App) HandshakeService() *service.HandshakeService       { return a.handSvc }
func (a *App) MitigationService() *service.MitigationService     { return a.dispSvc }
func (a *App) BatchService() *service.BatchService               { return a.batchSvc }
func (a *App) SubscriptionService() *service.SubscriptionService { return a.subSvc }
func (a *App) TelemetryService() *service.TelemetryService       { return a.telemetrySvc }
func (a *App) ImportExportService() *service.ImportExportService { return a.importSvc }
func (a *App) OverdueService() *service.OverdueService           { return a.overdueSvc }
func (a *App) MaintenanceService() *service.MaintenanceService   { return a.maintSvc }

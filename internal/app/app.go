// Package app wires configuration, storage, HTTP, and lifecycle together.
package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/checker"
	"github.com/mrweez/iwmmd/internal/config"
	"github.com/mrweez/iwmmd/internal/demo"
	"github.com/mrweez/iwmmd/internal/domain"
	"github.com/mrweez/iwmmd/internal/httpapi"
	"github.com/mrweez/iwmmd/internal/jobs"
	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/platform/metrics"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/platform/version"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
)

// App is a fully constructed IWMMD process.
type App struct {
	cfg    *config.Config
	store  *store.Store
	logger *slog.Logger

	domains    *domain.Service
	alerts     *alert.Engine
	channels   *notify.ChannelService
	settings   *settings.Service
	worker     *jobs.Worker
	dispatcher *notify.Dispatcher
	demo       *demo.Seeder

	httpServer     *http.Server
	redirectServer *http.Server
	metricsServer  *http.Server
	tlsReloader    *certReloader
}

// New opens storage, applies migrations, and builds the HTTP servers.
func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	if logger == nil {
		logger = slog.Default()
	}

	st, err := store.Open(store.Options{DataDir: cfg.DataDir, DSN: cfg.DBDSN})
	if err != nil {
		return nil, err
	}
	if err := st.Migrate(); err != nil {
		_ = st.Close()
		return nil, err
	}

	box, err := secrets.New(cfg.MasterKey)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("initialize secret box: %w", err)
	}
	settingsSvc := settings.New(st, box)
	alertEngine := &alert.Engine{Store: st, Settings: settingsSvc, BaseURL: cfg.BaseURL, Logger: logger}

	var lookup domain.Checker = checker.New(cfg.DataDir)
	var demoState *demo.State
	var demoSeeder *demo.Seeder
	if cfg.Demo.Enabled {
		interval, err := time.ParseDuration(cfg.Demo.ResetInterval)
		if err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("parse demo reset interval: %w", err)
		}
		demoState = demo.NewState(cfg.Demo.Role, interval)
		demoSeeder = demo.NewSeeder(st, box, demoState, logger)
		lookup = checker.Demo{}
		if err := demoSeeder.Initialize(context.Background(), time.Now().UTC()); err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("initialize demo data: %w", err)
		}
	}

	domainSvc := &domain.Service{
		Store: st, Checker: lookup, Settings: settingsSvc, Alerts: alertEngine, Logger: logger,
	}
	sender := &notify.Sender{Store: st, Settings: settingsSvc, Box: box, Demo: cfg.Demo.Enabled}
	channelSvc := &notify.ChannelService{Store: st, Box: box, Sender: sender}

	var oidcClient *auth.OIDC
	if !cfg.Demo.Enabled {
		oidcClient, err = auth.NewOIDC(context.Background(), cfg.OIDC)
		if err != nil {
			_ = st.Close()
			return nil, err
		}
	}
	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	sessions := auth.NewSessions(st, box, base.Scheme == "https")

	m := metrics.New(version.Version, version.Commit)
	router := httpapi.NewRouter(httpapi.Deps{
		Config:   cfg,
		Store:    st,
		Metrics:  m,
		Logger:   logger,
		OIDC:     oidcClient,
		Sessions: sessions,
		Domains:  domainSvc,
		Alerts:   alertEngine,
		Channels: channelSvc,
		Settings: settingsSvc,
		Demo:     demoState,
	})

	a := &App{
		cfg: cfg, store: st, logger: logger,
		domains: domainSvc, alerts: alertEngine, channels: channelSvc, settings: settingsSvc,
		worker:     &jobs.Worker{Store: st, Domains: domainSvc, Logger: logger, Concurrency: 4, Interval: 30 * time.Second},
		dispatcher: &notify.Dispatcher{Store: st, Sender: sender, Settings: settingsSvc, Logger: logger, Interval: 5 * time.Second, Batch: 10},
		demo:       demoSeeder,
	}
	a.httpServer = newHTTPServer("", router, logger)
	if cfg.TLS.Enabled {
		reloader, err := newCertReloader(cfg.TLS.CertFile, cfg.TLS.KeyFile, base.Hostname())
		if err != nil {
			_ = st.Close()
			return nil, err
		}
		a.tlsReloader = reloader
		a.httpServer.Addr = fmt.Sprintf(":%d", cfg.TLS.Port)
		a.httpServer.TLSConfig = &tls.Config{
			MinVersion:     tls.VersionTLS12,
			GetCertificate: reloader.GetCertificate,
		}

		if cfg.TLS.HTTPRedirect && cfg.HTTPAddr != "" {
			a.redirectServer = newHTTPServer(cfg.HTTPAddr, redirectToBase(base), logger)
		}
	} else {
		a.httpServer.Addr = cfg.HTTPAddr
	}

	if cfg.Metrics.Addr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", m.Handler())
		a.metricsServer = newHTTPServer(cfg.Metrics.Addr, mux, logger)
	}
	return a, nil
}

// Run starts all listeners and blocks until the context is canceled or a
// listener fails.
func (a *App) Run(ctx context.Context) error {
	defer func() {
		if err := a.Close(); err != nil {
			a.logger.Error("close application", "error", err)
		}
	}()

	runCtx, stop := context.WithCancel(ctx)
	defer stop()

	a.logger.Info("starting iwmmd",
		"version", version.String(),
		"base_url", a.cfg.BaseURL,
		"http_addr", a.httpServer.Addr,
		"tls", a.cfg.TLS.Enabled,
		"metrics_addr", a.cfg.Metrics.Addr,
	)
	if a.demo != nil {
		a.logger.Warn("DEMO MODE ENABLED: anonymous read-write access with disposable mock data",
			"role", a.cfg.Demo.Role, "reset_interval", a.cfg.Demo.ResetInterval)
		go a.demo.Run(runCtx)
	} else if a.cfg.OIDC.Issuer == "" {
		a.logger.Warn("OIDC is not configured; authenticated API access is unavailable")
	}

	go a.worker.Run(runCtx)
	go a.dispatcher.Run(runCtx)
	if a.demo == nil {
		go func() {
			if err := a.alerts.GenerateAll(runCtx, time.Now().UTC()); err != nil && !errors.Is(err, context.Canceled) {
				a.logger.Error("generate initial alerts", "error", err)
			}
		}()
	}

	errCh := make(chan error, 3)
	if a.metricsServer != nil {
		startServer("metrics", a.metricsServer, false, errCh)
	}
	startServer("http", a.httpServer, a.cfg.TLS.Enabled, errCh)
	if a.redirectServer != nil {
		startServer("http-redirect", a.redirectServer, false, errCh)
	}

	if a.tlsReloader != nil {
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		defer signal.Stop(hup)
		go func() {
			for {
				select {
				case <-runCtx.Done():
					return
				case <-hup:
					if err := a.tlsReloader.Reload(); err != nil {
						a.logger.Error("reload TLS certificate", "error", err)
						continue
					}
					a.logger.Info("reloaded TLS certificate")
				}
			}
		}()
	}

	select {
	case <-runCtx.Done():
		a.logger.Info("shutting down")
		return a.shutdown()
	case err := <-errCh:
		a.logger.Error("server stopped unexpectedly", "error", err)
		stop()
		return errors.Join(err, a.shutdown())
	}
}

// Close releases storage resources. Run calls it automatically.
func (a *App) Close() error {
	if a.store == nil {
		return nil
	}
	return a.store.Close()
}

func (a *App) shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var errs []error
	for _, srv := range []*http.Server{a.httpServer, a.redirectServer, a.metricsServer} {
		if srv == nil {
			continue
		}
		if err := srv.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func startServer(name string, srv *http.Server, useTLS bool, errCh chan<- error) {
	go func() {
		var err error
		if useTLS {
			err = srv.ListenAndServeTLS("", "")
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("%s server: %w", name, err)
		}
	}()
}

func newHTTPServer(addr string, handler http.Handler, logger *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
}

func redirectToBase(base *url.URL) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := *base
		target.Path = r.URL.Path
		target.RawPath = ""
		target.RawQuery = r.URL.RawQuery
		http.Redirect(w, r, target.String(), http.StatusPermanentRedirect)
	})
}

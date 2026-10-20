// Package httpapi exposes the IWMMD HTTP API and the embedded web UI.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/config"
	"github.com/mrweez/iwmmd/internal/domain"
	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/platform/metrics"
	"github.com/mrweez/iwmmd/internal/platform/version"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
	"github.com/mrweez/iwmmd/internal/webui"
)

// Deps contains the router dependencies.
type Deps struct {
	Config   *config.Config
	Store    *store.Store
	Metrics  *metrics.Metrics
	Logger   *slog.Logger
	OIDC     *auth.OIDC
	Sessions *auth.Sessions
	Domains  *domain.Service
	Alerts   *alert.Engine
	Channels *notify.ChannelService
	Settings *settings.Service
}

// API holds handlers.
type API struct {
	deps Deps
}

// NewRouter builds the application router.
func NewRouter(deps Deps) http.Handler {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	api := &API{deps: deps}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.CleanPath)
	r.Use(accessLog(logger))
	if deps.Metrics != nil {
		r.Use(deps.Metrics.Middleware)
	}
	r.Use(securityHeaders(deps.Config))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := deps.Store.Ready(ctx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "Not ready", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Use(noStore)
		r.Route("/v1", func(r chi.Router) {
			r.Get("/meta", api.meta)
			r.Get("/me", api.me)
			r.Get("/auth/login", api.login)
			r.Get("/auth/callback", api.callback)

			r.Group(func(r chi.Router) {
				r.Use(api.authenticate)
				r.Use(api.csrf)
				r.Post("/auth/logout", api.logout)

				r.Group(func(r chi.Router) {
					r.Use(api.requireRole("viewer"))
					r.Get("/stats", api.stats)
					r.Get("/domains", api.listDomains)
					r.Get("/domains/{domainID}", api.getDomain)
					r.Get("/domains/{domainID}/checks", api.listChecks)
					r.Get("/domains/{domainID}/checks/{checkID}/raw", api.rawCheck)
					r.Get("/domains/{domainID}/notifications", api.domainNotifications)
					r.Get("/domains/{domainID}/incidents", api.domainIncidents)
					r.Get("/alert-rules", api.listRules)
					r.Get("/channels", api.listChannels)
					r.Get("/settings", api.getSettings)
					r.Get("/incidents", api.listIncidents)
					r.Get("/notifications", api.listNotifications)
				})

				r.Group(func(r chi.Router) {
					r.Use(api.requireRole("member"))
					r.Post("/domains", api.createDomain)
					r.Post("/domains/import", api.importDomains)
					r.Patch("/domains/{domainID}", api.updateDomain)
					r.Delete("/domains/{domainID}", api.archiveDomain)
					r.Post("/domains/{domainID}/restore", api.restoreDomain)
					r.Post("/domains/{domainID}/check", api.checkDomain)
					r.Post("/domains/{domainID}/mute", api.muteDomain)
					r.Post("/domains/{domainID}/unmute", api.unmuteDomain)
					r.Post("/alert-rules", api.createRule)
					r.Patch("/alert-rules/{ruleID}", api.updateRule)
					r.Delete("/alert-rules/{ruleID}", api.archiveRule)
				})

				r.Group(func(r chi.Router) {
					r.Use(api.requireRole("admin"))
					r.Post("/channels", api.createChannel)
					r.Patch("/channels/{channelID}", api.updateChannel)
					r.Delete("/channels/{channelID}", api.archiveChannel)
					r.Post("/channels/{channelID}/test", api.testChannel)
					r.Post("/channels/{channelID}/mute", api.muteChannel)
					r.Post("/channels/{channelID}/unmute", api.unmuteChannel)
					r.Put("/settings/smtp", api.saveSMTP)
					r.Post("/settings/smtp/test", api.testSMTP)
					r.Put("/settings/alerts", api.saveAlerts)
					r.Put("/settings/retention", api.saveRetention)
					r.Put("/settings/check", api.saveCheck)
					r.Get("/users", api.listUsers)
					r.Patch("/users/{userID}/role", api.setUserRole)
					r.Delete("/users/{userID}/sessions", api.revokeUserSessions)
					r.Get("/audit", api.listAudit)
					r.Get("/jobs", api.listJobs)
					r.Delete("/domains/{domainID}/permanent", api.deleteDomainPermanent)
					r.Post("/notifications/{notificationID}/retry", api.retryNotification)
				})
			})
		})
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeProblem(w, http.StatusNotFound, "Not found", "The requested API endpoint does not exist.")
		})
	})

	r.Handle("/*", webui.Handler())
	return r
}

func (a *API) meta(w http.ResponseWriter, r *http.Request) {
	provider := a.deps.Config.OIDC.Provider
	issuer := a.deps.Config.OIDC.Issuer
	writeJSON(w, http.StatusOK, map[string]any{
		"version":          version.Version,
		"commit":           version.Commit,
		"date":             version.Date,
		"auth_configured":  a.deps.OIDC != nil,
		"provider":         provider,
		"issuer":           issuer,
		"tls_enabled":      a.deps.Config.TLS.Enabled,
		"provider_presets": auth.Presets(),
	})
}

func securityHeaders(cfg *config.Config) func(http.Handler) http.Handler {
	hsts := ""
	if cfg.TLS.Enabled {
		maxAge := "31536000"
		if cfg.TLS.HSTSMaxAge != "" {
			if d, err := time.ParseDuration(cfg.TLS.HSTSMaxAge); err == nil {
				maxAge = strconv.FormatInt(int64(d.Seconds()), 10)
			}
		}
		hsts = "max-age=" + maxAge
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			if hsts != "" {
				w.Header().Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &logRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			logger.Info("http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

type logRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *logRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

func (r *logRecorder) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *logRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

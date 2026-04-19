package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aykutsp/ntp-server/internal/config"
	"github.com/aykutsp/ntp-server/internal/metrics"
	"github.com/aykutsp/ntp-server/internal/ntp"
)

type RuntimeProvider interface {
	Status() ntp.Status
	Config() config.Config
}

type Server struct {
	cfg     config.APIConfig
	logger  *slog.Logger
	metrics *metrics.Counters
	runtime RuntimeProvider
	http    *http.Server
}

func New(cfg config.APIConfig, logger *slog.Logger, m *metrics.Counters, runtime RuntimeProvider) *Server {
	s := &Server{
		cfg:     cfg,
		logger:  logger,
		metrics: m,
		runtime: runtime,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/v1/status", s.authRequired(s.handleStatus))
	mux.HandleFunc("/v1/stats", s.authRequired(s.handleStats))
	mux.HandleFunc("/v1/config", s.authRequired(s.handleConfig))

	s.http = &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           mux,
		ReadTimeout:       time.Duration(cfg.ReadTimeoutMillis) * time.Millisecond,
		WriteTimeout:      time.Duration(cfg.WriteTimeoutMillis) * time.Millisecond,
		ReadHeaderTimeout: 2 * time.Second,
	}

	return s
}

func (s *Server) Start() {
	go func() {
		s.logger.Info("management API started", "listen", s.cfg.ListenAddress)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("management API failed", "error", err.Error())
		}
	}()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

func (s *Server) authRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.cfg.AuthToken) == "" {
			next(w, r)
			return
		}
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if auth != "Bearer "+s.cfg.AuthToken {
			writeJSON(w, http.StatusUnauthorized, map[string]any{
				"error": "unauthorized",
			})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	st := s.runtime.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"synced": st.Synced,
		"time":   st.NowUTC,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.runtime.Status())
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.metrics.Snapshot())
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	if !s.cfg.ExposeConfig {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error": "config endpoint disabled",
		})
		return
	}
	cfg := s.runtime.Config()
	if cfg.API.AuthToken != "" {
		cfg.API.AuthToken = "***"
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(s.metrics.Prometheus()))
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

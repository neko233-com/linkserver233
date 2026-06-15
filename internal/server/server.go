package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neko233-com/linkserver233/internal/config"
	"github.com/neko233-com/linkserver233/internal/ratelimit"
	"github.com/neko233-com/linkserver233/internal/store"
)

const (
	apiPrefix       = "/api/v1/links/"
	maxRequestBytes = 1 << 20
)

// Server wires configuration, storage, rate limiting, and HTTP routing.
type Server struct {
	cfg     config.ServeConfig
	store   store.Store
	logger  *slog.Logger
	limiter *ratelimit.Limiter
	now     func() time.Time
	handler http.Handler
}

// Option customizes a Server, primarily for tests.
type Option func(*Server)

// WithClock overrides the time source used for expiry and status calculations.
func WithClock(now func() time.Time) Option {
	return func(s *Server) {
		if now != nil {
			s.now = now
		}
	}
}

// WithRateLimiter injects a pre-built limiter.
func WithRateLimiter(limiter *ratelimit.Limiter) Option {
	return func(s *Server) {
		s.limiter = limiter
	}
}

// New builds a ready-to-serve Server.
func New(cfg config.ServeConfig, linkStore store.Store, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	s := &Server{
		cfg:    cfg,
		store:  linkStore,
		logger: logger,
		now:    func() time.Time { return time.Now() },
	}

	for _, opt := range opts {
		opt(s)
	}

	if s.limiter == nil {
		s.limiter = ratelimit.New(cfg.RateLimitPerMinute/60.0, cfg.RateLimitBurst, s.now)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/agent", s.handleAgentGuide)
	mux.HandleFunc("/llms.txt", s.handleLLMSText)
	mux.HandleFunc("/api/v1/links", s.handleLinksCollection)
	mux.HandleFunc(apiPrefix, s.handleLinkItem)
	mux.HandleFunc("/api/v1/stats", s.handleStats)
	mux.HandleFunc("/api/v1/import", s.handleImport)
	mux.HandleFunc("/", s.handleRedirectOrHome)

	s.handler = s.withLogging(s.withRateLimit(mux))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// StartJanitor launches a background goroutine that purges expired links until
// ctx is cancelled.
func (s *Server) StartJanitor(ctx context.Context) {
	if s.cfg.JanitorInterval <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(s.cfg.JanitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				removed, err := s.store.DeleteExpired(s.now().UTC())
				if err != nil {
					s.logger.Error("janitor purge failed", "error", err)
					continue
				}
				if removed > 0 {
					s.logger.Info("purged expired links", "count", removed)
				}
			}
		}
	}()
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.limiter.Enabled() && !s.limiter.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, errors.New("rate limit exceeded"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		s.logger.Info("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.statusCode,
			"duration", time.Since(startedAt),
		)
	})
}

func (s *Server) publicURL(r *http.Request, pathValue string) string {
	baseURL := s.cfg.BaseURL
	if baseURL == "" {
		scheme := "http"
		if forwardedProto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwardedProto != "" {
			scheme = forwardedProto
		} else if r.TLS != nil {
			scheme = "https"
		}
		baseURL = scheme + "://" + r.Host
	}

	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(pathValue, "/")
}

func (s *Server) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, err)
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, store.ErrExpired), errors.Is(err, store.ErrExhausted):
		writeError(w, http.StatusGone, err)
	case errors.Is(err, store.ErrDisabled):
		writeError(w, http.StatusNotFound, errors.New("link not found"))
	default:
		s.logger.Error("store operation failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("store operation failed"))
	}
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		first := strings.TrimSpace(strings.Split(forwarded, ",")[0])
		if first != "" {
			return first
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func mergeTargetQuery(targetURL, rawQuery string) (string, error) {
	if rawQuery == "" {
		return targetURL, nil
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		return "", err
	}
	if parsed.RawQuery == "" {
		parsed.RawQuery = rawQuery
	} else {
		parsed.RawQuery = parsed.RawQuery + "&" + rawQuery
	}
	return parsed.String(), nil
}

func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decodeJSON(body io.ReadCloser, dst any) error {
	defer body.Close()

	reader := io.LimitReader(body, maxRequestBytes)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode json: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": err.Error()})
}

func writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

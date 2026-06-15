package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/linkserver233/internal/agentdocs"
	"github.com/neko233-com/linkserver233/internal/buildinfo"
	"github.com/neko233-com/linkserver233/internal/link"
	"github.com/neko233-com/linkserver233/internal/security"
	"github.com/neko233-com/linkserver233/internal/store"
)

const (
	defaultListLimit = 50
	maxListLimit     = 500
)

type createLinkRequest struct {
	Path           string     `json:"path"`
	TargetURL      string     `json:"target_url"`
	Description    string     `json:"description"`
	RedirectStatus int        `json:"redirect_status"`
	Tags           []string   `json:"tags"`
	Password       string     `json:"password"`
	MaxClicks      uint64     `json:"max_clicks"`
	ExpiresIn      string     `json:"expires_in"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Enabled        *bool      `json:"enabled"`
}

type updateLinkRequest struct {
	TargetURL      string     `json:"target_url"`
	Description    string     `json:"description"`
	RedirectStatus int        `json:"redirect_status"`
	Tags           []string   `json:"tags"`
	Password       *string    `json:"password"`
	MaxClicks      *uint64    `json:"max_clicks"`
	ExpiresIn      string     `json:"expires_in"`
	ExpiresAt      *time.Time `json:"expires_at"`
	ClearExpiry    bool       `json:"clear_expiry"`
	Enabled        *bool      `json:"enabled"`
}

type linkResponse struct {
	Path            string      `json:"path"`
	ShortURL        string      `json:"short_url"`
	TargetURL       string      `json:"target_url"`
	Description     string      `json:"description,omitempty"`
	RedirectStatus  int         `json:"redirect_status"`
	Tags            []string    `json:"tags,omitempty"`
	Status          link.Status `json:"status"`
	Enabled         bool        `json:"enabled"`
	HasPassword     bool        `json:"has_password"`
	MaxClicks       uint64      `json:"max_clicks"`
	RemainingClicks *uint64     `json:"remaining_clicks,omitempty"`
	ExpiresAt       *time.Time  `json:"expires_at,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	Clicks          uint64      `json:"clicks"`
	LastVisitedAt   *time.Time  `json:"last_visited_at,omitempty"`
}

type linkListResponse struct {
	Items  []linkResponse `json:"items"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

type statsResponse struct {
	TotalLinks        int            `json:"total_links"`
	TotalClicks       uint64         `json:"total_clicks"`
	Active            int            `json:"active"`
	Expired           int            `json:"expired"`
	Disabled          int            `json:"disabled"`
	Exhausted         int            `json:"exhausted"`
	PasswordProtected int            `json:"password_protected"`
	TopLinks          []linkResponse `json:"top_links"`
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": buildinfo.Version,
	})
}

func (s *Server) handleAgentGuide(w http.ResponseWriter, _ *http.Request) {
	writeText(w, http.StatusOK, agentdocs.AgentGuide)
}

func (s *Server) handleLLMSText(w http.ResponseWriter, _ *http.Request) {
	writeText(w, http.StatusOK, agentdocs.LLMSText)
}

func (s *Server) handleLinksCollection(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.listLinks(w, r)
	case http.MethodPost:
		s.createLink(w, r)
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleLinkItem(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}

	pathValue, err := s.pathFromAPIRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	switch r.Method {
	case http.MethodGet:
		record, err := s.store.Get(pathValue)
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.makeResponse(r, record))
	case http.MethodPut:
		s.updateLink(w, r, pathValue)
	case http.MethodDelete:
		if err := s.store.Delete(pathValue); err != nil {
			s.writeStoreError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, http.MethodGet, http.MethodPut, http.MethodDelete)
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, http.MethodGet)
		return
	}

	records, err := s.store.List()
	if err != nil {
		s.logger.Error("stats list failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("failed to compute stats"))
		return
	}

	now := s.now().UTC()
	stats := statsResponse{TotalLinks: len(records)}
	for _, record := range records {
		stats.TotalClicks += record.Clicks
		if record.HasPassword() {
			stats.PasswordProtected++
		}
		switch record.Status(now) {
		case link.StatusActive:
			stats.Active++
		case link.StatusExpired:
			stats.Expired++
		case link.StatusDisabled:
			stats.Disabled++
		case link.StatusExhausted:
			stats.Exhausted++
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Clicks > records[j].Clicks
	})
	limit := min(len(records), 5)
	for _, record := range records[:limit] {
		stats.TopLinks = append(stats.TopLinks, s.makeResponse(r, record))
	}

	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleRedirectOrHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		s.handleHome(w)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
		return
	}

	pathValue, err := link.NormalizeLookupPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	record, err := s.store.Get(pathValue)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	now := s.now().UTC()
	switch record.Status(now) {
	case link.StatusDisabled:
		http.NotFound(w, r)
		return
	case link.StatusExpired:
		writeError(w, http.StatusGone, errors.New("link has expired"))
		return
	case link.StatusExhausted:
		writeError(w, http.StatusGone, errors.New("link click limit reached"))
		return
	}

	if record.HasPassword() && !s.passwordSatisfied(w, r, record) {
		return
	}

	updated, err := s.store.RegisterVisit(pathValue)
	if err != nil {
		s.writeRedirectStoreError(w, r, err)
		return
	}

	target, err := mergeTargetQuery(updated.TargetURL, redirectQuery(r.URL.Query()))
	if err != nil {
		s.logger.Error("build redirect target failed", "path", pathValue, "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("failed to build redirect target"))
		return
	}

	http.Redirect(w, r, target, updated.RedirectStatus)
}

func (s *Server) handleHome(w http.ResponseWriter) {
	var builder strings.Builder
	fmt.Fprintf(&builder, "linkserver233 %s\n\n", buildinfo.Version)
	builder.WriteString("POST /api/v1/links       create a short or custom link\n")
	builder.WriteString("GET  /api/v1/links       list and filter links\n")
	builder.WriteString("GET  /api/v1/links/{p}   show one link\n")
	builder.WriteString("PUT  /api/v1/links/{p}   update one link\n")
	builder.WriteString("DEL  /api/v1/links/{p}   delete one link\n")
	builder.WriteString("GET  /api/v1/stats       aggregate statistics\n")
	builder.WriteString("POST /api/v1/import      bulk import links\n")
	builder.WriteString("GET  /healthz            health check\n")
	builder.WriteString("GET  /agent              agent usage guide\n")
	builder.WriteString("GET  /llms.txt           machine-readable index\n\n")
	builder.WriteString("Example:\n")
	builder.WriteString(`curl -X POST http://127.0.0.1:8080/api/v1/links -H "Content-Type: application/json" -d '{"target_url":"https://example.com/very/long/url"}'` + "\n")
	writeText(w, http.StatusOK, builder.String())
}

func (s *Server) listLinks(w http.ResponseWriter, r *http.Request) {
	records, err := s.store.List()
	if err != nil {
		s.logger.Error("list links failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("failed to list links"))
		return
	}

	query := r.URL.Query()
	statusFilter := link.Status(strings.ToLower(strings.TrimSpace(query.Get("status"))))
	tagFilter := strings.TrimSpace(query.Get("tag"))
	search := strings.ToLower(strings.TrimSpace(query.Get("q")))
	now := s.now().UTC()

	filtered := make([]link.Record, 0, len(records))
	for _, record := range records {
		if statusFilter != "" && record.Status(now) != statusFilter {
			continue
		}
		if tagFilter != "" && !containsTag(record.Tags, tagFilter) {
			continue
		}
		if search != "" && !matchesSearch(record, search) {
			continue
		}
		filtered = append(filtered, record)
	}

	limit := parseIntDefault(query.Get("limit"), defaultListLimit)
	if limit < 1 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	offset := parseIntDefault(query.Get("offset"), 0)
	if offset < 0 {
		offset = 0
	}

	total := len(filtered)
	start := min(offset, total)
	end := min(start+limit, total)
	page := filtered[start:end]

	items := make([]linkResponse, 0, len(page))
	for _, record := range page {
		items = append(items, s.makeResponse(r, record))
	}

	writeJSON(w, http.StatusOK, linkListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	})
}

func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var request createLinkRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	targetURL, err := link.NormalizeTargetURL(request.TargetURL, s.cfg.AllowPrivateTargets)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	redirectStatus, err := link.NormalizeRedirectStatus(request.RedirectStatus)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	now := s.now().UTC()
	expiresAt, err := s.resolveExpiry(now, request.ExpiresAt, request.ExpiresIn, nil, false, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	passwordHash, err := hashOptionalPassword(request.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}

	record := link.Record{
		TargetURL:      targetURL,
		Description:    strings.TrimSpace(request.Description),
		RedirectStatus: redirectStatus,
		Tags:           normalizeTags(request.Tags),
		Enabled:        enabled,
		PasswordHash:   passwordHash,
		MaxClicks:      request.MaxClicks,
		ExpiresAt:      expiresAt,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	customPath := strings.TrimSpace(request.Path)
	if customPath != "" {
		normalized, err := link.NormalizePath(customPath)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		record.Path = normalized
		created, err := s.store.Create(record)
		if err != nil {
			s.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, s.makeResponse(r, created))
		return
	}

	created, err := s.createGenerated(record)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.makeResponse(r, created))
}

func (s *Server) createGenerated(base link.Record) (link.Record, error) {
	for range 16 {
		code, err := link.GenerateShortCode(s.cfg.ShortCodeLength)
		if err != nil {
			return link.Record{}, err
		}
		base.Path = code
		created, err := s.store.Create(base)
		if err == nil {
			return created, nil
		}
		if !errors.Is(err, store.ErrConflict) {
			return link.Record{}, err
		}
	}
	return link.Record{}, errors.New("failed to allocate a unique short code")
}

func (s *Server) updateLink(w http.ResponseWriter, r *http.Request, pathValue string) {
	current, err := s.store.Get(pathValue)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}

	var request updateLinkRequest
	if err := decodeJSON(r.Body, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	targetURL, err := link.NormalizeTargetURL(request.TargetURL, s.cfg.AllowPrivateTargets)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	redirectStatus, err := link.NormalizeRedirectStatus(request.RedirectStatus)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	now := s.now().UTC()
	expiresAt, err := s.resolveExpiry(now, request.ExpiresAt, request.ExpiresIn, current.ExpiresAt, request.ClearExpiry, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	current.TargetURL = targetURL
	current.Description = strings.TrimSpace(request.Description)
	current.RedirectStatus = redirectStatus
	current.Tags = normalizeTags(request.Tags)
	current.ExpiresAt = expiresAt
	current.UpdatedAt = now

	if request.MaxClicks != nil {
		current.MaxClicks = *request.MaxClicks
	}
	if request.Enabled != nil {
		current.Enabled = *request.Enabled
	}
	if request.Password != nil {
		hash, err := hashOptionalPassword(*request.Password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		current.PasswordHash = hash
	}

	updated, err := s.store.Update(current)
	if err != nil {
		s.writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, s.makeResponse(r, updated))
}

// resolveExpiry computes the effective expiry honoring default/max TTL and the
// require-expiry policy. applyDefault is true only for creation.
func (s *Server) resolveExpiry(now time.Time, at *time.Time, in string, current *time.Time, clear, applyDefault bool) (*time.Time, error) {
	var candidate *time.Time
	switch {
	case clear:
		candidate = nil
	case at != nil:
		value := at.UTC()
		candidate = &value
	case strings.TrimSpace(in) != "":
		duration, err := link.ParseFlexibleDuration(in)
		if err != nil {
			return nil, fmt.Errorf("invalid expires_in: %w", err)
		}
		value := now.Add(duration)
		candidate = &value
	case current != nil:
		candidate = current
	case applyDefault && s.cfg.DefaultTTL > 0:
		value := now.Add(s.cfg.DefaultTTL)
		candidate = &value
	case applyDefault && s.cfg.MaxTTL > 0:
		value := now.Add(s.cfg.MaxTTL)
		candidate = &value
	}

	if candidate != nil {
		if !candidate.After(now) {
			return nil, errors.New("expiry must be in the future")
		}
		if s.cfg.MaxTTL > 0 && candidate.After(now.Add(s.cfg.MaxTTL).Add(time.Second)) {
			return nil, fmt.Errorf("expiry exceeds max-ttl of %s", s.cfg.MaxTTL)
		}
	}
	if candidate == nil && s.cfg.RequireExpiry {
		return nil, errors.New("an expiry is required by server policy")
	}

	return candidate, nil
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.AdminToken == "" {
		return true
	}

	token := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
	if token == "" {
		authorization := strings.TrimSpace(r.Header.Get("Authorization"))
		if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
			token = strings.TrimSpace(authorization[7:])
		}
	}

	if !security.ConstantTimeEqual(token, s.cfg.AdminToken) {
		writeError(w, http.StatusUnauthorized, errors.New("missing or invalid admin token"))
		return false
	}

	return true
}

func (s *Server) passwordSatisfied(w http.ResponseWriter, r *http.Request, record link.Record) bool {
	provided := firstNonEmpty(
		r.URL.Query().Get("pw"),
		r.URL.Query().Get("password"),
		r.Header.Get("X-Link-Password"),
	)

	if provided != "" {
		ok, err := security.VerifyPassword(record.PasswordHash, provided)
		if err != nil {
			s.logger.Error("verify link password failed", "path", record.Path, "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("failed to verify password"))
			return false
		}
		if ok {
			return true
		}
	}

	if wantsHTML(r) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(passwordFormHTML(r.URL.Path, provided != "")))
		return false
	}

	writeError(w, http.StatusUnauthorized, errors.New("password required"))
	return false
}

func (s *Server) writeRedirectStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrDisabled):
		http.NotFound(w, r)
	case errors.Is(err, store.ErrExpired):
		writeError(w, http.StatusGone, errors.New("link has expired"))
	case errors.Is(err, store.ErrExhausted):
		writeError(w, http.StatusGone, errors.New("link click limit reached"))
	default:
		s.logger.Error("register visit failed", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("failed to resolve link"))
	}
}

func (s *Server) pathFromAPIRequest(r *http.Request) (string, error) {
	rawPath := strings.TrimPrefix(r.URL.Path, apiPrefix)
	rawPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", fmt.Errorf("decode path: %w", err)
	}
	return link.NormalizeLookupPath(rawPath)
}

func (s *Server) makeResponse(r *http.Request, record link.Record) linkResponse {
	now := s.now().UTC()
	return linkResponse{
		Path:            record.Path,
		ShortURL:        s.publicURL(r, record.Path),
		TargetURL:       record.TargetURL,
		Description:     record.Description,
		RedirectStatus:  record.RedirectStatus,
		Tags:            record.Tags,
		Status:          record.Status(now),
		Enabled:         record.Enabled,
		HasPassword:     record.HasPassword(),
		MaxClicks:       record.MaxClicks,
		RemainingClicks: record.RemainingClicks(),
		ExpiresAt:       record.ExpiresAt,
		CreatedAt:       record.CreatedAt,
		UpdatedAt:       record.UpdatedAt,
		Clicks:          record.Clicks,
		LastVisitedAt:   record.LastVisitedAt,
	}
}

func redirectQuery(values url.Values) string {
	values.Del("pw")
	values.Del("password")
	return values.Encode()
}

func hashOptionalPassword(password string) (string, error) {
	if password == "" {
		return "", nil
	}
	return security.HashPassword(password)
}

func containsTag(tags []string, want string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, want) {
			return true
		}
	}
	return false
}

func matchesSearch(record link.Record, needle string) bool {
	return strings.Contains(strings.ToLower(record.Path), needle) ||
		strings.Contains(strings.ToLower(record.TargetURL), needle) ||
		strings.Contains(strings.ToLower(record.Description), needle)
}

func parseIntDefault(raw string, fallback int) int {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func passwordFormHTML(path string, retry bool) string {
	message := ""
	if retry {
		message = `<p style="color:#c0392b">Incorrect password, try again.</p>`
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Protected link</title>
<style>
body{font-family:system-ui,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
form{background:#1e293b;padding:2rem;border-radius:12px;box-shadow:0 10px 30px rgba(0,0,0,.4);max-width:360px;width:90%%}
h1{font-size:1.1rem;margin:0 0 1rem}
input{width:100%%;padding:.6rem;border-radius:8px;border:1px solid #334155;background:#0f172a;color:#e2e8f0;box-sizing:border-box}
button{margin-top:1rem;width:100%%;padding:.6rem;border:0;border-radius:8px;background:#38bdf8;color:#08111e;font-weight:600;cursor:pointer}
</style>
</head>
<body>
<form method="get" action="%s">
<h1>This link is password protected</h1>
%s
<input type="password" name="pw" placeholder="Enter password" autofocus required>
<button type="submit">Open link</button>
</form>
</body>
</html>`, htmlAttr(path), message)
}

func htmlAttr(value string) string {
	replacer := strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")
	return replacer.Replace(value)
}

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/linkserver233/internal/config"
	"github.com/neko233-com/linkserver233/internal/ratelimit"
	"github.com/neko233-com/linkserver233/internal/store"
)

type testClock struct{ t time.Time }

func (c *testClock) now() time.Time { return c.t }

func baseConfig() config.ServeConfig {
	return config.ServeConfig{
		BaseURL:         "https://go.example.com",
		ShortCodeLength: 7,
	}
}

func newServer(t *testing.T, cfg config.ServeConfig, clock *testClock, opts ...Option) (*Server, store.Store) {
	t.Helper()

	fs, err := store.NewFileStore(filepath.Join(t.TempDir(), "links.json"))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	allOpts := append([]Option{WithClock(clock.now)}, opts...)
	return New(cfg, fs, nil, allOpts...), fs
}

func doRequest(t *testing.T, srv http.Handler, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func decodeLink(t *testing.T, rec *httptest.ResponseRecorder) linkResponse {
	t.Helper()
	var response linkResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode link response: %v", err)
	}
	return response
}

func TestCreateAndRedirectLink(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, fs := newServer(t, baseConfig(), clock)

	body := `{"path":"docs/latest","target_url":"https://example.com/guide?src=docs","redirect_status":307,"description":"docs","tags":["docs","help"]}`
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	created := decodeLink(t, rec)
	if created.Path != "docs/latest" {
		t.Fatalf("unexpected path %q", created.Path)
	}
	if created.ShortURL != "https://go.example.com/docs/latest" {
		t.Fatalf("unexpected short url %q", created.ShortURL)
	}
	if created.Status != "active" {
		t.Fatalf("expected active status, got %q", created.Status)
	}

	redirect := doRequest(t, srv, http.MethodGet, "/docs/latest?lang=zh", "", nil)
	if redirect.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected 307, got %d", redirect.Code)
	}
	if location := redirect.Header().Get("Location"); location != "https://example.com/guide?src=docs&lang=zh" {
		t.Fatalf("unexpected location %q", location)
	}

	record, err := fs.Get("docs/latest")
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if record.Clicks != 1 {
		t.Fatalf("expected 1 click, got %d", record.Clicks)
	}
}

func TestCreateGeneratedShortLink(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	cfg := baseConfig()
	cfg.ShortCodeLength = 6
	srv, _ := newServer(t, cfg, clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"target_url":"https://example.com/some/really/long/path"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	created := decodeLink(t, rec)
	if len(created.Path) != 6 {
		t.Fatalf("expected generated path length 6, got %q", created.Path)
	}
}

func TestCreateRejectsInternalTarget(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"target_url":"http://169.254.169.254/latest/meta-data"}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for SSRF target, got %d", rec.Code)
	}
}

func TestExpiringLinkReturnsGone(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"promo","target_url":"https://example.com/promo","expires_in":"1h"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	ok := doRequest(t, srv, http.MethodGet, "/promo", "", nil)
	if ok.Code != http.StatusFound {
		t.Fatalf("expected 302 before expiry, got %d", ok.Code)
	}

	clock.t = clock.t.Add(2 * time.Hour)
	gone := doRequest(t, srv, http.MethodGet, "/promo", "", nil)
	if gone.Code != http.StatusGone {
		t.Fatalf("expected 410 after expiry, got %d", gone.Code)
	}
}

func TestOneTimeLinkExhausts(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"once","target_url":"https://example.com/once","max_clicks":1}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	first := doRequest(t, srv, http.MethodGet, "/once", "", nil)
	if first.Code != http.StatusFound {
		t.Fatalf("expected 302 on first visit, got %d", first.Code)
	}
	second := doRequest(t, srv, http.MethodGet, "/once", "", nil)
	if second.Code != http.StatusGone {
		t.Fatalf("expected 410 on second visit, got %d", second.Code)
	}
}

func TestPasswordProtectedLink(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"secret","target_url":"https://example.com/secret","password":"s3cret"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeLink(t, rec)
	if !created.HasPassword {
		t.Fatal("expected has_password true")
	}

	missing := doRequest(t, srv, http.MethodGet, "/secret", "", nil)
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without password, got %d", missing.Code)
	}

	wrong := doRequest(t, srv, http.MethodGet, "/secret?pw=nope", "", nil)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong password, got %d", wrong.Code)
	}

	ok := doRequest(t, srv, http.MethodGet, "/secret?pw=s3cret&utm=email", "", nil)
	if ok.Code != http.StatusFound {
		t.Fatalf("expected 302 with correct password, got %d", ok.Code)
	}
	location := ok.Header().Get("Location")
	if strings.Contains(location, "pw=") || strings.Contains(location, "s3cret") {
		t.Fatalf("password leaked into redirect target: %q", location)
	}
	if !strings.Contains(location, "utm=email") {
		t.Fatalf("expected utm passthrough, got %q", location)
	}
}

func TestDisabledLinkNotFound(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	rec := doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"hidden","target_url":"https://example.com/hidden","enabled":false}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}

	visit := doRequest(t, srv, http.MethodGet, "/hidden", "", nil)
	if visit.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled link, got %d", visit.Code)
	}
}

func TestAdminTokenRequired(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	cfg := baseConfig()
	cfg.AdminToken = "topsecret"
	srv, _ := newServer(t, cfg, clock)

	unauth := doRequest(t, srv, http.MethodGet, "/api/v1/links", "", nil)
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", unauth.Code)
	}

	auth := doRequest(t, srv, http.MethodGet, "/api/v1/links", "", map[string]string{
		"Authorization": "Bearer topsecret",
	})
	if auth.Code != http.StatusOK {
		t.Fatalf("expected 200 with token, got %d", auth.Code)
	}
}

func TestListFilteringAndPagination(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"a","target_url":"https://example.com/a","tags":["docs"]}`, nil)
	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"b","target_url":"https://example.com/b","tags":["blog"]}`, nil)
	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"c","target_url":"https://example.com/c","tags":["docs"]}`, nil)

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/links?tag=docs", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var list linkListResponse
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Total != 2 {
		t.Fatalf("expected 2 docs links, got %d", list.Total)
	}

	paged := doRequest(t, srv, http.MethodGet, "/api/v1/links?limit=1&offset=1", "", nil)
	var pagedList linkListResponse
	if err := json.NewDecoder(paged.Body).Decode(&pagedList); err != nil {
		t.Fatalf("decode paged list: %v", err)
	}
	if pagedList.Total != 3 || len(pagedList.Items) != 1 {
		t.Fatalf("expected total 3 with 1 item, got total %d items %d", pagedList.Total, len(pagedList.Items))
	}
}

func TestStatsEndpoint(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"a","target_url":"https://example.com/a"}`, nil)
	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"b","target_url":"https://example.com/b","enabled":false}`, nil)
	doRequest(t, srv, http.MethodGet, "/a", "", nil)

	rec := doRequest(t, srv, http.MethodGet, "/api/v1/stats", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats: %d", rec.Code)
	}
	var stats statsResponse
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.TotalLinks != 2 {
		t.Fatalf("expected 2 links, got %d", stats.TotalLinks)
	}
	if stats.Active != 1 || stats.Disabled != 1 {
		t.Fatalf("unexpected status counts: active=%d disabled=%d", stats.Active, stats.Disabled)
	}
	if stats.TotalClicks != 1 {
		t.Fatalf("expected 1 total click, got %d", stats.TotalClicks)
	}
}

func TestImportReplaceAndCreate(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	body := `{"replace":true,"links":[{"path":"x","target_url":"https://example.com/x"},{"path":"y","target_url":"https://example.com/y","password":"p"}]}`
	rec := doRequest(t, srv, http.MethodPost, "/api/v1/import", body, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
	var imported importResponse
	if err := json.NewDecoder(rec.Body).Decode(&imported); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if imported.Imported != 2 || !imported.Replaced {
		t.Fatalf("unexpected import result: %+v", imported)
	}

	dup := doRequest(t, srv, http.MethodPost, "/api/v1/import", `{"links":[{"path":"x","target_url":"https://example.com/x"}]}`, nil)
	var dupResult importResponse
	if err := json.NewDecoder(dup.Body).Decode(&dupResult); err != nil {
		t.Fatalf("decode dup import: %v", err)
	}
	if dupResult.Imported != 0 || len(dupResult.Skipped) != 1 {
		t.Fatalf("expected 1 skipped, got %+v", dupResult)
	}

	visit := doRequest(t, srv, http.MethodGet, "/y?pw=p", "", nil)
	if visit.Code != http.StatusFound {
		t.Fatalf("expected imported password link to work, got %d", visit.Code)
	}
}

func TestRateLimitReturns429(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	fixed := time.Unix(0, 0)
	limiter := ratelimit.New(1, 2, func() time.Time { return fixed })
	srv, _ := newServer(t, baseConfig(), clock, WithRateLimiter(limiter))

	if got := doRequest(t, srv, http.MethodGet, "/healthz", "", nil).Code; got != http.StatusOK {
		t.Fatalf("first request: %d", got)
	}
	if got := doRequest(t, srv, http.MethodGet, "/healthz", "", nil).Code; got != http.StatusOK {
		t.Fatalf("second request: %d", got)
	}
	limited := doRequest(t, srv, http.MethodGet, "/healthz", "", nil)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 on third request, got %d", limited.Code)
	}
}

func TestAgentAndLLMSEndpoints(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	agent := doRequest(t, srv, http.MethodGet, "/agent", "", nil)
	if agent.Code != http.StatusOK || !strings.Contains(agent.Body.String(), "Agent Guide") {
		t.Fatalf("unexpected agent response: %d", agent.Code)
	}

	llms := doRequest(t, srv, http.MethodGet, "/llms.txt", "", nil)
	if llms.Code != http.StatusOK || !strings.Contains(llms.Body.String(), "linkserver233") {
		t.Fatalf("unexpected llms response: %d", llms.Code)
	}
}

func TestUpdateLink(t *testing.T) {
	clock := &testClock{t: time.Now().UTC()}
	srv, _ := newServer(t, baseConfig(), clock)

	doRequest(t, srv, http.MethodPost, "/api/v1/links", `{"path":"edit","target_url":"https://example.com/old"}`, nil)

	update := `{"target_url":"https://example.com/new","redirect_status":308,"enabled":false}`
	rec := doRequest(t, srv, http.MethodPut, "/api/v1/links/edit", update, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	updated := decodeLink(t, rec)
	if updated.TargetURL != "https://example.com/new" {
		t.Fatalf("target not updated: %q", updated.TargetURL)
	}
	if updated.Enabled {
		t.Fatal("expected link to be disabled")
	}

	visit := doRequest(t, srv, http.MethodGet, "/edit", "", nil)
	if visit.Code != http.StatusNotFound {
		t.Fatalf("expected disabled link to 404, got %d", visit.Code)
	}
}

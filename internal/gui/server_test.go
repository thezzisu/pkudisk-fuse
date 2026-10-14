package gui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testServer(t *testing.T) (*server, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	secret, err := randomToken()
	if err != nil {
		t.Fatal(err)
	}
	stops, opens := new(atomic.Int32), new(atomic.Int32)
	return &server{host: "127.0.0.1:12345", origin: "http://127.0.0.1:12345", version: "test",
		control: secret, tokens: newTokens(), stop: func() { stops.Add(1) },
		activate: func() error { opens.Add(1); return nil }}, stops, opens
}

func request(s *server, method, path, token, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, s.origin+path, strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func login(t *testing.T, s *server) string {
	t.Helper()
	ticket, err := s.tokens.ticket()
	if err != nil {
		t.Fatal(err)
	}
	w := request(s, "POST", "/api/session", "", s.origin, fmt.Sprintf(`{"ticket":%q}`, ticket))
	var result struct {
		Token string `json:"token"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil || len(result.Token) != 43 {
		t.Fatalf("login returned %d", w.Code)
	}
	if w.Header().Get("Set-Cookie") != "" {
		t.Fatal("browser auth must not set cookies")
	}
	return result.Token
}

func TestBrowserAuthenticationAndExit(t *testing.T) {
	s, stops, _ := testServer(t)
	if w := request(s, "GET", "/api/status", "", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	token := login(t, s)
	w := request(s, "GET", "/api/status", token, "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"version":"test"`) {
		t.Fatal(w.Code)
	}
	if w := request(s, "GET", "/api/quit", token, "", ""); w.Code != 405 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/quit", token, "", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if stops.Load() != 0 {
		t.Fatal("unsafe request stopped service")
	}
	if w := request(s, "POST", "/api/quit", token, s.origin, ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if stops.Load() != 1 {
		t.Fatal("authenticated quit was ignored")
	}
}

func TestOriginAndRebindingProtection(t *testing.T) {
	s, stops, opens := testServer(t)
	token := login(t, s)
	for _, origin := range []string{"https://evil.example", "null", "http://127.0.0.1:12346", "http://localhost:12345"} {
		for _, path := range []string{"/", "/api/status", "/api/quit", "/api/session", "/internal/activate"} {
			w := request(s, "POST", path, token, origin, `{}`)
			if w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("%s %s: %d", origin, path, w.Code)
			}
		}
	}
	for _, host := range []string{"evil.example:12345", "localhost:12345", "127.0.0.1", "127.0.0.1:1"} {
		r := httptest.NewRequest("GET", s.origin+"/api/status", nil)
		r.Host = host
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Forwarded-Host", s.host)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("host %s: %d", host, w.Code)
		}
	}
	for _, site := range []string{"cross-site", "same-site"} {
		r := httptest.NewRequest("POST", s.origin+"/api/quit", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", s.origin)
		r.Header.Set("Sec-Fetch-Site", site)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal(site, w.Code)
		}
	}
	if stops.Load() != 0 || opens.Load() != 0 {
		t.Fatal("rejected request caused a side effect")
	}
}

func TestTicketRedemptionCannotBeForgedByForms(t *testing.T) {
	s, _, _ := testServer(t)
	ticket, _ := s.tokens.ticket()
	body := fmt.Sprintf(`{"ticket":%q}`, ticket)
	for _, contentType := range []string{"text/plain", "application/x-www-form-urlencoded", "multipart/form-data", ""} {
		r := httptest.NewRequest("POST", s.origin+"/api/session", strings.NewReader(body))
		r.Header.Set("Origin", s.origin)
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 415 {
			t.Fatal(contentType, w.Code)
		}
	}
	if w := request(s, "POST", "/api/session", "", "", body); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := request(s, "OPTIONS", "/api/session", "", "https://evil.example", ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	for _, malformed := range []string{body + body, body + strings.Repeat(" ", 5000), `{"ticket":"x"}`, `{"ticket":"x","unknown":1}`} {
		if w := request(s, "POST", "/api/session", "", s.origin, malformed); w.Code != 400 {
			t.Fatal(w.Code)
		}
	}
	if w := request(s, "POST", "/api/session", "", s.origin, body); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/api/session", "", s.origin, body); w.Code != 401 {
		t.Fatal("replayed ticket", w.Code)
	}
}

func TestControlAndBrowserCredentialsAreSeparate(t *testing.T) {
	s, _, opens := testServer(t)
	token := login(t, s)
	for _, credential := range []string{"", token} {
		if w := request(s, "POST", "/internal/activate", credential, "", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	if w := request(s, "GET", "/api/status", s.control, "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request(s, "POST", "/internal/activate", s.control, s.origin, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if opens.Load() != 0 {
		t.Fatal("browser activated control endpoint")
	}
	if w := request(s, "POST", "/internal/activate", s.control, "", ""); w.Code != 204 {
		t.Fatal(w.Code)
	}
	if opens.Load() != 1 {
		t.Fatal("control request ignored")
	}
}

func TestEmbeddedAssetsAndSecurityHeaders(t *testing.T) {
	s, _, _ := testServer(t)
	paths := []string{"/", "/accounts", "/icon.png"}
	found := map[string]bool{}
	if err := fs.WalkDir(assets, "web/_nuxt", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		for _, suffix := range []string{".js", ".css"} {
			if !entry.IsDir() && strings.HasSuffix(path, suffix) && !found[suffix] {
				found[suffix] = true
				paths = append(paths, "/"+strings.TrimPrefix(path, "web/"))
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatal("Nuxt asset bundle is missing scripts or styles")
	}
	for _, path := range paths {
		w := request(s, "GET", path, "", "", "")
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatal(path, w.Code)
		}
		cache := "no-store"
		if strings.HasPrefix(path, "/_nuxt/") {
			cache = "public, max-age=31536000, immutable"
		}
		referrer := "no-referrer"
		if path == "/" || path == "/accounts" || path == "/transfers" || path == "/mounts" || path == "/recycle" || path == "/recovery" || path == "/settings" {
			referrer = "strict-origin-when-cross-origin"
		}
		if w.Header().Get("Cache-Control") != cache || w.Header().Get("Referrer-Policy") != referrer ||
			!strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("missing protection", path)
		}
		if strings.Contains(w.Body.String(), s.control) {
			t.Fatal("asset leaked control credential")
		}
	}
	if w := request(s, "GET", "/?ticket=secret", "", "", ""); w.Code != 400 {
		t.Fatal("query credential accepted")
	}
	if w := request(s, "GET", "/instance.json", "", "", ""); w.Code != 404 {
		t.Fatal("instance metadata exposed")
	}
	if w := request(s, "HEAD", "/", "", "", ""); w.Code != 200 || w.Body.Len() != 0 {
		t.Fatal("invalid HEAD")
	}
}

func TestNuxtScriptsHavePerResponseNonces(t *testing.T) {
	s, _, _ := testServer(t)
	first := request(s, "GET", "/", "", "", "")
	second := request(s, "GET", "/", "", "", "")
	if first.Body.String() == second.Body.String() {
		t.Fatal("HTML nonce was reused")
	}
	policy := first.Header().Get("Content-Security-Policy")
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") || strings.Contains(policy, "unsafe-eval") {
		t.Fatal("unsafe script policy")
	}
	if !strings.Contains(first.Body.String(), `nonce="`) || !strings.Contains(policy, "'nonce-") {
		t.Fatal("Nuxt scripts have no nonce")
	}
	input := []byte(`<script type="module" src="/entry.js"></script><script>const example="<script>";</script><style>body{}</style>`)
	out := string(nonceHTML(input, "fixture"))
	if strings.Count(out, `nonce="fixture"`) != 3 || !strings.Contains(out, `const example="<script>";`) {
		t.Fatal("nonce insertion altered inline script contents")
	}
}

func TestTicketExpirySessionExpiryAndRestart(t *testing.T) {
	tokens := newTokens()
	now := time.Now()
	tokens.now = func() time.Time { return now }
	ticket, _ := tokens.ticket()
	now = now.Add(ticketLifetime)
	if _, err := tokens.exchange(ticket); err == nil {
		t.Fatal("expired ticket accepted")
	}
	ticket, _ = tokens.ticket()
	session, err := tokens.exchange(ticket)
	if err != nil || !tokens.authenticated(session) || tokens.authenticated(ticket) {
		t.Fatal("credential types mixed")
	}
	if _, err := tokens.exchange(session); err == nil {
		t.Fatal("session used as ticket")
	}
	now = now.Add(sessionLifetime)
	if tokens.authenticated(session) {
		t.Fatal("expired session accepted")
	}
	ticket, _ = tokens.ticket()
	session, _ = tokens.exchange(ticket)
	tokens.clear()
	if tokens.authenticated(session) || newTokens().authenticated(session) {
		t.Fatal("session survived restart")
	}
}

func TestConcurrentTicketRedemption(t *testing.T) {
	tokens := newTokens()
	ticket, _ := tokens.ticket()
	var successes atomic.Int32
	var workers sync.WaitGroup
	for range 32 {
		workers.Go(func() {
			if _, err := tokens.exchange(ticket); err == nil {
				successes.Add(1)
			}
		})
	}
	workers.Wait()
	if successes.Load() != 1 {
		t.Fatalf("redeemed ticket %d times", successes.Load())
	}
}

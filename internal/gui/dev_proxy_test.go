package gui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestDevelopmentProxyOnlyAcceptsLoopbackOrigins(t *testing.T) {
	for _, value := range []string{"https://example.com", "file:///tmp/dev", "http://user@localhost:3000", "http://localhost:3000/path", "http://localhost:3000/?token=secret", "http://localhost:3000/#secret", "http://127.0.0.1.example.com:3000"} {
		if _, err := developmentProxy(value); err == nil {
			t.Errorf("invalid origin accepted: %q", value)
		}
	}
	for _, value := range []string{"http://127.0.0.1:3000", "http://localhost:3000/", "https://[::1]:3000"} {
		if _, err := developmentProxy(value); err != nil {
			t.Errorf("loopback rejected: %q: %v", value, err)
		}
	}
	proxy, err := developmentProxy("")
	if err != nil || proxy != nil {
		t.Fatal("an unset environment enabled development mode")
	}
	s, _, _ := testServer(t)
	s.devProxy = proxy
	w := request(s, http.MethodGet, "/", "", "", "")
	policy := w.Header().Get("Content-Security-Policy")
	if w.Code != http.StatusOK || !strings.Contains(policy, "'nonce-") || strings.Contains(policy, "unsafe-eval") || strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Fatal("production policy was relaxed", w.Code, policy)
	}
}

func TestDevelopmentProxyStripsCredentialsAndReservesBackendRoutes(t *testing.T) {
	var forwarded atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
		for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer", "X-Forwarded-Host"} {
			if r.Header.Get(name) != "" {
				t.Errorf("%s reached Nuxt", name)
			}
		}
		if r.URL.Path != "/_nuxt/dev.js" || r.URL.RawQuery != "v=123" {
			t.Errorf("asset URL changed: %s", r.URL)
		}
		w.Header().Set("Content-Type", "text/javascript")
		_, _ = io.WriteString(w, "export const hot = true")
	}))
	defer upstream.Close()
	s, _, _ := testServer(t)
	var err error
	s.devProxy, err = developmentProxy(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	token := login(t, s)
	r := httptest.NewRequest(http.MethodGet, s.origin+"/_nuxt/dev.js?v=123", nil)
	for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer", "X-Forwarded-Host"} {
		r.Header.Set(name, "private-local-credential")
	}
	r.Header.Set("Origin", s.origin)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "export const hot = true" || forwarded.Load() != 1 {
		t.Fatal("asset was not proxied", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api", "/api/status", "/api/unknown", "/internal", "/internal/unknown", "/internal/activate", "/internal/stop"} {
		_ = request(s, http.MethodGet, path, token, s.origin, "")
	}
	for _, path := range []string{"/?ticket=private", "/?token=private", "/_nuxt/?token=private", "/_nuxt/?access_token=private"} {
		if w := request(s, http.MethodGet, path, token, s.origin, ""); w.Code != http.StatusBadRequest {
			t.Errorf("credential query accepted: %s (%d)", path, w.Code)
		}
	}
	if w := request(s, http.MethodGet, "/_nuxt/dev.js", "", "https://evil.example", ""); w.Code != http.StatusForbidden {
		t.Fatal("foreign origin accepted")
	}
	if forwarded.Load() != 1 {
		t.Fatalf("API/control/credential requests reached Nuxt: %d", forwarded.Load())
	}
}

func TestDevelopmentProxyWebSocketKeepsHMRAndRejectsForeignOrCredentialQueries(t *testing.T) {
	var upgraded atomic.Int32
	upstream := httptest.NewServer(websocket.Handler(func(conn *websocket.Conn) {
		defer conn.Close()
		upgraded.Add(1)
		for _, name := range []string{"Authorization", "Cookie", "Proxy-Authorization", "Referer"} {
			if conn.Request().Header.Get(name) != "" {
				t.Errorf("websocket forwarded %s", name)
			}
		}
		var message string
		if err := websocket.Message.Receive(conn, &message); err != nil {
			return
		}
		_ = websocket.Message.Send(conn, "hot:"+message)
	}))
	defer upstream.Close()
	s, _, _ := testServer(t)
	var err error
	s.devProxy, err = developmentProxy(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	local := httptest.NewServer(s)
	defer local.Close()
	s.origin, s.host = local.URL, strings.TrimPrefix(local.URL, "http://")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	dial := func(path, origin, protocol string) (*websocket.Conn, error) {
		config, err := websocket.NewConfig("ws://"+s.host+path, origin)
		if err != nil {
			return nil, err
		}
		config.Protocol = []string{protocol}
		config.Header.Set("Authorization", "Bearer "+s.control)
		config.Header.Set("Cookie", "session=private")
		config.Header.Set("Referer", s.origin+"/#ticket=private")
		return config.DialContext(ctx)
	}
	conn, err := dial("/_nuxt/?token=vite-token", s.origin, "vite-hmr")
	if err != nil {
		t.Fatal("HMR handshake failed", err)
	}
	if err := websocket.Message.Send(conn, "changed"); err != nil {
		t.Fatal(err)
	}
	var message string
	if err := websocket.Message.Receive(conn, &message); err != nil || message != "hot:changed" {
		t.Fatal("HMR message was not forwarded", message, err)
	}
	conn.Close()
	for _, test := range []struct{ path, origin, protocol string }{
		{"/_nuxt/?token=vite-token", "https://evil.example", "vite-hmr"},
		{"/_nuxt/?token=" + s.control, s.origin, "vite-hmr"},
		{"/_nuxt/?token=vite-token&token=" + s.control, s.origin, "vite-hmr"},
		{"/_nuxt/?token=vite-token", s.origin, "other-protocol"},
		{"/api/status?token=vite-token", s.origin, "vite-hmr"},
		{"/_nuxt/?ticket=private", s.origin, "vite-hmr"},
	} {
		if conn, err := dial(test.path, test.origin, test.protocol); err == nil {
			conn.Close()
			t.Errorf("unsafe websocket accepted: %s", test.path)
		}
	}
	if upgraded.Load() != 1 {
		t.Fatalf("unsafe requests reached Nuxt websocket: %d", upgraded.Load())
	}
}

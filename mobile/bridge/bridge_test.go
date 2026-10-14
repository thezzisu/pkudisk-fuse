package mobilegui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBridgeRestartAndSingleUseActivation(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "data")
	bridge := NewBridge(directory, filepath.Join(t.TempDir(), "cache"), "test")
	bridge.Close()
	defer bridge.Close()
	for _, forced := range []bool{false, true, false} {
		address, err := bridge.Open()
		if err != nil {
			t.Fatal(err)
		}
		if bridge.QuitRequested() {
			t.Fatal("opening a new service retained the previous quit request")
		}
		u, err := url.Parse(address)
		if err != nil || u.Hostname() != "127.0.0.1" || u.RawQuery != "" || !bridge.Running() {
			t.Fatal("invalid activation")
		}
		origin := u.Scheme + "://" + u.Host
		client := &http.Client{Timeout: 5 * time.Second}
		ticket := strings.TrimPrefix(u.Fragment, "ticket=")
		for _, status := range []int{200, 401} {
			req, _ := http.NewRequest("POST", origin+"/api/session", strings.NewReader(`{"ticket":"`+ticket+`"}`))
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", "application/json")
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != status {
				t.Fatal("single-use activation failed", res.StatusCode)
			}
		}
		// Gomobile calls can arrive from multiple native worker threads.
		var wg sync.WaitGroup
		urls := make(chan string, 8)
		for range 8 {
			wg.Go(func() {
				u, err := bridge.Open()
				if err != nil {
					t.Error(err)
					return
				}
				urls <- u
			})
		}
		wg.Wait()
		close(urls)
		seen := map[string]bool{address: true}
		for u := range urls {
			if seen[u] || !strings.HasPrefix(u, origin+"/#ticket=") {
				t.Fatal("reopen did not reuse service with fresh ticket")
			}
			seen[u] = true
		}
		if forced {
			bridge.Close()
		} else if err := bridge.Stop(); err != nil {
			t.Fatal(err)
		}
		if bridge.Running() {
			t.Fatal("stopped bridge reports running")
		}
		if bridge.QuitRequested() == forced {
			t.Fatal("explicit quit and forced closure were not distinguished")
		}
		if _, err := os.Stat(filepath.Join(directory, "gui", "instance.json")); !os.IsNotExist(err) {
			t.Fatal("instance metadata remains", err)
		}
		if err := bridge.Stop(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBridgeObservesServiceExit(t *testing.T) {
	b := NewBridge(filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "cache"), "test")
	defer b.Close()
	if _, err := b.Open(); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	b.current.cancel()
	done := b.current.done
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("service did not stop")
	}
	if b.Running() {
		t.Fatal("service exit not observed")
	}
	if b.QuitRequested() {
		t.Fatal("unexpected service exit requested app closure")
	}
	if _, err := b.Open(); err != nil {
		t.Fatal("cannot reopen after service exit", err)
	}
}

func TestBrowserQuitClosesHostOnlyAfterAuthenticatedRequest(t *testing.T) {
	b := NewBridge(filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "cache"), "test")
	defer b.Close()
	address, err := b.Open()
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(address)
	origin := u.Scheme + "://" + u.Host
	client := &http.Client{Timeout: 5 * time.Second}
	request := func(path, token, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("POST", origin+path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	denied := request("/api/quit", "", "")
	denied.Body.Close()
	if denied.StatusCode != http.StatusUnauthorized || b.QuitRequested() || !b.Running() {
		t.Fatal("unauthenticated request affected the host lifetime")
	}
	login := request("/api/session", "", `{"ticket":"`+strings.TrimPrefix(u.Fragment, "ticket=")+`"}`)
	var session struct{ Token string }
	if err := json.NewDecoder(login.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	quit := request("/api/quit", session.Token, "")
	quit.Body.Close()
	if quit.StatusCode != http.StatusNoContent {
		t.Fatal("browser quit rejected", quit.StatusCode)
	}
	select {
	case <-b.current.done:
	case <-time.After(5 * time.Second):
		t.Fatal("browser quit did not finish")
	}
	if b.Running() || !b.QuitRequested() {
		t.Fatal("host was not told to close after explicit browser quit")
	}
}

func TestIdleSuspendCanResumeWithoutRequestingQuit(t *testing.T) {
	b := NewBridge(filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "cache"), "test")
	defer b.Close()
	first, err := b.Open()
	if err != nil {
		t.Fatal(err)
	}
	status, err := b.Status()
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Running, Busy bool
		Tasks         []json.RawMessage
	}
	if err := json.Unmarshal([]byte(status), &snapshot); err != nil || !snapshot.Running || snapshot.Busy || snapshot.Tasks == nil {
		t.Fatalf("invalid idle snapshot: %s (%v)", status, err)
	}
	if !b.Suspend() || b.Running() || b.QuitRequested() || !b.Suspended() {
		t.Fatal("idle suspend did not preserve its distinct lifecycle state")
	}
	b.Close() // Android can destroy the idle Service before the Activity returns.
	second, err := b.Open()
	if err != nil || first == second || !b.Running() || b.Suspended() || b.QuitRequested() {
		t.Fatalf("cannot resume suspended host: %v", err)
	}
}

package gui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func uploadRequest(t *testing.T, s *server, token, relative string, data []byte, overwrite bool) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fields := map[string]string{"account": "campus", "destination": fixtureLibrary, "relativePath": relative, "lastModified": "1720000000000", "timeoutSeconds": "600", "fast": "true", "clientId": "browser-fixture"}
	if overwrite {
		fields["overwrite"] = "true"
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", "picked.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = writer.Close()
	r := httptest.NewRequest("POST", s.origin+"/api/uploads", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Origin", s.origin)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestBrowserUploadFolderConflictAndTemporaryCleanup(t *testing.T) {
	f, _, s, token := applicationServer(t)
	temp := t.TempDir()
	t.Setenv("TMPDIR", temp)
	t.Setenv("TMP", temp)
	t.Setenv("TEMP", temp)
	// Exceeds the parser's memory limit, exercising disk spooling and cleanup.
	payload := bytes.Repeat([]byte("browser-file-data\n"), 600000)
	view := decodeResponse[jobInfo](t, uploadRequest(t, s, token, "Browser folder/notes.txt", payload, false), 200)
	if view.State != "completed" || view.ClientID != "browser-fixture" {
		t.Fatal(view)
	}
	var uploaded []byte
	for _, object := range f.server.Objects("campus") {
		if object.Entry.Name == "notes.txt" {
			uploaded = object.Data
		}
	}
	if !bytes.Equal(uploaded, payload) {
		t.Fatal("uploaded bytes differ")
	}
	if files, err := os.ReadDir(temp); err != nil || len(files) != 0 {
		t.Fatal("temporary upload leaked", files, err)
	}
	if w := uploadRequest(t, s, token, "Browser folder/notes.txt", []byte("replacement"), false); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, relative := range []string{"../escape.txt", "/absolute.txt", "bad/../escape", "bad\\name/file.txt"} {
		if w := uploadRequest(t, s, token, relative, []byte("unsafe"), false); w.Code != 400 {
			t.Fatal(relative, w.Code, w.Body.String())
		}
	}
	if w := uploadRequest(t, s, "", "untrusted.txt", payload, false); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/api/picker"} {
		if w := appRequest(t, s, token, "POST", path, map[string]any{}); w.Code != 404 {
			t.Fatal("native picker still exposed")
		}
	}
	if w := appRequest(t, s, "", "POST", "/api/tasks", operationInput{Kind: "upload", Source: filepath.Join(f.root, "upload.txt"), Destination: fixtureLibrary}); w.Code != 401 {
		t.Fatal("unauthenticated local path upload accepted")
	}
}

type preparedDownload struct {
	ID     string `json:"id"`
	URL    string `json:"url"`
	cookie *http.Cookie
}

func prepareAttachment(t *testing.T, s *server, session string, archive bool) preparedDownload {
	t.Helper()
	path := fixtureLibrary + "/report"
	if archive {
		path = fixtureLibrary + "/folder"
	}
	w := appRequest(t, s, session, "POST", "/api/downloads", browserDownload{Account: "campus", Path: path, Archive: archive})
	prepared := decodeResponse[preparedDownload](t, w, http.StatusOK)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one attachment proof cookie: %v", cookies)
	}
	prepared.cookie = cookies[0]
	if prepared.URL != "/download/"+prepared.ID || len(prepared.ID) != 43 || strings.Contains(prepared.URL, prepared.cookie.Value) {
		t.Fatal("resource URL contained authority or was malformed", prepared.URL)
	}
	if prepared.cookie.Name != downloadCookie || prepared.cookie.Path != prepared.URL || !prepared.cookie.HttpOnly || prepared.cookie.SameSite != http.SameSiteStrictMode || prepared.cookie.MaxAge != 60 || len(prepared.cookie.Value) != 43 {
		t.Fatal("attachment cookie scope or lifetime is incorrect", prepared.cookie)
	}
	if strings.Contains(w.Body.String(), prepared.cookie.Value) {
		t.Fatal("proof exposed to script in JSON")
	}
	return prepared
}

func attachmentRequest(s *server, prepared preparedDownload, cookie *http.Cookie, origin, site string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, s.origin+prepared.URL, nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	r.Header.Set("Origin", origin)
	r.Header.Set("Sec-Fetch-Site", site)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestBrowserAttachmentsUsePathScopedProofAndStreamTasks(t *testing.T) {
	_, a, s, session := applicationServer(t)
	prepared := prepareAttachment(t, s, session, false)
	// Knowing the URL alone, session token or a different attachment's cookie
	// does not authorize this attachment and does not consume either GET.
	if w := attachmentRequest(s, prepared, nil, "", ""); w.Code != 401 {
		t.Fatal("resource URL acted as a credential", w.Code)
	}
	other := prepareAttachment(t, s, session, false)
	if w := attachmentRequest(s, prepared, other.cookie, "", ""); w.Code != 401 {
		t.Fatal("proof was not bound to resource", w.Code)
	}
	for _, credential := range []string{session, s.control, prepared.cookie.Value} {
		r := httptest.NewRequest("GET", s.origin+prepared.URL, nil)
		r.Header.Set("Authorization", "Bearer "+credential)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("header authority was accepted for attachment", w.Code)
		}
	}
	for i := range 2 {
		w := attachmentRequest(s, prepared, prepared.cookie, "", "")
		if w.Code != 200 || w.Body.Len() != 1536 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") {
			t.Fatal(w.Code, w.Header(), w.Body.Len())
		}
		if i == 1 {
			cookies := w.Result().Cookies()
			if len(cookies) != 1 || cookies[0].MaxAge != -1 {
				t.Fatal("spent proof cookie not cleared")
			}
		}
	}
	if w := attachmentRequest(s, prepared, prepared.cookie, "", ""); w.Code != 401 {
		t.Fatal("third GET replay succeeded", w.Code)
	}
	a.mu.Lock()
	matched := 0
	for _, j := range a.jobs {
		if j.info.Kind == "download" && j.info.ClientID == prepared.ID {
			matched++
			if j.info.State != "completed" {
				t.Error(j.info.State)
			}
		}
	}
	a.mu.Unlock()
	if matched != 2 {
		t.Fatal("stream jobs lost the shared preparation ID", matched)
	}
	archive := prepareAttachment(t, s, session, true)
	w := attachmentRequest(s, archive, archive.cookie, s.origin, "same-origin")
	reader, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if w.Code != 200 || err != nil || len(reader.File) != 1 {
		t.Fatal("bad ZIP", w.Code, err)
	}
}

func TestAttachmentProofExpiresAndCannotAuthorizeAPIs(t *testing.T) {
	_, a, s, session := applicationServer(t)
	prepared := prepareAttachment(t, s, session, false)
	a.mu.Lock()
	input := a.downloads[prepared.ID]
	input.Expires = time.Now().Add(-time.Second)
	a.downloads[prepared.ID] = input
	a.mu.Unlock()
	if w := attachmentRequest(s, prepared, prepared.cookie, "", ""); w.Code != 401 {
		t.Fatal("expired proof accepted")
	}
	fresh := prepareAttachment(t, s, session, false)
	for _, path := range []string{"/api/status", "/api/accounts", "/api/downloads/data?account=campus&path=" + url.QueryEscape(fixtureLibrary+"/report") + "&timeoutSeconds=600"} {
		r := httptest.NewRequest("GET", s.origin+path, nil)
		r.AddCookie(fresh.cookie)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("attachment cookie authorized API", path, w.Code)
		}
	}
	if w := appRequest(t, s, session, "GET", "/api/downloads/data", nil); w.Code != 404 {
		t.Fatal("obsolete fetch download route remains available", w.Code)
	}
}

func TestAttachmentRejectsCrossOriginAndHostRebinding(t *testing.T) {
	_, _, s, session := applicationServer(t)
	prepared := prepareAttachment(t, s, session, false)
	for _, values := range [][2]string{{"https://evil.example", ""}, {"null", ""}, {s.origin, "cross-site"}, {"", "same-site"}} {
		if w := attachmentRequest(s, prepared, prepared.cookie, values[0], values[1]); w.Code != 403 {
			t.Fatal("cross-origin attachment accepted", values, w.Code)
		}
	}
	r := httptest.NewRequest("GET", s.origin+prepared.URL, nil)
	r.Host = "rebound.example"
	r.AddCookie(prepared.cookie)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("rebound host accepted", w.Code)
	}
	for _, method := range []string{"POST", "HEAD"} {
		r := httptest.NewRequest(method, s.origin+prepared.URL, strings.NewReader("ticket="+prepared.cookie.Value))
		r.Header.Set("Origin", s.origin)
		r.AddCookie(prepared.cookie)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 405 {
			t.Fatal("non-GET attachment accepted", method, w.Code)
		}
	}
	// Rejected cross-origin requests did not spend either same-origin retrieval.
	if w := attachmentRequest(s, prepared, prepared.cookie, s.origin, "same-origin"); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if w := attachmentRequest(s, prepared, prepared.cookie, "", ""); w.Code != 200 {
		t.Fatal("OS download request rejected", w.Code)
	}
}

func TestConcurrentAttachmentRequestsCannotExceedTwoUses(t *testing.T) {
	_, _, s, session := applicationServer(t)
	prepared := prepareAttachment(t, s, session, false)
	codes := make(chan int, 8)
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() { codes <- attachmentRequest(s, prepared, prepared.cookie, "", "").Code })
	}
	group.Wait()
	close(codes)
	accepted := 0
	for code := range codes {
		if code == 200 {
			accepted++
		} else if code != 401 {
			t.Fatal(code)
		}
	}
	if accepted != 2 {
		t.Fatal("proof retrieval limit raced", accepted)
	}
}

func TestControlStopWaitsForCleanupAndRefusesActiveWork(t *testing.T) {
	f := newBusinessFixture(t)
	a := NewApplication(context.Background(), f.options())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	directory := filepath.Join(t.TempDir(), "gui")
	opened := make(chan string, 2)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Directory: directory, NewApplication: func(context.Context) (*Application, error) { return a, nil }, NewTray: func(func(), func()) (Tray, error) { return &testTray{done: make(chan struct{})}, nil }, OpenBrowser: func(address string) error { opened <- address; return nil }})
	}()
	_ = receiveURL(t, opened)
	release := make(chan struct{})
	view, err := a.newJob(jobInfo{Kind: "copy"}, 0, func(ctx context.Context, _ *job) (any, error) {
		select {
		case <-release:
			return nil, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Stop(ctx, directory); err != ErrBusy {
		t.Fatal("stop ignored active work", err)
	}
	close(release)
	waitJob(t, a, view.ID)
	stopped, err := Stop(ctx, directory)
	if err != nil || !stopped {
		t.Fatal(stopped, err)
	}
	awaitRun(t, done)
	if _, err := os.Stat(filepath.Join(directory, "instance.json")); !os.IsNotExist(err) {
		t.Fatal("stop returned before metadata cleanup")
	}
	if stopped, err := Stop(ctx, directory); err != nil || stopped {
		t.Fatal("idle stop failed", stopped, err)
	}
	select {
	case <-opened:
		t.Fatal("stop opened a browser")
	default:
	}
}

func TestControlStopRejectsBrowserAuthority(t *testing.T) {
	s, stops, _ := testServer(t)
	token := login(t, s)
	for _, credential := range []string{"", token} {
		if w := request(s, "POST", "/internal/stop", credential, "", ""); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	for _, origin := range []string{s.origin, "https://evil.example"} {
		if w := request(s, "POST", "/internal/stop", s.control, origin, ""); w.Code < 400 {
			t.Fatal("browser-controlled request accepted")
		}
	}
	r := httptest.NewRequest("POST", s.origin+"/internal/stop", nil)
	r.Header.Set("Authorization", "Bearer "+s.control)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	if stops.Load() != 0 {
		t.Fatal("unauthorized stop")
	}
	if w := request(s, "POST", "/internal/stop", s.control, "", ""); w.Code != 204 || stops.Load() != 1 {
		t.Fatal(w.Code)
	}
}

func TestRecoveryStillUploadsRetainedContents(t *testing.T) {
	f, a, s, token := applicationServer(t)
	cache := filepath.Join(f.root, "recovery")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(cache, "retained.data")
	if err := os.WriteFile(data, []byte("saved work"), 0600); err != nil {
		t.Fatal(err)
	}
	record := map[string]any{"data": "retained.data", "entry": map[string]any{"name": "recovered.txt"}}
	encoded, _ := json.Marshal(record)
	_ = os.WriteFile(filepath.Join(cache, "retained.json"), encoded, 0600)
	view := decodeResponse[jobInfo](t, appRequest(t, s, token, "POST", "/api/tasks", operationInput{Kind: "recover", Account: "campus", Source: data, Destination: fixtureLibrary}), 202)
	if result := waitJob(t, a, view.ID); result.State != "completed" {
		t.Fatal(result.Error)
	}
}

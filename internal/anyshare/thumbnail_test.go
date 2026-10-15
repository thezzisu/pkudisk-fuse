package anyshare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var thumbnailEntry = Entry{ID: "gns://library/photo", Name: "照片.JPG", Size: 1024, Rev: "current"}

func thumbnailPNG(t *testing.T) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func thumbnailClient(t *testing.T, address string) *Client {
	t.Helper()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/open-doc/v1/file-thumbnail" || q.Get("id") != "photo" || q.Get("type") != "96*96" || q.Get("redirect") != "false" || q.Has("token") || r.Header.Get("Authorization") != "Bearer private-account-token" {
			t.Error("thumbnail API did not use the verified protocol")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"url": address})
	}))
	t.Cleanup(endpoint.Close)
	client, err := New(endpoint.URL, &testTokenSource{token: "private-account-token"})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestThumbnailUsesServerSignedStorageWithoutAccountCredentials(t *testing.T) {
	payload := thumbnailPNG(t)
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.URL.Query().Get("signed") != "storage-proof" {
			t.Error("account authority leaked into storage request or signed URL changed")
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer storage.Close()
	client := thumbnailClient(t, storage.URL+"/thumbnail?signed=storage-proof")
	preview, err := client.Thumbnail(context.Background(), thumbnailEntry)
	if err != nil || preview.ContentType != "image/png" || !bytes.Equal(preview.Data, payload) {
		t.Fatal("thumbnail bytes or detected MIME differ", err, preview.ContentType)
	}
}

func TestThumbnailRejectsActiveOversizedIncompleteAndRedirectedContent(t *testing.T) {
	for _, test := range []struct {
		name string
		body func(http.ResponseWriter, *http.Request)
	}{
		{"html", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = io.WriteString(w, "<html>active content</html>")
		}},
		{"svg", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `<svg xmlns="http://www.w3.org/2000/svg"><script>bad()</script></svg>`)
		}},
		{"declared oversized", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", strconv.Itoa(maxThumbnailBytes+1))
			w.WriteHeader(http.StatusOK)
		}},
		{"chunked oversized", func(w http.ResponseWriter, _ *http.Request) {
			w.(http.Flusher).Flush()
			_, _ = w.Write(make([]byte, maxThumbnailBytes+1))
		}},
		{"incomplete", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Length", "1000")
			_, _ = w.Write(thumbnailPNG(t))
		}},
		{"partial", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(thumbnailPNG(t))
		}},
		{"redirect", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/must-not-follow?private=secret", http.StatusFound)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/must-not-follow" {
					t.Error("storage redirect was followed")
				}
				test.body(w, r)
			}))
			defer storage.Close()
			client := thumbnailClient(t, storage.URL+"/thumbnail?private=secret")
			preview, err := client.Thumbnail(context.Background(), thumbnailEntry)
			if err == nil || len(preview.Data) != 0 || strings.Contains(err.Error(), "secret") {
				t.Fatal("unsafe thumbnail accepted or URL leaked", err)
			}
		})
	}
}

func TestThumbnailRejectsUntrustedURLsAndHTTPSDowngrade(t *testing.T) {
	for _, address := range []string{"data:image/png,private-secret", "https://user:private-secret@example.com/", "https://example.com/#private-secret", "//user:private-secret@example.com/"} {
		client := thumbnailClient(t, address)
		if _, err := client.Thumbnail(context.Background(), thumbnailEntry); err == nil || strings.Contains(err.Error(), "private-secret") {
			t.Fatal("invalid URL accepted or leaked", err)
		}
	}
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"url":"http://127.0.0.1:1/private-secret"}`)
	}))
	defer endpoint.Close()
	client, _ := New(endpoint.URL, &testTokenSource{token: "private-account-token"})
	client.api = endpoint.Client()
	if _, err := client.Thumbnail(context.Background(), thumbnailEntry); err == nil || strings.Contains(err.Error(), "private-secret") {
		t.Fatal("HTTPS downgrade accepted or URL leaked", err)
	}
}

func TestThumbnailProcessingDoesNotBusyPollAndCancellationClosesStorage(t *testing.T) {
	var requests atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"code":503008001}`)
	}))
	defer endpoint.Close()
	client, _ := New(endpoint.URL, &testTokenSource{token: "private-account-token"})
	if _, err := client.Thumbnail(context.Background(), thumbnailEntry); !errors.Is(err, ErrThumbnailPending) || requests.Load() != 1 {
		t.Fatal("pending thumbnail retried synchronously", err, requests.Load())
	}
	started, stopped := make(chan struct{}), make(chan struct{})
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(stopped)
	}))
	defer storage.Close()
	client = thumbnailClient(t, storage.URL+"/thumbnail")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.Thumbnail(ctx, thumbnailEntry); done <- err }()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled thumbnail did not stop", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("thumbnail storage request leaked")
	}
}

func TestThumbnailEligibilityMatchesOfficialIconTypes(t *testing.T) {
	for _, name := range []string{"photo.JPG", "video.mp4", "drawing.dwg", "layers.psd", "vector.ai", "render.svg"} {
		if !ThumbnailSupported(name) {
			t.Fatal("official thumbnail type omitted", name)
		}
	}
	for _, name := range []string{"notes.txt", "book.pdf", "doc.docx", "archive.zip", "unknown.webp", "folder"} {
		if ThumbnailSupported(name) {
			t.Fatal("unsupported thumbnail type accepted", name)
		}
	}
	client, _ := New("https://unused.example", &testTokenSource{token: "private-account-token"})
	for _, entry := range []Entry{
		{ID: "gns://library/folder", Name: "folder.jpg", Dir: true, Size: -1},
		{ID: "gns://library/empty", Name: "empty.jpg", Size: 0},
		{ID: "https://arbitrary.example/photo", Name: "photo.jpg", Size: 1},
	} {
		if _, err := client.Thumbnail(context.Background(), entry); !errors.Is(err, ErrThumbnailUnavailable) {
			t.Fatal("invalid entry issued a thumbnail request", err)
		}
	}
}

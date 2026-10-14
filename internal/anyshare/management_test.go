package anyshare

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecyclePaginationAndMutations(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.URL.Path == "/api/efast/v1/owned-doc-lib" {
			fmt.Fprint(w, `[{"id":"gns://lib","name":"Library","type":"user_doc_lib"}]`)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		switch r.URL.Path {
		case "/api/efast/v1/recycle/list":
			if body["docid"] != "gns://lib" || body["limit"] != float64(100) {
				t.Error(body)
			}
			start := int(body["start"].(float64))
			files := []map[string]any{}
			count := 100
			if start == 100 {
				count = 1
			} else if start != 0 {
				t.Error("unexpected offset", start)
			}
			for i := 0; i < count; i++ {
				files = append(files, map[string]any{"docid": fmt.Sprintf("gns://lib/f-%d", start+i), "name": fmt.Sprintf("file-%d", start+i), "size": -2, "modified": 1753084919050792, "path": "gns://Library/Original"})
			}
			json.NewEncoder(w).Encode(map[string]any{"files": files, "dirs": []any{}})
		case "/api/efast/v1/recycle/restore":
			if body["docid"] != "gns://lib/f-1" || body["ondup"] != float64(2) {
				t.Error(body)
			}
			fmt.Fprint(w, `{"docid":"gns://lib/renamed","name":"new name"}`)
		case "/api/efast/v1/recycle/delete":
			if body["docid"] != "gns://lib/f-2" {
				t.Error(body)
			}
			w.WriteHeader(204)
		default:
			t.Error("unexpected request", r.URL.Path)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL, &testTokenSource{token: "token"})
	ctx := context.Background()
	libraries, err := c.RecycleLibraries(ctx)
	if err != nil || len(libraries) != 1 || !libraries[0].Dir {
		t.Fatal(libraries, err)
	}
	entries, err := c.RecycleList(ctx, "gns://lib")
	if err != nil || len(entries) != 101 || entries[100].Size != -2 || !entries[0].Modified.Equal(time.UnixMicro(1753084919050792)) {
		t.Fatal(len(entries), err)
	}
	restored, err := c.RecycleRestore(ctx, "gns://lib/f-1", true)
	if err != nil || restored.ID != "gns://lib/renamed" {
		t.Fatal(restored, err)
	}
	if err := c.RecycleDelete(ctx, "gns://lib/f-2"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 5 {
		t.Fatal(requests)
	}
}

func TestRecycleRejectsRepeatedOrInvalidEntries(t *testing.T) {
	for _, test := range []struct{ name, reply string }{
		{"other library", `{"files":[{"docid":"gns://other/file","name":"test","size":1}]}`},
		{"invalid size", `{"files":[{"docid":"gns://lib/file","name":"test","size":-3}]}`},
		{"invalid date", `{"files":[{"docid":"gns://lib/file","name":"test","size":1,"modified":true}]}`},
		{"repeated", `{"files":[{"docid":"gns://lib/file","name":"test","size":1},{"docid":"gns://lib/file","name":"test","size":1}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, test.reply) }))
			defer server.Close()
			c, _ := New(server.URL, &testTokenSource{token: "token"})
			_, err := c.RecycleList(context.Background(), "gns://lib")
			if test.name == "repeated" {
				if err != nil {
					t.Fatal("repeated stable rows should be deduplicated", err)
				}
			} else if err == nil {
				t.Fatal("invalid recycle response accepted")
			}
		})
	}
}

func TestRecycleRestoreNameConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"code":403002039}`)
	}))
	defer server.Close()
	c, _ := New(server.URL, &testTokenSource{token: "token"})
	_, err := c.RecycleRestore(context.Background(), "gns://lib/file", false)
	if !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "--rename") {
		t.Fatal(err)
	}
}

func TestRevisionMetadataAndRestore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["docid"] != "gns://lib/file" {
			t.Error(body)
		}
		if strings.HasSuffix(r.URL.Path, "restorerevision") {
			if body["rev"] != "older" {
				t.Error(body)
			}
			w.WriteHeader(204)
			return
		}
		io.WriteString(w, `[{"rev":"older","size":12,"name":"report.txt","editor":"Editor","modified":1753084919050792}]`)
	}))
	defer server.Close()
	c, _ := New(server.URL, &testTokenSource{token: "token"})
	entry := Entry{ID: "gns://lib/file", Name: "report.txt"}
	versions, err := c.Revisions(context.Background(), entry)
	if err != nil || len(versions) != 1 || versions[0].Size != 12 || versions[0].Editor != "Editor" || !versions[0].Modified.Equal(time.UnixMicro(1753084919050792)) {
		t.Fatal(versions, err)
	}
	if err := c.RestoreRevision(context.Background(), entry, "older"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Revisions(context.Background(), Entry{ID: entry.ID, Dir: true}); err == nil {
		t.Fatal("directory versions accepted")
	}
}

func TestFastUploadHashesMatchOfficialProtocol(t *testing.T) {
	data := bytes.Repeat([]byte("file bytes\x00\xff"), 40000)
	mtime := time.UnixMicro(1753084919050792)
	var progress []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["length"] != float64(len(data)) {
			t.Error(body)
		}
		switch r.URL.Path {
		case "/api/efast/v1/file/predupload":
			if body["slice_md5"] != fmt.Sprintf("%x", md5.Sum(data[:204800])) {
				t.Error("incorrect probe hash")
			}
			io.WriteString(w, `{"match":true}`)
		case "/api/efast/v1/file/dupload":
			if body["md5"] != fmt.Sprintf("%x", md5.Sum(data)) || body["crc32"] != fmt.Sprintf("%08x", crc32.ChecksumIEEE(data)) || body["docid"] != "gns://lib/folder" || body["name"] != "file.bin" || body["ondup"] != float64(1) || body["client_mtime"] != float64(mtime.UnixMilli()) {
				t.Error("incorrect full hashes or duplicate/time policy", body)
			}
			io.WriteString(w, `{"success":true,"docid":"gns://lib/folder/new","rev":"new-rev"}`)
		default:
			t.Error("unexpected endpoint", r.URL.Path)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL, &testTokenSource{token: "token"})
	matched, entry, err := c.FastUpload(context.Background(), "gns://lib/folder", "file.bin", bytes.NewReader(data), int64(len(data)), mtime, func(n int64) { progress = append(progress, n) })
	if err != nil || !matched || entry.ID != "gns://lib/folder/new" || entry.Name != "file.bin" || len(progress) < 3 || progress[0] != 0 || progress[len(progress)-1] != int64(len(data)) {
		t.Fatal(matched, entry, progress, err)
	}
}

func TestFastUploadMismatchAndCancellation(t *testing.T) {
	for _, reply := range []string{`{"match":false}`, `{"match":true}`} {
		t.Run(reply, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "predupload") {
					io.WriteString(w, reply)
				} else {
					io.WriteString(w, `{"success":false}`)
				}
			}))
			defer server.Close()
			c, _ := New(server.URL, &testTokenSource{token: "token"})
			matched, _, err := c.FastUpload(context.Background(), "gns://lib", "test", bytes.NewReader([]byte("abc")), 3, time.Now(), nil)
			if err != nil || matched || calls != map[string]int{`{"match":false}`: 1, `{"match":true}`: 2}[reply] {
				t.Fatal(matched, calls, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, _, err := uploadHashes(ctx, bytes.NewReader(make([]byte, 1<<20)), 1<<20, func(n int64) {
		if n > 0 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err := uploadHashes(context.Background(), bytes.NewReader(nil), 1, nil); !errors.Is(err, io.EOF) {
		t.Fatal("short source was accepted", err)
	}
	md, crc, err := uploadHashes(context.Background(), bytes.NewReader(nil), 0, nil)
	if err != nil || md != "d41d8cd98f00b204e9800998ecf8427e" || crc != "00000000" {
		t.Fatal(md, crc, err)
	}
}

func TestDestructiveOperationsRejectLibraryRootsAndMissingRevisions(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/revisions") {
			io.WriteString(w, `[{"rev":"retained","name":"report.txt","size":10}]`)
			return
		}
		mutations++
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client, _ := New(server.URL, &testTokenSource{token: "token"})
	for _, id := range []string{"", "gns://library"} {
		if err := client.Delete(context.Background(), Entry{ID: id, Dir: true}); !errors.Is(err, ErrPermission) {
			t.Fatalf("library deletion was not blocked: %v", err)
		}
	}
	if err := client.RestoreRevision(context.Background(), Entry{ID: "gns://library/file"}, "removed"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing revision was not rejected: %v", err)
	}
	if mutations != 0 {
		t.Fatal("invalid operation reached a mutation endpoint")
	}
}

type recycleDeletionFunc func(context.Context, string) error

func (f recycleDeletionFunc) RecycleDelete(ctx context.Context, id string) error { return f(ctx, id) }

func TestRecycleEmptyPreservesReviewedSnapshotAndPartialProgress(t *testing.T) {
	snapshot := []RecycleEntry{{Entry: Entry{ID: "first"}}, {Entry: Entry{ID: "second"}}, {Entry: Entry{ID: "third"}}}
	var removed []string
	var progress []int
	failure := errors.New("server unavailable")
	count, err := RecycleEmpty(context.Background(), recycleDeletionFunc(func(_ context.Context, id string) error {
		if id == "second" {
			return failure
		}
		removed = append(removed, id)
		return nil
	}), snapshot, func(completed, total int) {
		if total != len(snapshot) {
			t.Fatal("snapshot size changed")
		}
		progress = append(progress, completed)
	})
	if count != 1 || !errors.Is(err, failure) || strings.Join(removed, ",") != "first" || len(progress) != 1 || progress[0] != 1 {
		t.Fatalf("partial delete result lost: count=%d removed=%v progress=%v error=%v", count, removed, progress, err)
	}
}

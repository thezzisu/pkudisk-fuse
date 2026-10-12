package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thezzisu/pkudisk-fuse/internal/accounts"
	"github.com/thezzisu/pkudisk-fuse/internal/anyshare"
	"github.com/thezzisu/pkudisk-fuse/internal/auth"
)

func TestManagementCommandsRequireExplicitScriptConsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, args := range [][]string{
		{"delete", "/Library/file", "--non-interactive"},
		{"delete", "--non-interactive", "-y"},
		{"recycle", "remove", "gns://lib/bin/file", "--non-interactive"},
		{"recycle", "empty", "--non-interactive"},
		{"recycle", "restore", "--non-interactive"},
		{"recycle", "restore", "gns://lib/bin/file", "--non-interactive"},
		{"revision", "ls", "--non-interactive"},
		{"revision", "restore", "/Library/file", "old", "--non-interactive"},
		{"revision", "restore", "/Library/file", "--non-interactive", "-y"},
		{"recycle", "list", "--help"}, {"revision", "list", "--help"},
	} {
		out, stderr, err := testExecute(args...)
		if exitCode(err) != 2 || out != "" || strings.Contains(stderr, "Using account") {
			t.Fatal(args, out, stderr, err)
		}
	}
}

// Drive parsing, account restore, real HTTP payloads and both output streams
// together. Recycle IDs deliberately differ from the active document IDs.
func TestManagementCommandsThroughHTTP(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	const lib = "gns://lib"
	objects := map[string]anyshare.Entry{
		lib + "/file":   {ID: lib + "/file", Name: "file.txt", Size: 1536, Rev: "current"},
		lib + "/folder": {ID: lib + "/folder", Name: "Folder", Dir: true, Size: -1},
	}
	recycled := map[string]anyshare.RecycleEntry{}
	deletes, restores, removed, revisionRestores := 0, 0, 0, 0
	injectNew := false
	var offsets []int
	var refreshes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/anyshare/oauth2/login/refreshToken" {
			n := refreshes.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "client.oauth2_token", Value: fmt.Sprintf("access-%d", n), Path: "/"})
			http.SetCookie(w, &http.Cookie{Name: "client.oauth2_refresh_token", Value: fmt.Sprintf("refresh-%d", n), Path: "/"})
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer access-") {
			t.Error("unauthenticated API")
			w.WriteHeader(401)
			return
		}
		var body struct {
			DocID string `json:"docid"`
			Rev   string `json:"rev"`
			OnDup int    `json:"ondup"`
			Start int    `json:"start"`
			Limit int    `json:"limit"`
		}
		if r.Method == "POST" {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
		}
		switch r.URL.Path {
		case "/api/efast/v1/entry-doc-lib", "/api/efast/v1/owned-doc-lib":
			fmt.Fprint(w, `[{"id":"gns://lib","name":"Library","type":"user_doc_lib"}]`)
		case "/api/efast/v1/folders/" + lib + "/sub_objects":
			dirs, files := []any{}, []any{}
			for _, entry := range objects {
				if entry.Dir {
					dirs = append(dirs, entry)
				} else {
					files = append(files, entry)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"dirs": dirs, "files": files})
		case "/api/efast/v1/file/delete", "/api/efast/v1/dir/delete":
			deletes++
			entry := objects[body.DocID]
			delete(objects, body.DocID)
			entry.ID = lib + "/bin/" + strings.TrimPrefix(entry.ID, lib+"/")
			entry.Modified = time.UnixMicro(1753084919050792)
			recycled[entry.ID] = anyshare.RecycleEntry{Entry: entry, Path: "gns://Library"}
			w.WriteHeader(204)
		case "/api/efast/v1/recycle/list":
			if body.DocID != lib || body.Limit != 100 {
				t.Error("unbounded/incorrect recycle request", body)
			}
			offsets = append(offsets, body.Start)
			ids := []string{}
			for id := range recycled {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			dirs, files := []any{}, []any{}
			for _, id := range ids[min(body.Start, len(ids)):min(body.Start+body.Limit, len(ids))] {
				entry := recycled[id]
				item := map[string]any{"docid": entry.ID, "name": entry.Name, "size": entry.Size, "modified": entry.Modified.UnixMicro(), "path": entry.Path}
				if entry.Dir {
					dirs = append(dirs, item)
				} else {
					files = append(files, item)
				}
			}
			json.NewEncoder(w).Encode(map[string]any{"dirs": dirs, "files": files})
		case "/api/efast/v1/recycle/restore":
			restores++
			entry, ok := recycled[body.DocID]
			if !ok {
				w.WriteHeader(404)
				return
			}
			delete(recycled, body.DocID)
			entry.ID = strings.Replace(entry.ID, "/bin/", "/", 1)
			objects[entry.ID] = entry.Entry
			json.NewEncoder(w).Encode(map[string]string{"docid": entry.ID, "name": entry.Name})
		case "/api/efast/v1/recycle/delete":
			removed++
			delete(recycled, body.DocID)
			if injectNew {
				injectNew = false
				recycled[lib+"/bin/new"] = anyshare.RecycleEntry{Entry: anyshare.Entry{ID: lib + "/bin/new", Name: "New concurrent deletion", Size: 1}}
			}
			w.WriteHeader(204)
		case "/api/efast/v1/file/revisions":
			if body.DocID != lib+"/file" {
				t.Error(body)
			}
			fmt.Fprint(w, `[{"rev":"current","size":1536,"name":"file.txt","editor":"Editor","modified":1753084919050792},{"rev":"old","size":4,"name":"old.txt","editor":"Editor","modified":1753084900000000}]`)
		case "/api/efast/v1/file/restorerevision":
			if body.DocID != lib+"/file" || body.Rev != "old" {
				t.Error(body)
			}
			revisionRestores++
			w.WriteHeader(204)
		default:
			t.Error("unexpected request", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	store, err := accounts.Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(accounts.Account{Name: "fixture", Username: "test", Password: "password", Server: server.URL, Session: auth.State{AccessToken: "old", RefreshToken: "old"}}, true, false); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		t.Helper()
		out, stderr, err := testExecute(append(args, "--non-interactive")...)
		if !strings.Contains(stderr, `Using account "fixture"`) || strings.Contains(out, "Using account") {
			t.Fatal("account announcement misplaced", out, stderr, err)
		}
		return out, err
	}
	mustRun := func(args ...string) string {
		t.Helper()
		out, err := run(args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return out
	}
	if _, err := run("delete", "/Library", "-ry"); err == nil || deletes != 0 {
		t.Fatal("library root deleted")
	}
	if _, err := run("delete", "/Library/Folder", "-y"); exitCode(err) != 2 || deletes != 0 {
		t.Fatal("directory deleted without recursive consent", err)
	}
	text := mustRun("revision", "ls", "/Library/file.txt")
	if !strings.Contains(text, "1.5 KiB") || !strings.Contains(text, "CURRENT") || !strings.Contains(text, "old") {
		t.Fatal(text)
	}
	if text := mustRun("revision", "ls", "/Library/file.txt", "-b"); strings.Contains(text, "KiB") || !strings.Contains(text, "1536") {
		t.Fatal(text)
	}
	mustRun("revision", "restore", "/Library/file.txt", "old", "-y")
	if _, err := run("revision", "restore", "/Library/file.txt", "missing", "-y"); err == nil || revisionRestores != 1 {
		t.Fatal("unknown version restored")
	}
	mustRun("delete", "/Library/file.txt", "-y")
	var entries []anyshare.RecycleEntry
	if err := json.Unmarshal([]byte(mustRun("recycle", "ls", "/Library", "-j")), &entries); err != nil || len(entries) != 1 || entries[0].ID != lib+"/bin/file" || entries[0].Size != 1536 || entries[0].Modified.IsZero() {
		t.Fatal(entries, err)
	}
	if text := mustRun("recycle", "ls"); !strings.Contains(text, "1.5 KiB") || !strings.Contains(text, "ORIGINAL PATH") {
		t.Fatal(text)
	}
	if text := mustRun("recycle", "ls", "-b"); strings.Contains(text, "KiB") || !strings.Contains(text, "1536") {
		t.Fatal(text)
	}
	mustRun("recycle", "restore", lib+"/bin/file", "--rename", "-y")
	if restores != 1 {
		t.Fatal("restore count", restores)
	}
	mustRun("delete", "/Library/Folder", "-ry")
	mustRun("recycle", "remove", lib+"/bin/folder", "-y")
	if removed != 1 {
		t.Fatal("permanent removal count", removed)
	}
	if out := mustRun("recycle", "ls", "-j"); strings.TrimSpace(out) != "[]" {
		t.Fatal(out)
	}

	// 101 entries force pagination. A concurrent new deletion must be retained,
	// and deleting while listing must not skip half of the original entries.
	for i := 0; i < 101; i++ {
		id := fmt.Sprintf("%s/bin/old-%03d", lib, i)
		recycled[id] = anyshare.RecycleEntry{Entry: anyshare.Entry{ID: id, Name: fmt.Sprintf("old-%03d", i), Size: -2}}
	}
	injectNew = true
	offsets = nil
	text = mustRun("recycle", "empty", "/Library", "-y")
	if !strings.Contains(text, "101") || removed != 102 || len(recycled) != 1 || recycled[lib+"/bin/new"].ID == "" || len(offsets) != 2 || offsets[1] != 100 {
		t.Fatal("empty failed snapshot/pagination guarantees", text, removed, len(recycled), offsets)
	}
}

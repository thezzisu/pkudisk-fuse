package gui

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalFilesystemPickerBoundary(t *testing.T) {
	_, _, s, token := applicationServer(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	path := "/api/local-fs?path=" + url.QueryEscape(root)
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if w := appRequest(t, s, "", "GET", path, nil); w.Code != 401 {
		t.Fatalf("unauthenticated filesystem access: %d", w.Code)
	}
	result := decodeResponse[localListing](t, appRequest(t, s, token, "GET", path, nil), 200)
	if result.Path != canonicalRoot || result.Empty || len(result.Entries) != 2 || result.Entries[0].Name != "empty" || !result.Entries[0].Empty || result.Entries[1].Size != 7 {
		t.Fatalf("invalid directory listing: %+v", result)
	}
	for _, invalid := range []string{".", "..", "../escape", "nested/name", `nested\name`, ""} {
		w := appRequest(t, s, token, "POST", "/api/local-fs/mkdir", map[string]string{"path": root, "name": invalid})
		if w.Code != 400 {
			t.Fatalf("unsafe name %q accepted: %d", invalid, w.Code)
		}
	}
	body := map[string]string{"path": root, "name": "新文件夹"}
	decodeResponse[map[string]string](t, appRequest(t, s, token, "POST", "/api/local-fs/mkdir", body), 201)
	if w := appRequest(t, s, token, "POST", "/api/local-fs/mkdir", body); w.Code != 409 {
		t.Fatalf("existing directory overwritten: %d", w.Code)
	}
	if w := appRequest(t, s, token, "GET", "/api/local-fs?path=relative", nil); w.Code != 400 {
		t.Fatalf("relative directory accepted: %d", w.Code)
	}
	if w := appRequest(t, s, token, "GET", path+"/missing", nil); w.Code != 404 {
		t.Fatalf("missing path: %d", w.Code)
	}
}

func TestLocalPathTransfersUseSharedQueue(t *testing.T) {
	f, app, s, token := applicationServer(t)
	upload := decodeResponse[jobInfo](t, appRequest(t, s, token, "POST", "/api/tasks", operationInput{
		Kind: "upload", Account: "campus", Source: filepath.Join(f.root, "upload.txt"), Destination: fixtureLibrary,
	}), 202)
	result := waitJob(t, app, upload.ID)
	if result.State != "completed" {
		t.Fatalf("upload failed: %+v", result)
	}
	destination := t.TempDir()
	download := decodeResponse[jobInfo](t, appRequest(t, s, token, "POST", "/api/tasks", operationInput{
		Kind: "download", Account: "campus", Source: fixtureLibrary + "/report", Destination: destination,
	}), 202)
	result = waitJob(t, app, download.ID)
	if result.State != "completed" {
		t.Fatalf("download failed: %+v", result)
	}
	if files, err := os.ReadDir(destination); err != nil || len(files) != 1 {
		t.Fatalf("download absent: %v %v", files, err)
	}
}

func TestLocalPickerOffersOnlyTransferableFilesAndResolvesDirectoryLinks(t *testing.T) {
	_, _, s, token := applicationServer(t)
	root := t.TempDir()
	file := filepath.Join(root, "actual.txt")
	directory := filepath.Join(root, "actual-directory")
	if err := os.WriteFile(file, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(file, filepath.Join(root, "file-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	link := filepath.Join(root, "directory-link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	result := decodeResponse[localListing](t, appRequest(t, s, token, "GET", "/api/local-fs?path="+url.QueryEscape(root), nil), 200)
	if len(result.Entries) != 3 {
		t.Fatalf("unexpected file choices: %+v", result.Entries)
	}
	for _, entry := range result.Entries {
		if entry.Name == "file-link" {
			t.Fatal("offered a symlink that Upload rejects")
		}
	}
	resolved := decodeResponse[localListing](t, appRequest(t, s, token, "GET", "/api/local-fs?path="+url.QueryEscape(link), nil), 200)
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Path != canonicalDirectory || !resolved.Empty {
		t.Fatalf("directory link not resolved: %+v", resolved)
	}
}

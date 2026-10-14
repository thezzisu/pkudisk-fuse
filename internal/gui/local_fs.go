package gui

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/adrg/xdg"
)

type localEntry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Dir      bool      `json:"dir"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Empty    bool      `json:"empty"`
}

type localRoot struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Available bool   `json:"available"`
	Free      bool   `json:"free,omitempty"`
}

type localListing struct {
	Path    string       `json:"path"`
	Parent  string       `json:"parent"`
	Empty   bool         `json:"empty"`
	Roots   []localRoot  `json:"roots"`
	Entries []localEntry `json:"entries"`
}

func localDirectory(path string) (string, error) {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return "", invalid("choose an absolute local directory")
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", localFSError(err)
	}
	info, err := os.Stat(canonical)
	if err != nil {
		return "", localFSError(err)
	}
	if !info.IsDir() {
		return "", invalid("choose a local directory")
	}
	return canonical, nil
}

func localFSError(err error) error {
	switch {
	case errors.Is(err, os.ErrPermission):
		return &apiError{Code: "permission", Message: "directory access denied"}
	case errors.Is(err, os.ErrNotExist):
		return &apiError{Code: "not_found", Message: "local path no longer exists"}
	case errors.Is(err, os.ErrExist):
		return &apiError{Code: "exists", Message: "a file or directory already uses this name"}
	default:
		return err
	}
}

func emptyDirectory(path string) bool {
	dir, err := os.Open(path)
	if err != nil {
		return false
	}
	defer dir.Close()
	_, err = dir.Readdirnames(1)
	return errors.Is(err, io.EOF)
}

func (a *Application) localRoots() []localRoot {
	roots := make([]localRoot, 0, 8)
	if xdg.Home != "" {
		roots = append(roots, localRoot{Kind: "home", Path: xdg.Home},
			localRoot{Kind: "desktop", Path: xdg.UserDirs.Desktop},
			localRoot{Kind: "downloads", Path: xdg.UserDirs.Download})
	}
	if a.opts.CacheDirectory != "" {
		roots = append(roots, localRoot{Kind: "cache", Path: a.opts.CacheDirectory})
	}
	for i := range roots {
		roots[i].Name = filepath.Base(roots[i].Path)
		info, err := os.Stat(roots[i].Path)
		roots[i].Available = err == nil && info.IsDir()
	}
	return append(roots, localDrives()...)
}

func allowLocalFS(w http.ResponseWriter) bool {
	if hostFlags().CanBrowseLocalFS {
		return true
	}
	writeError(w, &apiError{Code: "permission", Message: "this host does not expose its local filesystem"})
	return false
}

func (a *Application) browseLocalFS(w http.ResponseWriter, r *http.Request) {
	if !allowLocalFS(w) {
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		var err error
		path, err = os.UserHomeDir()
		if err != nil {
			writeError(w, err)
			return
		}
	}
	path, err := localDirectory(path)
	if err != nil {
		writeError(w, err)
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		writeError(w, localFSError(err))
		return
	}
	result := localListing{Path: path, Parent: filepath.Dir(path), Empty: len(entries) == 0,
		Roots: a.localRoots(), Entries: make([]localEntry, 0, len(entries))}
	for _, entry := range entries {
		if err := r.Context().Err(); err != nil {
			writeError(w, err)
			return
		}
		child := filepath.Join(path, entry.Name())
		info, err := os.Stat(child)
		if err != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
			continue
		}
		// Upload deliberately rejects file symlinks. Do not offer files that the
		// shared transfer service cannot consume. Directory links remain browsable;
		// localDirectory resolves them before a directory can be selected.
		if entry.Type()&os.ModeSymlink != 0 && !info.IsDir() {
			continue
		}
		result.Entries = append(result.Entries, localEntry{Name: entry.Name(), Path: child,
			Dir: info.IsDir(), Size: info.Size(), Modified: info.ModTime(), Empty: info.IsDir() && emptyDirectory(child)})
	}
	slices.SortFunc(result.Entries, func(a, b localEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	writeJSON(w, result)
}

func (a *Application) mkdirLocalFS(w http.ResponseWriter, r *http.Request) {
	if !allowLocalFS(w) {
		return
	}
	var input struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Name == "" || input.Name == "." || input.Name == ".." || strings.ContainsAny(input.Name, "/\\\x00") || filepath.Base(input.Name) != input.Name {
		writeError(w, invalid("choose a single folder name"))
		return
	}
	parent, err := localDirectory(input.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	// OpenRoot anchors creation to this directory even if its ancestors are renamed
	// or replaced while the authenticated browser request is in flight.
	root, err := os.OpenRoot(parent)
	if err != nil {
		writeError(w, localFSError(err))
		return
	}
	defer root.Close()
	if err := root.Mkdir(input.Name, 0700); err != nil {
		writeError(w, localFSError(err))
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"path": filepath.Join(parent, input.Name), "name": input.Name})
}

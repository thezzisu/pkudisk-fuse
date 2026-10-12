package anyshare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// RecycleEntry has a recycle-bin ID, distinct from the original document ID.
// Modified is the deletion time; Path is the original parent location.
type RecycleEntry struct {
	Entry
	Path string `json:"path"`
}

// RecycleLibraries matches the official client's recycle-bin root. Accessible
// shared libraries may not permit recycle management.
func (c *Client) RecycleLibraries(ctx context.Context) ([]Entry, error) {
	b, err := c.request(ctx, "GET", "/efast/v1/owned-doc-lib", nil)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	for i := range entries {
		entries[i].Dir, entries[i].Size = true, -1
	}
	return entries, nil
}

type recycledObject struct {
	DocID    string          `json:"docid"`
	Name     string          `json:"name"`
	Size     int64           `json:"size"`
	Path     string          `json:"path"`
	Modified json.RawMessage `json:"modified"`
}

// RecycleList reads all bounded pages before returning. Emptying the bin must
// operate on this snapshot, not advance offsets into a shrinking list.
func (c *Client) RecycleList(ctx context.Context, library string) ([]RecycleEntry, error) {
	if library == "" || library == "gns://" || library != LibraryID(library) {
		return nil, errors.New("recycle scope must be a library ID")
	}
	const limit = 100
	result := []RecycleEntry{}
	seen := map[string]bool{}
	for start := 0; ; start += limit {
		var response struct{ Dirs, Files []recycledObject }
		if err := c.post(ctx, "/efast/v1/recycle/list", map[string]any{"docid": library, "start": start, "limit": limit}, &response); err != nil {
			return nil, err
		}
		newEntries := 0
		for kind, entries := range [][]recycledObject{response.Dirs, response.Files} {
			for _, item := range entries {
				// Failed/unpublished uploads can appear as files with size -2.
				if LibraryID(item.DocID) != library || ParentID(item.DocID) == "" || !ValidName(item.Name) || item.Size < -2 {
					return nil, errors.New("server returned an invalid recycled object")
				}
				if seen[item.DocID] {
					// The endpoint has no stable cursor and can repeat rows while
					// the server refreshes its recycle index. Keep one copy and
					// stop below if a whole page makes no progress.
					continue
				}
				seen[item.DocID] = true
				newEntries++
				modified, err := searchTime(item.Modified)
				if err != nil {
					return nil, fmt.Errorf("invalid recycle timestamp: %w", err)
				}
				entry := Entry{ID: item.DocID, Name: item.Name, Size: item.Size, Dir: kind == 0, Modified: modified}
				if entry.Dir {
					entry.Size = -1
				}
				result = append(result, RecycleEntry{Entry: entry, Path: item.Path})
			}
		}
		if len(response.Dirs)+len(response.Files) < limit {
			return result, nil
		}
		if newEntries == 0 {
			return result, nil
		}
	}
}

// Rename asks the server to choose a new name on a restore name collision.
func (c *Client) RecycleRestore(ctx context.Context, id string, rename bool) (Entry, error) {
	if ParentID(id) == "" || LibraryID(id) == "" {
		return Entry{}, errors.New("restore requires a recycled object ID")
	}
	ondup := 1
	if rename {
		ondup = 2
	}
	var response Entry
	if err := c.post(ctx, "/efast/v1/recycle/restore", map[string]any{"docid": id, "ondup": ondup}, &response); err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code == 403002039 {
			return Entry{}, fmt.Errorf("restore name conflicts; use --rename to keep both objects: %w", ErrExists)
		}
		return Entry{}, err
	}
	if response.ID == "" || LibraryID(response.ID) != LibraryID(id) {
		return Entry{}, errors.New("restore response omitted a valid document ID")
	}
	return response, nil
}

// RecycleDelete permanently removes a recycled object. This cannot be undone.
func (c *Client) RecycleDelete(ctx context.Context, id string) error {
	if ParentID(id) == "" || LibraryID(id) == "" {
		return errors.New("permanent deletion requires a recycled object ID")
	}
	return c.post(ctx, "/efast/v1/recycle/delete", map[string]string{"docid": id}, nil)
}

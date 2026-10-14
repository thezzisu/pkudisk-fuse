package anyshare

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// ValidateDelete protects library roots across every surface, including direct
// callers of Delete. A library is an account resource, not a removable folder.
func ValidateDelete(entry Entry) error {
	if ParentID(entry.ID) == "" {
		return fmt.Errorf("library roots cannot be deleted: %w", ErrPermission)
	}
	return nil
}

// RecycleSnapshot reads all selected libraries before any destructive action.
// The caller can display and confirm this immutable selection before deletion.
func (c *Client) RecycleSnapshot(ctx context.Context, libraries []Entry) ([]RecycleEntry, error) {
	entries := []RecycleEntry{}
	for _, library := range libraries {
		items, err := c.RecycleList(ctx, library.ID)
		if err != nil {
			return nil, fmt.Errorf("recycle bin %s: %w", library.Name, err)
		}
		entries = append(entries, items...)
	}
	return entries, nil
}

type RecycleDeleter interface {
	RecycleDelete(context.Context, string) error
}

// RecycleEmpty removes only the reviewed snapshot, never newly arriving items.
// completed is retained on failure so all surfaces report partial progress.
func RecycleEmpty(ctx context.Context, backend RecycleDeleter, snapshot []RecycleEntry, observe func(completed, total int)) (completed int, err error) {
	for i, entry := range snapshot {
		if err := ctx.Err(); err != nil {
			return i, err
		}
		if err := backend.RecycleDelete(ctx, entry.ID); err != nil {
			return i, fmt.Errorf("remove %s: %w (%d of %d objects already removed)", entry.ID, err, i, len(snapshot))
		}
		if observe != nil {
			observe(i+1, len(snapshot))
		}
	}
	return len(snapshot), nil
}

func (c *Client) RecycleEmpty(ctx context.Context, snapshot []RecycleEntry, observe func(int, int)) (int, error) {
	return RecycleEmpty(ctx, c, snapshot, observe)
}

// Revision resolves a retained version and rejects stale or fabricated IDs.
func (c *Client) Revision(ctx context.Context, entry Entry, rev string) (Revision, error) {
	versions, err := c.Revisions(ctx, entry)
	if err != nil {
		return Revision{}, err
	}
	for _, version := range versions {
		if version.Rev == rev {
			return version, nil
		}
	}
	return Revision{}, fmt.Errorf("%w: revision %s", ErrNotFound, rev)
}

type LibraryStatsBackend interface {
	LibraryQuota(context.Context, string) (Quota, error)
	Writable(context.Context, string) (bool, error)
}

type LibraryStat struct {
	Quota                       *Quota `json:"quota,omitempty"`
	Writable                    *bool  `json:"writable,omitempty"`
	QuotaError, PermissionError error  `json:"-"`
}

// LibraryStats returns independent permission/quota results. A temporarily
// unavailable statistic must not hide libraries that can still be browsed.
func LibraryStats(ctx context.Context, backend LibraryStatsBackend, entries []Entry) map[string]LibraryStat {
	rows := make([]LibraryStat, len(entries))
	var work errgroup.Group
	work.SetLimit(4)
	for i, entry := range entries {
		work.Go(func() error {
			quota, err := backend.LibraryQuota(ctx, entry.ID)
			rows[i].QuotaError = err
			if err == nil {
				rows[i].Quota = &quota
			}
			writable, err := backend.Writable(ctx, entry.ID)
			rows[i].PermissionError = err
			if err == nil {
				rows[i].Writable = &writable
			}
			return nil
		})
	}
	_ = work.Wait()
	result := make(map[string]LibraryStat, len(entries))
	for i, entry := range entries {
		result[entry.ID] = rows[i]
	}
	return result
}

package anyshare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Revision struct {
	Rev      string    `json:"rev"`
	Name     string    `json:"name"`
	Editor   string    `json:"editor"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified_at"`
}

func (c *Client) Revisions(ctx context.Context, entry Entry) ([]Revision, error) {
	if entry.ID == "" || entry.Dir {
		return nil, errors.New("version history requires a remote file")
	}
	var response []struct {
		Rev      string          `json:"rev"`
		Name     string          `json:"name"`
		Editor   string          `json:"editor"`
		Size     int64           `json:"size"`
		Modified json.RawMessage `json:"modified"`
	}
	if err := c.post(ctx, "/efast/v1/file/revisions", map[string]string{"docid": entry.ID}, &response); err != nil {
		return nil, err
	}
	result := make([]Revision, 0, len(response))
	for _, item := range response {
		if item.Rev == "" || item.Size < 0 {
			return nil, errors.New("server returned an invalid file revision")
		}
		modified, err := searchTime(item.Modified)
		if err != nil {
			return nil, fmt.Errorf("invalid revision timestamp: %w", err)
		}
		result = append(result, Revision{Rev: item.Rev, Name: item.Name, Editor: item.Editor, Size: item.Size, Modified: modified})
	}
	return result, nil
}

func (c *Client) RestoreRevision(ctx context.Context, entry Entry, rev string) error {
	if entry.ID == "" || entry.Dir || rev == "" {
		return errors.New("file and revision are required")
	}
	return c.post(ctx, "/efast/v1/file/restorerevision", map[string]string{"docid": entry.ID, "rev": rev}, nil)
}

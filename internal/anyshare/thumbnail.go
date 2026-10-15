package anyshare

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

var (
	ErrThumbnailUnavailable = errors.New("thumbnail unavailable")
	ErrThumbnailPending     = errors.New("thumbnail is not ready")
)

const maxThumbnailBytes = 2 << 20

// Thumbnail is a small server-generated raster image, never an original file
// or an HTML/SVG preview. Signed storage URLs and account credentials remain
// inside the client.
type Thumbnail struct {
	Data        []byte
	ContentType string
}

// ThumbnailSupported follows the official client's file-icon support list.
// Office documents and PDFs use their normal file icons.
func ThumbnailSupported(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".gif", ".bmp", ".png", ".wmf", ".emf", ".svg", ".tga", ".tif",
		".avi", ".rmvb", ".rm", ".mp4", ".3gp", ".mkv", ".mov", ".mpg", ".mpeg", ".wmv", ".flv", ".asf", ".h264", ".x264", ".mts", ".m2ts", ".m4v",
		".dwg", ".dwt", ".dxf", ".psd", ".psb", ".ai":
		return true
	}
	return false
}

func (c *Client) Thumbnail(ctx context.Context, entry Entry) (Thumbnail, error) {
	if entry.Dir || entry.Size <= 0 || LibraryID(entry.ID) == "" || ParentID(entry.ID) == "" || !ValidName(entry.Name) || !ThumbnailSupported(entry.Name) {
		return Thumbnail{}, ErrThumbnailUnavailable
	}
	// The deployed endpoint accepts a bearer header, avoiding the official
	// browser client's token query parameter. Only the service chooses a URL.
	query := url.Values{"id": {objectID(entry.ID)}, "type": {"96*96"}, "redirect": {"false"}}
	data, err := c.request(ctx, http.MethodGet, "/open-doc/v1/file-thumbnail?"+query.Encode(), nil)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && api.Code == 503008001 {
			return Thumbnail{}, ErrThumbnailPending
		}
		return Thumbnail{}, err
	}
	var result struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(data, &result) != nil || result.URL == "" {
		return Thumbnail{}, ErrThumbnailUnavailable
	}
	base, _ := url.Parse(c.base)
	address, err := url.Parse(result.URL)
	if err != nil || address.User != nil || address.Fragment != "" {
		return Thumbnail{}, errors.New("invalid thumbnail storage URL")
	}
	address = base.ResolveReference(address)
	// Like file downloads, thumbnails may live on another signed HTTPS object
	// storage origin. signed sends no account bearer and refuses redirects and
	// HTTPS downgrades. The URL is never accepted as GUI input or returned to it.
	response, err := c.signed(ctx, signedRequest{http.MethodGet, address.String()}, nil, 0, nil)
	if err != nil {
		return Thumbnail{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ContentLength > maxThumbnailBytes {
		return Thumbnail{}, ErrThumbnailUnavailable
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, maxThumbnailBytes+1))
	if err != nil {
		return Thumbnail{}, err
	}
	if len(data) > maxThumbnailBytes || response.ContentLength >= 0 && int64(len(data)) != response.ContentLength {
		return Thumbnail{}, ErrThumbnailUnavailable
	}
	contentType := http.DetectContentType(data)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return Thumbnail{Data: data, ContentType: contentType}, nil
	default:
		return Thumbnail{}, ErrThumbnailUnavailable
	}
}

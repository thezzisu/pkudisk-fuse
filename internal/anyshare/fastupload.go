package anyshare

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

// FastUpload creates a new file using content already in object storage. False
// means no complete match, and ordinary upload can proceed. Existing files use
// the ordinary revision-checked uploader instead.
func (c *Client) FastUpload(ctx context.Context, parent, name string, data io.ReaderAt, size int64, mtime time.Time, observe func(int64)) (bool, Entry, error) {
	if !ValidName(name) || LibraryID(parent) == "" || size < 0 || data == nil {
		return false, Entry{}, errors.New("invalid fast-upload metadata")
	}
	probe := make([]byte, min(size, 204800))
	if err := ctx.Err(); err != nil {
		return false, Entry{}, err
	}
	if _, err := io.ReadFull(io.NewSectionReader(data, 0, int64(len(probe))), probe); err != nil {
		return false, Entry{}, err
	}
	digest := md5.Sum(probe)
	var match struct {
		Match bool `json:"match"`
	}
	if err := c.post(ctx, "/efast/v1/file/predupload", map[string]any{"slice_md5": hex.EncodeToString(digest[:]), "length": size}, &match); err != nil {
		return false, Entry{}, err
	}
	if !match.Match {
		return false, Entry{}, nil
	}
	md5sum, crc, err := uploadHashes(ctx, data, size, observe)
	if err != nil {
		return false, Entry{}, err
	}
	// dupload follows the official sync client, whose file.lastModified value
	// is milliseconds since Unix epoch (the object-storage uploader uses a
	// different microsecond field).
	body := map[string]any{"client_mtime": mtime.UnixMilli(), "crc32": crc, "docid": parent, "length": size, "md5": md5sum, "name": name, "ondup": 1}
	var response struct {
		Success bool   `json:"success"`
		DocID   string `json:"docid"`
		Rev     string `json:"rev"`
		Name    string `json:"name"`
	}
	if err := c.post(ctx, "/efast/v1/file/dupload", body, &response); err != nil {
		return false, Entry{}, err
	}
	if !response.Success {
		return false, Entry{}, nil
	}
	if ParentID(response.DocID) != parent || response.Rev == "" {
		return false, Entry{}, errors.New("fast upload completed but response omitted a valid ID or revision; inspect the destination before retrying")
	}
	if response.Name == "" {
		response.Name = name
	}
	return true, Entry{ID: response.DocID, Name: response.Name, Rev: response.Rev, Size: size}, nil
}

// Hash in one bounded-memory pass, checking cancellation between chunks.
func uploadHashes(ctx context.Context, data io.ReaderAt, size int64, observe func(int64)) (string, string, error) {
	md5sum, crc := md5.New(), crc32.NewIEEE()
	buf := make([]byte, 256<<10)
	if observe != nil {
		observe(0)
	}
	for off := int64(0); off < size; {
		if err := ctx.Err(); err != nil {
			return "", "", err
		}
		part := buf[:min(int64(len(buf)), size-off)]
		if _, err := io.ReadFull(io.NewSectionReader(data, off, int64(len(part))), part); err != nil {
			return "", "", err
		}
		md5sum.Write(part)
		crc.Write(part)
		off += int64(len(part))
		if observe != nil {
			observe(off)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(md5sum.Sum(nil)), fmt.Sprintf("%08x", crc.Sum32()), nil
}

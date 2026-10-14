// Package localfile provides private files, process locks and atomic file
// installation using the host platform's permissions and filesystem APIs.
package localfile

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func ExpandPath(value string) string {
	if value != "~" && !strings.HasPrefix(value, "~/") && !strings.HasPrefix(value, `~\`) {
		return value
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return value
	}
	if value == "~" {
		return dir
	}
	return filepath.Join(dir, filepath.FromSlash(value[2:]))
}

func ConfigDir() (string, error) {
	if value := os.Getenv("XDG_CONFIG_HOME"); value != "" {
		return value, nil
	}
	return os.UserConfigDir()
}

func CacheDir() (string, error) {
	if value := os.Getenv("XDG_CACHE_HOME"); value != "" {
		return value, nil
	}
	return os.UserCacheDir()
}

func Lock(ctx context.Context, path string, exclusive bool) (*os.File, error) {
	file, err := OpenPrivate(path, os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			file.Close()
			return nil, err
		}
		locked, err := tryLock(file, exclusive)
		if err != nil {
			file.Close()
			return nil, err
		}
		if locked {
			return file, nil
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
}

// TryLock attempts a process lock without waiting. A nil file and nil error
// means another process owns the lock. Keep the file open for the lock's
// lifetime, and never unlink a lock file while other processes may use it.
func TryLock(path string, exclusive bool) (*os.File, error) {
	file, err := OpenPrivate(path, os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, err
	}
	locked, err := tryLock(file, exclusive)
	if err != nil || !locked {
		file.Close()
		return nil, err
	}
	return file, nil
}

func CreateTemp(dir, pattern string) (*os.File, error) {
	if err := EnsurePrivateDir(dir); err != nil {
		return nil, err
	}
	for range 10 {
		var id [12]byte
		if _, err := rand.Read(id[:]); err != nil {
			return nil, err
		}
		name := strings.Replace(pattern, "*", hex.EncodeToString(id[:]), 1)
		file, err := OpenPrivate(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE|os.O_EXCL)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		return file, err
	}
	return nil, errors.New("could not allocate a private temporary file")
}

package main

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/thezzisu/pkudisk-fuse/internal/diagnostic"
	"github.com/thezzisu/pkudisk-fuse/internal/vfs"
)

type mountOptions struct {
	mountpoint, root, cache               string
	readOnly, debug, verbose, allAccounts bool
	cacheLimit, partSize                  int64
	ttl, timeout, refreshInterval         time.Duration
	read                                  vfs.ReadOptions
	trace                                 *diagnostic.Logger
	background                            bool
	workerHandles                         string
	ready                                 func() error
	driverOptions                         []string
}

// Use the same byte syntax in all mount size flags, including numeric bytes.
type byteSize struct{ value *int64 }

func (b byteSize) String() string { return humanize.IBytes(uint64(*b.value)) }
func (b byteSize) Type() string   { return "size" }
func (b byteSize) Set(text string) error {
	if size, err := strconv.ParseInt(text, 10, 64); err == nil && size >= 0 {
		*b.value = size
		return nil
	}
	size, err := humanize.ParseBytes(text)
	if err != nil || size > math.MaxInt64 {
		return fmt.Errorf("invalid size %q; use bytes or units such as 4MiB and 10GiB", text)
	}
	*b.value = int64(size)
	return nil
}

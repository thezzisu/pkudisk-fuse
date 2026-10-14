package main

import (
	"context"
	"github.com/thezzisu/pkudisk-fuse/internal/anyshare"
	"github.com/thezzisu/pkudisk-fuse/internal/display"
	"github.com/thezzisu/pkudisk-fuse/internal/tui"
	"io"
	"time"
)

var ttyInput = tui.Input
var interactiveIO = tui.Interactive
var tuiText = display.Text

func runTUI(ctx context.Context, backend interface {
	Roots(context.Context) ([]anyshare.Entry, error)
	List(context.Context, string) ([]anyshare.Entry, error)
}, account, server string, timeout time.Duration, sizes sizeFormat, in io.Reader, out io.Writer) error {
	return tui.Run(ctx, backend, account, server, timeout, display.SizeFormat{Bytes: sizes.bytes}, in, out)
}

var pickRecipient = tui.PickRecipient

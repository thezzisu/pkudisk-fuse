//go:build android || ios || (!linux && !darwin && !windows)

package mount

import (
	"context"
	"io"
)

const PlatformHelp = "Filesystem mounting is unavailable on this platform."
const PlatformExample = ""
const PlatformPoint = ""

func CheckSupport() error { return ErrUnsupported }

func Run(context.Context, *Source, Options, io.Writer) error { return ErrUnsupported }

// Package display contains terminal-safe shared presentation primitives.
package display

import (
	"fmt"
	"github.com/dustin/go-humanize"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type SizeFormat struct{ Bytes bool }

func (s SizeFormat) Value(n int64) string {
	if n < 0 {
		return "-"
	}
	if s.Bytes {
		return strconv.FormatInt(n, 10)
	}
	return humanize.IBytes(uint64(n))
}
func (s SizeFormat) Label(n int64) string {
	if s.Bytes {
		return fmt.Sprintf("%d bytes", n)
	}
	return s.Value(n)
}
func Text(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '?'
		}
		return r
	}, value)
}
func Time(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format(time.RFC3339)
}

func Expiration(stamp *time.Time) string {
	if stamp == nil {
		return "never"
	}
	return stamp.Format(time.RFC3339)
}

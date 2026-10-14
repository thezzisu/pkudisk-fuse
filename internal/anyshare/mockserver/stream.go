package mockserver

import (
	"context"
	"io"
	"net/http"
	"time"
)

const streamChunk = 64 << 10

// Delay by bytes rather than Write calls: ServeContent may choose smaller copy
// buffers, while its ordinary Range and HEAD behavior should remain intact.
type delayedResponseWriter struct {
	http.ResponseWriter
	ctx   context.Context
	delay time.Duration
	sent  int64
}

func (w *delayedResponseWriter) Write(data []byte) (written int, err error) {
	for len(data) > 0 {
		if err := w.ctx.Err(); err != nil {
			return written, err
		}
		if w.sent > 0 && w.sent%streamChunk == 0 {
			timer := time.NewTimer(w.delay)
			select {
			case <-w.ctx.Done():
				timer.Stop()
				return written, w.ctx.Err()
			case <-timer.C:
			}
		}
		size := min(len(data), streamChunk-int(w.sent%streamChunk))
		n, err := w.ResponseWriter.Write(data[:size])
		written += n
		w.sent += int64(n)
		if err != nil {
			return written, err
		}
		if n != size {
			return written, io.ErrShortWrite
		}
		if err := http.NewResponseController(w.ResponseWriter).Flush(); err != nil {
			return written, err
		}
		data = data[n:]
	}
	return written, nil
}

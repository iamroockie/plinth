package plinth

import (
	"bufio"
	"errors"
	"io"
	"net"
	"net/http"
)

// ResponseWriter wraps an [http.ResponseWriter] and records the status and the
// size of the response. Middleware that inspects responses, such as RequestLog and
// Recover from the middleware package, uses it.
//
// It works with [http.ResponseController]: flushing and hijacking are passed to the
// wrapped writer, and other features are reached through Unwrap.
type ResponseWriter struct {
	http.ResponseWriter

	status      int
	size        int64
	wroteHeader bool
}

// NewResponseWriter returns a ResponseWriter that wraps w.
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w}
}

// Status returns the status of the response. Until a status is written it
// returns 200, which is what net/http sends when a handler writes nothing. After
// Hijack without a written status it returns 101.
func (w *ResponseWriter) Status() int {
	if !w.wroteHeader {
		return http.StatusOK
	}
	return w.status
}

// Size returns the number of body bytes written.
func (w *ResponseWriter) Size() int64 {
	return w.size
}

// WroteHeader reports whether the response has started: a final status was
// written, the body was written or flushed, or the connection was hijacked.
func (w *ResponseWriter) WroteHeader() bool {
	return w.wroteHeader
}

// WriteHeader writes the status to the wrapped writer. Only the first final
// status is recorded. Informational 1xx statuses, except 101 Switching Protocols,
// are passed on without being recorded.
func (w *ResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)

	if !w.wroteHeader && isFinalStatus(status) {
		w.status = status
		w.wroteHeader = true
	}
}

// Write writes b to the wrapped writer and counts the bytes written. Without an
// earlier status it records 200.
func (w *ResponseWriter) Write(b []byte) (int, error) {
	w.markWritten()
	n, err := w.ResponseWriter.Write(b)
	w.size += int64(n)
	return n, err
}

// Unwrap returns the wrapped writer. It is used by [http.ResponseController].
func (w *ResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// Flush sends buffered data to the client if the wrapped writer supports it.
// See [ResponseWriter.FlushError].
func (w *ResponseWriter) Flush() {
	_ = w.FlushError()
}

// FlushError sends buffered data to the client. It returns
// [http.ErrNotSupported] if the wrapped writer cannot flush.
func (w *ResponseWriter) FlushError() error {
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if !errors.Is(err, http.ErrNotSupported) {
		w.markWritten()
	}
	return err
}

// Hijack lets the caller take over the connection, see [http.Hijacker]. It
// returns [http.ErrNotSupported] if the wrapped writer does not support it.
func (w *ResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, buf, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err != nil {
		return nil, nil, err
	}
	if !w.wroteHeader {
		w.status = http.StatusSwitchingProtocols
		w.wroteHeader = true
	}
	return conn, buf, nil
}

// ReadFrom copies src to the response and counts the bytes copied. It uses the
// ReadFrom method of the wrapped writer when there is one, so that net/http can
// send files efficiently.
func (w *ResponseWriter) ReadFrom(src io.Reader) (int64, error) {
	w.markWritten()

	var (
		n   int64
		err error
	)
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err = rf.ReadFrom(src)
	} else {
		n, err = io.Copy(w.ResponseWriter, src)
	}
	w.size += n

	return n, err
}

func (w *ResponseWriter) markWritten() {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
}

func isFinalStatus(status int) bool {
	return status >= http.StatusOK || status == http.StatusSwitchingProtocols
}

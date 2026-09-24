package plinth

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type plainWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newPlainWriter() *plainWriter {
	return &plainWriter{header: http.Header{}}
}

func (w *plainWriter) Header() http.Header {
	return w.header
}

func (w *plainWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *plainWriter) WriteHeader(status int) {
	w.status = status
}

type readerFromWriter struct {
	*plainWriter

	used bool
}

func (w *readerFromWriter) ReadFrom(src io.Reader) (int64, error) {
	w.used = true
	return w.body.ReadFrom(src)
}

type hijackWriter struct {
	*plainWriter

	conn net.Conn
}

func (w *hijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := bufio.NewReadWriter(bufio.NewReader(w.conn), bufio.NewWriter(w.conn))
	return w.conn, rw, nil
}

func newHijackWriter(t *testing.T) *hijackWriter {
	t.Helper()

	server, client := net.Pipe()
	t.Cleanup(func() {
		_ = server.Close()
		_ = client.Close()
	})
	return &hijackWriter{plainWriter: newPlainWriter(), conn: server}
}

func assertState(t *testing.T, w *ResponseWriter, status int, size int64, wrote bool) {
	t.Helper()

	if w.Status() != status || w.Size() != size || w.WroteHeader() != wrote {
		t.Errorf("state = (status %d, size %d, wrote %v), want (%d, %d, %v)",
			w.Status(), w.Size(), w.WroteHeader(), status, size, wrote)
	}
}

func TestResponseWriterNothingWritten(t *testing.T) {
	w := NewResponseWriter(httptest.NewRecorder())

	assertState(t, w, http.StatusOK, 0, false)
}

func TestResponseWriterWriteHeaderAndWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	w := NewResponseWriter(rec)

	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte("hello"))
	_, _ = w.Write([]byte(" world"))

	assertState(t, w, http.StatusCreated, 11, true)
	if rec.Code != http.StatusCreated || rec.Body.String() != "hello world" {
		t.Errorf("recorder = %d %q, want 201 %q", rec.Code, rec.Body.String(), "hello world")
	}
}

func TestResponseWriterImplicitOK(t *testing.T) {
	w := NewResponseWriter(httptest.NewRecorder())

	_, _ = w.Write([]byte("x"))

	assertState(t, w, http.StatusOK, 1, true)
}

func TestResponseWriterKeepsFirstStatus(t *testing.T) {
	w := NewResponseWriter(httptest.NewRecorder())

	w.WriteHeader(http.StatusAccepted)
	w.WriteHeader(http.StatusInternalServerError)

	assertState(t, w, http.StatusAccepted, 0, true)
}

func TestResponseWriterInformationalStatus(t *testing.T) {
	inner := newPlainWriter()
	w := NewResponseWriter(inner)

	w.WriteHeader(http.StatusEarlyHints)
	assertState(t, w, http.StatusOK, 0, false)
	if inner.status != http.StatusEarlyHints {
		t.Errorf("inner status = %d, want %d", inner.status, http.StatusEarlyHints)
	}

	w.WriteHeader(http.StatusNotFound)
	assertState(t, w, http.StatusNotFound, 0, true)
}

func TestResponseWriterSwitchingProtocols(t *testing.T) {
	w := NewResponseWriter(newPlainWriter())

	w.WriteHeader(http.StatusSwitchingProtocols)

	assertState(t, w, http.StatusSwitchingProtocols, 0, true)
}

func TestResponseWriterReadFrom(t *testing.T) {
	t.Run("inner without ReaderFrom", func(t *testing.T) {
		inner := newPlainWriter()
		w := NewResponseWriter(inner)

		n, err := w.ReadFrom(strings.NewReader("hello"))
		if err != nil || n != 5 {
			t.Fatalf("ReadFrom = %d, %v, want 5, nil", n, err)
		}
		assertState(t, w, http.StatusOK, 5, true)
		if inner.body.String() != "hello" {
			t.Errorf("inner body = %q, want %q", inner.body.String(), "hello")
		}
	})

	t.Run("inner with ReaderFrom", func(t *testing.T) {
		inner := &readerFromWriter{plainWriter: newPlainWriter()}
		w := NewResponseWriter(inner)

		n, err := w.ReadFrom(strings.NewReader("hello"))
		if err != nil || n != 5 {
			t.Fatalf("ReadFrom = %d, %v, want 5, nil", n, err)
		}
		assertState(t, w, http.StatusOK, 5, true)
		if !inner.used {
			t.Error("inner ReadFrom was not used")
		}
	})
}

func TestResponseWriterFlush(t *testing.T) {
	t.Run("supported", func(t *testing.T) {
		rec := httptest.NewRecorder()
		w := NewResponseWriter(rec)

		w.Flush()

		assertState(t, w, http.StatusOK, 0, true)
		if !rec.Flushed {
			t.Error("inner writer was not flushed")
		}
	})

	t.Run("not supported", func(t *testing.T) {
		w := NewResponseWriter(newPlainWriter())

		if err := w.FlushError(); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("FlushError = %v, want %v", err, http.ErrNotSupported)
		}
		assertState(t, w, http.StatusOK, 0, false)
	})
}

func TestResponseWriterHijack(t *testing.T) {
	t.Run("before header", func(t *testing.T) {
		w := NewResponseWriter(newHijackWriter(t))

		if _, _, err := w.Hijack(); err != nil {
			t.Fatalf("Hijack: %v", err)
		}
		assertState(t, w, http.StatusSwitchingProtocols, 0, true)
	})

	t.Run("after header", func(t *testing.T) {
		w := NewResponseWriter(newHijackWriter(t))
		w.WriteHeader(http.StatusOK)

		if _, _, err := w.Hijack(); err != nil {
			t.Fatalf("Hijack: %v", err)
		}
		assertState(t, w, http.StatusOK, 0, true)
	})

	t.Run("not supported", func(t *testing.T) {
		w := NewResponseWriter(newPlainWriter())

		if _, _, err := w.Hijack(); !errors.Is(err, http.ErrNotSupported) {
			t.Errorf("Hijack error = %v, want %v", err, http.ErrNotSupported)
		}
		assertState(t, w, http.StatusOK, 0, false)
	})
}

func TestResponseWriterUnwrap(t *testing.T) {
	inner := newPlainWriter()
	w := NewResponseWriter(inner)

	if w.Unwrap() != inner {
		t.Error("Unwrap did not return the wrapped writer")
	}
}

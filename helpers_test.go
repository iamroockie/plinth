package plinth

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"
)

type errorBody struct {
	Error struct {
		Code    ErrorCode    `json:"code"`
		Message string       `json:"message"`
		Details ErrorDetails `json:"details"`
	} `json:"error"`
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) errorBody {
	t.Helper()

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body %q: %v", rec.Body.String(), err)
	}
	return body
}

func asAPIError(t *testing.T, err error) apiError {
	t.Helper()

	e, ok := errors.AsType[apiError](err)
	if !ok {
		t.Fatalf("error %v (%T) is not an apiError", err, err)
	}
	return e
}

func captureDefaultLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return &buf
}

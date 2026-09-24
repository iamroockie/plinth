package plinth

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"strings"
)

// DefaultBodyLimit is the maximum size of a request body, in bytes, that
// [ParseRequestJSON] reads unless [WithBodyLimit] or [WithoutBodyLimit] is used.
const DefaultBodyLimit int64 = 1 << 20

// ParseOption configures [ParseRequestJSON].
type ParseOption func(*parseConfig)

type parseConfig struct {
	bodyLimit int64
	jsonOpts  []json.Options
}

// WithBodyLimit sets the maximum size of the request body in bytes. It panics if
// n is not positive.
func WithBodyLimit(n int64) ParseOption {
	if n <= 0 {
		panic(fmt.Sprintf("plinth: body limit must be positive, got %d", n))
	}
	return func(c *parseConfig) {
		c.bodyLimit = n
	}
}

// WithoutBodyLimit removes the limit on the size of the request body. Use it only
// when clients are trusted or bodies are limited elsewhere.
func WithoutBodyLimit() ParseOption {
	return func(c *parseConfig) {
		c.bodyLimit = 0
	}
}

// WithJSONOptions adds options for decoding with encoding/json/v2, such as
// json.RejectUnknownMembers(true).
func WithJSONOptions(opts ...json.Options) ParseOption {
	return func(c *parseConfig) {
		c.jsonOpts = append(c.jsonOpts, opts...)
	}
}

// ParseRequestJSON decodes the JSON body of r into a value of type T. Unknown
// fields are ignored unless [WithJSONOptions] says otherwise.
//
// The returned errors are ready to be returned from a [ResponseFunc] or passed to
// [WriteError]:
//   - 415 [UnsupportedMediaTypeError] if Content-Type is not application/json or
//     another application/*+json type;
//   - 413 [RequestEntityTooLargeError] if the body is over the limit, see
//     [DefaultBodyLimit]. The response to it closes the connection, so the server
//     does not read the rest of the body;
//   - 400 [BadRequestError] if the body is not valid JSON for T.
func ParseRequestJSON[T any](r *http.Request, opts ...ParseOption) (T, error) {
	var zero T

	cfg := parseConfig{bodyLimit: DefaultBodyLimit}
	for _, opt := range opts {
		opt(&cfg)
	}

	mediaType, _, err := mime.ParseMediaType(r.Header.Get(HeaderContentType))
	if err != nil || !isJSONMediaType(mediaType) {
		if err == nil {
			err = fmt.Errorf("unsupported content type %q", mediaType)
		}
		return zero, UnsupportedMediaTypeError(err)
	}

	body := r.Body
	if cfg.bodyLimit > 0 {
		if r.ContentLength > cfg.bodyLimit {
			return zero, RequestEntityTooLargeError(&http.MaxBytesError{Limit: cfg.bodyLimit})
		}
		body = http.MaxBytesReader(nil, body, cfg.bodyLimit)
	}

	var payload T

	err = json.UnmarshalRead(body, &payload, cfg.jsonOpts...)
	if err != nil {
		responseErr := BadRequestError("", err)
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			responseErr = RequestEntityTooLargeError(err)
		}
		return zero, responseErr
	}

	return payload, nil
}

// WriteJSON writes payload as a JSON response with the given status. It sets
// Content-Type to application/json unless the header is already set.
//
// For 204, 205 and 304 it writes neither a body nor Content-Type; a non-nil
// payload is dropped and reported with [ReportError]. If the status is outside
// 200-599 or payload cannot be encoded, WriteJSON reports the problem and writes a
// 500 error instead.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) {
	writeJSON(w, r, status, payload, nil)
}

func writeJSON(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	payload any,
	header []headerOp,
) {
	if status < http.StatusOK || status > 599 {
		ReportError(r.Context(), fmt.Errorf("invalid status for JSON response: %d", status))
		status = http.StatusInternalServerError
		payload = errorEnvelope{internalError(nil)}
		header = nil
	}

	if isBodylessStatus(status) {
		if payload != nil {
			dropped := fmt.Errorf("payload dropped: status %d forbids a body", status)
			ReportError(r.Context(), dropped)
		}
		applyHeader(w.Header(), header)
		w.WriteHeader(status)
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		ReportError(r.Context(), fmt.Errorf("marshal response: %w", err))
		status = http.StatusInternalServerError
		data, _ = json.Marshal(errorEnvelope{internalError(nil)})
		header = nil
	}

	applyHeader(w.Header(), header)

	if w.Header().Get(HeaderContentType) == "" {
		w.Header().Set(HeaderContentType, MIMEApplicationJSON)
	}
	w.WriteHeader(status)
	if _, err = w.Write(data); err != nil {
		ReportError(r.Context(), fmt.Errorf("write response: %w", err))
	}
}

func isBodylessStatus(status int) bool {
	return status == http.StatusNoContent || status == http.StatusResetContent ||
		status == http.StatusNotModified
}

func isJSONMediaType(mediaType string) bool {
	if mediaType == MIMEApplicationJSON {
		return true
	}
	subtype, ok := strings.CutPrefix(mediaType, "application/")
	return ok && strings.HasSuffix(subtype, "+json")
}

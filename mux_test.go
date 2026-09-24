package plinth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONMux(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "item "+r.PathValue("id"))
	})
	mux.HandleFunc("GET /dir/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "dir")
	})
	h := JSONMux(mux)

	tests := []struct {
		name         string
		method       string
		target       string
		wantStatus   int
		wantBody     string
		wantCode     ErrorCode
		wantAllow    string
		wantLocation string
	}{
		{
			name:       "matched route with path value",
			method:     http.MethodGet,
			target:     "/items/42",
			wantStatus: http.StatusOK,
			wantBody:   "item 42",
		},
		{
			name:       "head on get route",
			method:     http.MethodHead,
			target:     "/items/42",
			wantStatus: http.StatusOK,
		},
		{
			name:       "not found",
			method:     http.MethodGet,
			target:     "/nope",
			wantStatus: http.StatusNotFound,
			wantCode:   CodeNotFound,
		},
		{
			name:       "method not allowed",
			method:     http.MethodDelete,
			target:     "/items/42",
			wantStatus: http.StatusMethodNotAllowed,
			wantCode:   CodeMethodNotAllowed,
			wantAllow:  "GET, HEAD",
		},
		{
			name:         "trailing slash redirect",
			method:       http.MethodGet,
			target:       "/dir",
			wantStatus:   http.StatusTemporaryRedirect,
			wantLocation: "/dir/",
		},
		{
			name:         "clean path redirect to matched route",
			method:       http.MethodGet,
			target:       "//items/7",
			wantStatus:   http.StatusTemporaryRedirect,
			wantLocation: "/items/7",
		},
		{
			name:         "clean path redirect without match",
			method:       http.MethodGet,
			target:       "/a/../nope",
			wantStatus:   http.StatusTemporaryRedirect,
			wantLocation: "/nope",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.target, nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, r)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantStatus, rec.Body)
			}
			if tc.wantCode != "" {
				if got := decodeErrorBody(t, rec).Error.Code; got != tc.wantCode {
					t.Errorf("code = %q, want %q", got, tc.wantCode)
				}
				if got := rec.Header().Get(HeaderContentType); got != MIMEApplicationJSON {
					t.Errorf("Content-Type = %q, want %q", got, MIMEApplicationJSON)
				}
			} else if tc.wantBody != "" && rec.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tc.wantBody)
			}
			if got := rec.Header().Get(HeaderAllow); got != tc.wantAllow {
				t.Errorf("Allow = %q, want %q", got, tc.wantAllow)
			}
			if got := rec.Header().Get("Location"); got != tc.wantLocation {
				t.Errorf("Location = %q, want %q", got, tc.wantLocation)
			}
		})
	}
}

func TestJSONMuxCatchAll(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	rec := httptest.NewRecorder()
	JSONMux(mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusTeapot)
	}
}

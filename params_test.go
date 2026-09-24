package plinth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"
)

func assertBadRequest(t *testing.T, err error, message string) {
	t.Helper()

	e := asAPIError(t, err)
	if e.status != http.StatusBadRequest || e.Message != message {
		t.Errorf("error = (%d, %q), want (%d, %q)",
			e.status, e.Message, http.StatusBadRequest, message)
	}
}

func TestPathInt(t *testing.T) {
	tests := []struct {
		value   string
		want    int64
		wantErr bool
	}{
		{value: "42", want: 42},
		{value: "-7", want: -7},
		{value: "0", want: 0},
		{value: "9223372036854775807", want: 9223372036854775807},
		{value: "9223372036854775808", wantErr: true},
		{value: "abc", wantErr: true},
		{value: "1.5", wantErr: true},
		{value: " 1", wantErr: true},
		{value: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("id", tc.value)

			got, err := PathInt(r, "id")
			if tc.wantErr {
				assertBadRequest(t, err, `invalid path parameter "id"`)
				return
			}
			if err != nil {
				t.Fatalf("PathInt: %v", err)
			}
			if got != tc.want {
				t.Errorf("PathInt = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestPathUUID(t *testing.T) {
	id := uuid.NewV7()

	tests := []struct {
		name    string
		value   string
		want    uuid.UUID
		wantErr bool
	}{
		{name: "canonical", value: id.String(), want: id},
		{name: "nil uuid", value: uuid.Nil().String(), want: uuid.Nil()},
		{name: "invalid", value: "not-a-uuid", wantErr: true},
		{name: "missing", value: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.SetPathValue("id", tc.value)

			got, err := PathUUID(r, "id")
			if tc.wantErr {
				assertBadRequest(t, err, `invalid path parameter "id"`)
				return
			}
			if err != nil {
				t.Fatalf("PathUUID: %v", err)
			}
			if got != tc.want {
				t.Errorf("PathUUID = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		want    int64
		wantErr bool
	}{
		{name: "missing uses default", target: "/", want: 10},
		{name: "empty uses default", target: "/?limit=", want: 10},
		{name: "value", target: "/?limit=25", want: 25},
		{name: "first of repeated", target: "/?limit=1&limit=2", want: 1},
		{name: "invalid", target: "/?limit=many", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)

			got, err := QueryInt(r, "limit", 10)
			if tc.wantErr {
				assertBadRequest(t, err, `invalid query parameter "limit"`)
				return
			}
			if err != nil {
				t.Fatalf("QueryInt: %v", err)
			}
			if got != tc.want {
				t.Errorf("QueryInt = %d, want %d", got, tc.want)
			}
		})
	}
}

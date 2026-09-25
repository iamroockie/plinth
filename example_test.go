package plinth_test

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/iamroockie/plinth"
)

func printResponse(rec *httptest.ResponseRecorder) {
	body := jsontext.Value(rec.Body.Bytes())
	if err := body.Indent(jsontext.WithIndent("  ")); err != nil {
		fmt.Println(rec.Code, rec.Body.String())
		return
	}
	fmt.Println(rec.Code, string(body))
}

func ExampleRespondJSON() {
	type createUser struct {
		Name string `json:"name"`
	}

	handler := plinth.RespondJSON(func(r *http.Request) (*plinth.Response, error) {
		in, err := plinth.ParseRequestJSON[createUser](r)
		if err != nil {
			return nil, err
		}
		if in.Name == "" {
			return nil, plinth.ValidationError(
				plinth.FieldViolation{Field: "name", Code: "required"},
			)
		}

		resp := plinth.NewResponse(http.StatusCreated, map[string]string{"name": in.Name})
		return resp.SetHeader("Location", "/users/1"), nil
	})

	for _, body := range []string{`{"name":"gopher"}`, `{}`, `{"name":`} {
		r := httptest.NewRequest(http.MethodPost, "/users", strings.NewReader(body))
		r.Header.Set(plinth.HeaderContentType, plinth.MIMEApplicationJSON)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		printResponse(rec)
	}

	// Output:
	// 201 {
	//   "name": "gopher"
	// }
	// 422 {
	//   "error": {
	//     "code": "validation_error",
	//     "message": "Validation failed",
	//     "details": [
	//       {
	//         "field": "name",
	//         "code": "required"
	//       }
	//     ]
	//   }
	// }
	// 400 {
	//   "error": {
	//     "code": "bad_request",
	//     "message": "Bad Request"
	//   }
	// }
}

func ExampleMatchViolations() {
	type createMonitor struct {
		URL             string `json:"url"`
		IntervalSeconds int    `json:"interval_seconds"`
	}

	// Sentinel errors of the domain validation.
	var (
		errUnsupportedScheme = errors.New("unsupported URL scheme")
		errIntervalRange     = errors.New("interval out of range")
	)
	validate := func(m createMonitor) error {
		var errs []error
		if !strings.HasPrefix(m.URL, "https://") {
			errs = append(errs, errUnsupportedScheme)
		}
		if m.IntervalSeconds < 10 || m.IntervalSeconds > 86400 {
			errs = append(errs, errIntervalRange)
		}
		return errors.Join(errs...)
	}

	// Violations are returned in the order of the rules.
	rules := []plinth.FieldRule{
		{
			Err:    errIntervalRange,
			Field:  "interval_seconds",
			Code:   "out_of_range",
			Params: plinth.Map{"min": 10, "max": 86400},
		},
		{Err: errUnsupportedScheme, Field: "url", Code: "unsupported_scheme"},
	}

	handler := plinth.RespondJSON(func(r *http.Request) (*plinth.Response, error) {
		in, err := plinth.ParseRequestJSON[createMonitor](r)
		if err != nil {
			return nil, err
		}
		if err := validate(in); err != nil {
			if v := plinth.MatchViolations(err, rules...); len(v) > 0 {
				return nil, plinth.ValidationError(v...)
			}
			return nil, err
		}
		return plinth.NewResponse(http.StatusCreated, in), nil
	})

	body := `{"url":"ftp://example.com","interval_seconds":5}`
	r := httptest.NewRequest(http.MethodPost, "/monitors", strings.NewReader(body))
	r.Header.Set(plinth.HeaderContentType, plinth.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)

	// The order of keys within params is not specified, so the response is
	// decoded and printed with fmt, which sorts them.
	var resp struct {
		Error struct {
			Code    plinth.ErrorCode        `json:"code"`
			Details []plinth.FieldViolation `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		panic(err)
	}
	fmt.Println(rec.Code, resp.Error.Code)
	for _, v := range resp.Error.Details {
		fmt.Println(v.Field, v.Code, v.Params)
	}

	// Output:
	// 422 validation_error
	// interval_seconds out_of_range map[max:86400 min:10]
	// url unsupported_scheme map[]
}

func ExampleNewError() {
	const codeOutOfStock plinth.ErrorCode = "out_of_stock"

	handler := plinth.RespondJSON(func(*http.Request) (*plinth.Response, error) {
		err := fmt.Errorf("reserve item: %w",
			plinth.NewError(http.StatusConflict, codeOutOfStock, "Item is out of stock", nil))
		return nil, err
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders", nil))
	printResponse(rec)

	// Output:
	// 409 {
	//   "error": {
	//     "code": "out_of_stock",
	//     "message": "Item is out of stock"
	//   }
	// }
}

func ExampleJSONMux() {
	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", plinth.RespondJSON(
		func(r *http.Request) (*plinth.Response, error) {
			id, err := plinth.PathInt(r, "id")
			if err != nil {
				return nil, err
			}
			return plinth.NewResponse(http.StatusOK, map[string]int64{"id": id}), nil
		},
	))
	handler := plinth.JSONMux(mux)

	for _, req := range []struct{ method, target string }{
		{http.MethodGet, "/users/42"},
		{http.MethodGet, "/posts"},
		{http.MethodDelete, "/users/42"},
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(req.method, req.target, nil))
		fmt.Println(req.method, req.target, rec.Code)
		if allow := rec.Header().Get(plinth.HeaderAllow); allow != "" {
			fmt.Println("Allow:", allow)
		}
		fmt.Println(rec.Body.String())
	}

	// Output:
	// GET /users/42 200
	// {"id":42}
	// GET /posts 404
	// {"error":{"code":"not_found","message":"Not Found"}}
	// DELETE /users/42 405
	// Allow: GET, HEAD
	// {"error":{"code":"method_not_allowed","message":"Method Not Allowed"}}
}

func ExampleReadyz() {
	redisDown := false
	handler := plinth.Readyz(time.Second, map[string]plinth.CheckFunc{
		// Methods can be passed as is: pool.Ping for a *pgxpool.Pool,
		// db.PingContext for a *sql.DB.
		"postgres": func(context.Context) error { return nil },
		"redis": func(context.Context) error {
			if redisDown {
				return errors.New("connection refused")
			}
			return nil
		},
	})

	for _, down := range []bool{false, true} {
		redisDown = down

		// The ErrorLog middleware does this for every request and logs the result.
		r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		r = r.WithContext(plinth.WithErrorReporter(r.Context()))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		fmt.Println(rec.Code, rec.Body.String())
		if err := plinth.FlushReportedError(r.Context()); err != nil {
			fmt.Println("logged:", err)
		}
	}

	// Output:
	// 200 {"status":"ready"}
	// 503 {"error":{"code":"service_unavailable","message":"Service Unavailable"}}
	// logged: Service Unavailable: redis: connection refused
}

func ExampleClientIPResolver() {
	resolver, err := plinth.NewClientIPResolver().WithTrustedProxies("10.0.0.0/8")
	if err != nil {
		panic(err)
	}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.2:51234"
	// 198.51.100.1 was sent by the client itself and cannot be trusted.
	// 203.0.113.7 was added by the proxy at 10.0.0.1, which the client connected to.
	r.Header.Set(plinth.HeaderXForwardedFor, "198.51.100.1, 203.0.113.7, 10.0.0.1")

	ip, err := resolver.Resolve(r)
	fmt.Println(ip, err)

	// Output: 203.0.113.7 <nil>
}

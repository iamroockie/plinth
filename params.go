package plinth

import (
	"fmt"
	"net/http"
	"strconv"
	"uuid"
)

// PathInt returns the path wildcard name of r, see [http.Request.PathValue], as
// a base-10 int64. A missing or invalid value gives a 400 [BadRequestError] with
// the message: invalid path parameter "name".
func PathInt(r *http.Request, name string) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, invalidParamError("path", name, err)
	}
	return n, nil
}

// PathUUID returns the path wildcard name of r, see [http.Request.PathValue],
// parsed with [uuid.Parse]. A missing or invalid value gives a 400
// [BadRequestError] with the message: invalid path parameter "name".
func PathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil(), invalidParamError("path", name, err)
	}
	return id, nil
}

// QueryInt returns the first value of the query parameter name as a base-10
// int64, or def if the parameter is missing or empty. An invalid value gives a
// 400 [BadRequestError] with the message: invalid query parameter "name".
func QueryInt(r *http.Request, name string, def int64) (int64, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return def, nil
	}

	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, invalidParamError("query", name, err)
	}
	return n, nil
}

func invalidParamError(kind, name string, err error) error {
	return BadRequestError(fmt.Sprintf("invalid %s parameter %q", kind, name), err)
}

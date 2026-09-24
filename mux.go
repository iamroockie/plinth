package plinth

import "net/http"

type probeWriter struct {
	header http.Header
	status int
}

// JSONMux wraps mux so that a request with no matching route gets a 404
// [NotFoundError] and a request with a wrong method gets a 405
// [MethodNotAllowedError] with the Allow header, both as JSON instead of the plain
// text that [http.ServeMux] writes. Redirects done by mux, such as adding a
// trailing slash or cleaning the path, are left as they are.
//
// A request with a matching route is looked up in mux twice: once to find out
// that the route exists and once to serve it.
func JSONMux(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		probe := &probeWriter{header: http.Header{}}
		h.ServeHTTP(probe, r)

		switch probe.status {
		case http.StatusNotFound:
			WriteError(w, r, NotFoundError(nil))
		case http.StatusMethodNotAllowed:
			w.Header().Set(HeaderAllow, probe.header.Get(HeaderAllow))
			WriteError(w, r, MethodNotAllowedError(nil))
		default:
			mux.ServeHTTP(w, r)
		}
	})
}

func (p *probeWriter) Header() http.Header {
	return p.header
}

func (p *probeWriter) Write(b []byte) (int, error) {
	p.WriteHeader(http.StatusOK)
	return len(b), nil
}

func (p *probeWriter) WriteHeader(status int) {
	if p.status == 0 {
		p.status = status
	}
}

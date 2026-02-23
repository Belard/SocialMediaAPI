package middleware

import (
	"net/http"

	"github.com/gorilla/mux"
)

// BodyLimit returns gorilla/mux middleware that caps the request body at
// maxBytes. When the limit is exceeded, any further Read on the body returns
// an error and the server can respond with 413 Request Entity Too Large.
//
// This uses http.MaxBytesReader, which is the standard-library mechanism for
// limiting request bodies. It works transparently with json.NewDecoder,
// io.ReadAll, r.ParseForm, etc.
//
// Apply this globally on the router for a sensible default (e.g. 1 MB), and
// use BodyLimitHandler on specific routes that need a higher limit (e.g.
// file uploads).
func BodyLimit(maxBytes int64) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// BodyLimitHandler wraps a single http.HandlerFunc with an overridden body
// size limit. Useful for routes that need a higher (or lower) cap than the
// global default — for example, file upload endpoints.
func BodyLimitHandler(maxBytes int64, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next(w, r)
	}
}

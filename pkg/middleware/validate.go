package middleware

import "net/http"

// RequestBodyLimit returns a middleware that limits the request body size.
// Requests exceeding maxBytes will fail with http.ErrContentLength or similar.
func RequestBodyLimit(maxBytes int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

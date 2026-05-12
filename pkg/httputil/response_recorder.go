package httputil

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

// ResponseRecorder wraps http.ResponseWriter and records response status and body size.
// It forwards http.Flusher and http.Hijacker to preserve SSE and WebSocket behavior
// through middleware stacks.
type ResponseRecorder struct {
	http.ResponseWriter
	statusCode   int
	wroteHeader  bool
	bytesWritten int64
}

var (
	_ http.Flusher  = (*ResponseRecorder)(nil)
	_ http.Hijacker = (*ResponseRecorder)(nil)
)

// NewResponseRecorder creates a response recorder with default status 200 OK.
func NewResponseRecorder(w http.ResponseWriter) *ResponseRecorder {
	return &ResponseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
}

// StatusCode returns the first status code written, or 200 before any write.
func (r *ResponseRecorder) StatusCode() int { return r.statusCode }

// Written reports whether the response header has been written.
func (r *ResponseRecorder) Written() bool { return r.wroteHeader }

// BytesWritten returns the number of response body bytes successfully written.
func (r *ResponseRecorder) BytesWritten() int64 { return r.bytesWritten }

// WriteHeader records and forwards the first response status code.
func (r *ResponseRecorder) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.statusCode = code
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(code)
}

// Write records body bytes and implicitly writes 200 OK if needed.
func (r *ResponseRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytesWritten += int64(n)
	return n, err
}

// Flush forwards flushing to the underlying writer when supported.
func (r *ResponseRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack forwards WebSocket/TCP hijacking when supported by the underlying writer.
func (r *ResponseRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not implement http.Hijacker")
}

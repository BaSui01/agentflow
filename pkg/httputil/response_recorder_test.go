package httputil

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type trackingWriter struct {
	*httptest.ResponseRecorder
	flushed  bool
	hijacked bool
}

func (w *trackingWriter) Flush() {
	w.flushed = true
	w.ResponseRecorder.Flush()
}

func (w *trackingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	w.hijacked = true
	return nil, nil, nil
}

func TestResponseRecorderRecordsStatusAndBytes(t *testing.T) {
	inner := httptest.NewRecorder()
	recorder := NewResponseRecorder(inner)

	assert.Equal(t, http.StatusOK, recorder.StatusCode())
	assert.False(t, recorder.Written())
	assert.Equal(t, int64(0), recorder.BytesWritten())

	recorder.WriteHeader(http.StatusCreated)
	recorder.WriteHeader(http.StatusTeapot)
	n, err := recorder.Write([]byte("hello"))

	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, http.StatusCreated, recorder.StatusCode())
	assert.True(t, recorder.Written())
	assert.Equal(t, int64(5), recorder.BytesWritten())
	assert.Equal(t, http.StatusCreated, inner.Code)
}

func TestResponseRecorderWriteImplicitlyWritesOK(t *testing.T) {
	inner := httptest.NewRecorder()
	recorder := NewResponseRecorder(inner)

	n, err := recorder.Write([]byte("ok"))

	require.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.Equal(t, http.StatusOK, recorder.StatusCode())
	assert.True(t, recorder.Written())
	assert.Equal(t, int64(2), recorder.BytesWritten())
}

func TestResponseRecorderForwardsFlushAndHijack(t *testing.T) {
	inner := &trackingWriter{ResponseRecorder: httptest.NewRecorder()}
	recorder := NewResponseRecorder(inner)

	recorder.Flush()
	_, _, err := recorder.Hijack()

	require.NoError(t, err)
	assert.True(t, inner.flushed)
	assert.True(t, inner.hijacked)
}

func TestResponseRecorderHijackUnsupported(t *testing.T) {
	recorder := NewResponseRecorder(httptest.NewRecorder())

	_, _, err := recorder.Hijack()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not implement http.Hijacker")
}

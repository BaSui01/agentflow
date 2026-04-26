package common

import (
	"io"
)

// DrainAndClose reads all remaining data from a ReadCloser and closes it.
// This ensures the connection can be reused, which is important for HTTP/1.1 keep-alive.
// Errors during drain or close are returned.
func DrainAndClose(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	_, drainErr := io.Copy(io.Discard, body)
	closeErr := body.Close()
	if drainErr != nil {
		return drainErr
	}
	return closeErr
}

// ReadAndClose reads all data from a ReadCloser and closes it.
// Returns the data and any error that occurred during read or close.
func ReadAndClose(body io.ReadCloser) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	return io.ReadAll(body)
}

// SafeClose closes a ReadCloser and logs any error to the provided logger.
// Use this when you want to close a response body but don't want to handle the error.
// Returns true if close was successful, false otherwise.
func SafeClose(body io.ReadCloser) bool {
	if body == nil {
		return true
	}
	return body.Close() == nil
}

// MustClose closes a ReadCloser and panics on error.
// Use only in tests or when close failure is unrecoverable.
func MustClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		panic(err)
	}
}

// LimitReadAndClose reads up to n bytes from a ReadCloser and closes it.
// Returns io.ErrUnexpectedEOF if the body exceeds the limit.
func LimitReadAndClose(body io.ReadCloser, n int64) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	defer body.Close()
	return io.ReadAll(io.LimitReader(body, n))
}

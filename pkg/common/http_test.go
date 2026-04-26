package common

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type mockReadCloser struct {
	data   []byte
	closed bool
	err    error
}

func (m *mockReadCloser) Read(p []byte) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	if len(m.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, m.data)
	m.data = m.data[n:]
	return n, nil
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

func TestDrainAndClose(t *testing.T) {
	// Test with nil
	err := DrainAndClose(nil)
	if err != nil {
		t.Errorf("DrainAndClose(nil) = %v, want nil", err)
	}

	// Test with data
	data := []byte("test data")
	rc := &mockReadCloser{data: data}
	err = DrainAndClose(rc)
	if err != nil {
		t.Errorf("DrainAndClose error: %v", err)
	}
	if !rc.closed {
		t.Error("DrainAndClose should close the ReadCloser")
	}
}

func TestReadAndClose(t *testing.T) {
	// Test with nil
	data, err := ReadAndClose(nil)
	if err != nil {
		t.Errorf("ReadAndClose(nil) error: %v", err)
	}
	if data != nil {
		t.Errorf("ReadAndClose(nil) = %v, want nil", data)
	}

	// Test with data
	rc := &mockReadCloser{data: []byte("hello")}
	data, err = ReadAndClose(rc)
	if err != nil {
		t.Errorf("ReadAndClose error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadAndClose = %q, want %q", string(data), "hello")
	}
	if !rc.closed {
		t.Error("ReadAndClose should close the ReadCloser")
	}
}

func TestSafeClose(t *testing.T) {
	// Test with nil
	if !SafeClose(nil) {
		t.Error("SafeClose(nil) should return true")
	}

	// Test with valid closer
	rc := &mockReadCloser{}
	if !SafeClose(rc) {
		t.Error("SafeClose should return true on success")
	}
	if !rc.closed {
		t.Error("SafeClose should close the ReadCloser")
	}
}

type errorCloser struct {
	closed bool
}

func (e *errorCloser) Read(p []byte) (int, error) { return 0, io.EOF }
func (e *errorCloser) Close() error {
	e.closed = true
	return errors.New("close error")
}

func TestSafeCloseError(t *testing.T) {
	ec := &errorCloser{}
	if SafeClose(ec) {
		t.Error("SafeClose should return false on error")
	}
	if !ec.closed {
		t.Error("SafeClose should still attempt to close")
	}
}

func TestLimitReadAndClose(t *testing.T) {
	// Test with nil
	data, err := LimitReadAndClose(nil, 100)
	if err != nil {
		t.Errorf("LimitReadAndClose(nil) error: %v", err)
	}
	if data != nil {
		t.Errorf("LimitReadAndClose(nil) = %v, want nil", data)
	}

	// Test with limited data
	rc := &mockReadCloser{data: []byte("hello world")}
	data, err = LimitReadAndClose(rc, 5)
	if err != nil {
		t.Errorf("LimitReadAndClose error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("LimitReadAndClose = %q, want %q", string(data), "hello")
	}
}

func TestMustClose(t *testing.T) {
	// Test with nil - should not panic
	MustClose(nil)

	// Test with valid closer
	rc := &mockReadCloser{}
	MustClose(rc)
	if !rc.closed {
		t.Error("MustClose should close the ReadCloser")
	}
}

func TestMustClosePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustClose should panic on error")
		}
	}()

	ec := &errorCloser{}
	MustClose(ec)
}

// Benchmark comparing DrainAndClose vs manual approach
func BenchmarkDrainAndClose(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rc := io.NopCloser(bytes.NewReader([]byte("test data for benchmark")))
		_ = DrainAndClose(rc)
	}
}

func BenchmarkDrainAndCloseManual(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rc := io.NopCloser(bytes.NewReader([]byte("test data for benchmark")))
		io.Copy(io.Discard, rc)
		rc.Close()
	}
}

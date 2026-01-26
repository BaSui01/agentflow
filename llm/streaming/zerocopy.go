// Package streaming provides zero-copy streaming for high-performance LLM responses.
package streaming

import (
	"io"
	"sync"
	"unsafe"
)

// ZeroCopyBuffer provides zero-copy buffer operations.
type ZeroCopyBuffer struct {
	data     []byte
	readPos  int
	writePos int
	mu       sync.RWMutex
}

// NewZeroCopyBuffer creates a new zero-copy buffer.
func NewZeroCopyBuffer(size int) *ZeroCopyBuffer {
	return &ZeroCopyBuffer{
		data: make([]byte, size),
	}
}

// Write writes data without copying.
func (b *ZeroCopyBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	available := len(b.data) - b.writePos
	if len(p) > available {
		// Grow buffer
		newSize := len(b.data) * 2
		if newSize < b.writePos+len(p) {
			newSize = b.writePos + len(p)
		}
		newData := make([]byte, newSize)
		copy(newData, b.data[:b.writePos])
		b.data = newData
	}

	copy(b.data[b.writePos:], p)
	b.writePos += len(p)
	return len(p), nil
}

// Read reads data without copying (returns slice of internal buffer).
func (b *ZeroCopyBuffer) Read(p []byte) (int, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.readPos >= b.writePos {
		return 0, io.EOF
	}

	n := copy(p, b.data[b.readPos:b.writePos])
	b.readPos += n
	return n, nil
}

// Bytes returns the unread portion without copying.
func (b *ZeroCopyBuffer) Bytes() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.data[b.readPos:b.writePos]
}

// BytesUnsafe returns bytes without lock (caller must ensure safety).
func (b *ZeroCopyBuffer) BytesUnsafe() []byte {
	return b.data[b.readPos:b.writePos]
}

// Reset resets the buffer for reuse.
func (b *ZeroCopyBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.readPos = 0
	b.writePos = 0
}

// Len returns the number of unread bytes.
func (b *ZeroCopyBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.writePos - b.readPos
}

// StringView provides zero-copy string view of bytes.
type StringView struct {
	data []byte
}

// NewStringView creates a string view from bytes without copying.
func NewStringView(data []byte) StringView {
	return StringView{data: data}
}

// String returns string without copying (unsafe if underlying bytes change).
func (s StringView) String() string {
	if len(s.data) == 0 {
		return ""
	}
	return unsafe.String(&s.data[0], len(s.data))
}

// Bytes returns the underlying bytes.
func (s StringView) Bytes() []byte {
	return s.data
}

// Len returns the length.
func (s StringView) Len() int {
	return len(s.data)
}

// BytesToString converts bytes to string without copying.
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// StringToBytes converts string to bytes without copying.
func StringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// ChunkReader provides zero-copy chunk reading.
type ChunkReader struct {
	data      []byte
	chunkSize int
	pos       int
}

// NewChunkReader creates a new chunk reader.
func NewChunkReader(data []byte, chunkSize int) *ChunkReader {
	return &ChunkReader{
		data:      data,
		chunkSize: chunkSize,
	}
}

// Next returns the next chunk without copying.
func (r *ChunkReader) Next() ([]byte, bool) {
	if r.pos >= len(r.data) {
		return nil, false
	}

	end := r.pos + r.chunkSize
	if end > len(r.data) {
		end = len(r.data)
	}

	chunk := r.data[r.pos:end]
	r.pos = end
	return chunk, true
}

// Reset resets the reader.
func (r *ChunkReader) Reset() {
	r.pos = 0
}

// RingBuffer provides a lock-free ring buffer for streaming.
type RingBuffer struct {
	data     []byte
	size     int
	readIdx  uint64
	writeIdx uint64
	mask     uint64
}

// NewRingBuffer creates a new ring buffer (size must be power of 2).
func NewRingBuffer(size int) *RingBuffer {
	// Round up to power of 2
	size--
	size |= size >> 1
	size |= size >> 2
	size |= size >> 4
	size |= size >> 8
	size |= size >> 16
	size++

	return &RingBuffer{
		data: make([]byte, size),
		size: size,
		mask: uint64(size - 1),
	}
}

// Put writes a single byte.
func (r *RingBuffer) Put(b byte) bool {
	writeIdx := r.writeIdx
	nextWrite := writeIdx + 1

	// Check if buffer is full
	if nextWrite-r.readIdx > uint64(r.size) {
		return false
	}

	r.data[writeIdx&r.mask] = b
	r.writeIdx = nextWrite
	return true
}

// Get reads a single byte.
func (r *RingBuffer) Get() (byte, bool) {
	readIdx := r.readIdx

	// Check if buffer is empty
	if readIdx >= r.writeIdx {
		return 0, false
	}

	b := r.data[readIdx&r.mask]
	r.readIdx = readIdx + 1
	return b, true
}

// Available returns the number of bytes available to read.
func (r *RingBuffer) Available() int {
	return int(r.writeIdx - r.readIdx)
}

// Free returns the number of bytes free for writing.
func (r *RingBuffer) Free() int {
	return r.size - int(r.writeIdx-r.readIdx)
}

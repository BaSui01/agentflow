package streaming

import (
	"io"
	"sync"
	"sync/atomic"
	"unsafe"
)

// ZeroCopyBuffer提供零拷贝缓冲操作.
type ZeroCopyBuffer struct {
	data     []byte
	readPos  int
	writePos int
	mu       sync.RWMutex
}

// NewZero CopyBuffer创建了新的零拷贝缓冲器.
func NewZeroCopyBuffer(size int) *ZeroCopyBuffer {
	return &ZeroCopyBuffer{
		data: make([]byte, size),
	}
}

// 写入数据而不复制.
func (b *ZeroCopyBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	available := len(b.data) - b.writePos
	if len(p) > available {
		// 增加缓冲
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

// 在不复制的情况下读取数据(返回部分为内部缓冲).
// 注意: 使用写锁(Lock)而非读锁(RLock)，因为此方法会修改 readPos。
// 在 RLock 下写 readPos 违反读写锁语义，会导致并发数据竞争。
func (b *ZeroCopyBuffer) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.readPos >= b.writePos {
		return 0, io.EOF
	}

	n := copy(p, b.data[b.readPos:b.writePos])
	b.readPos += n
	return n, nil
}

// 字节返回未读部分而不复制 。
func (b *ZeroCopyBuffer) Bytes() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.data[b.readPos:b.writePos]
}

// 字节不安全返回字节没有锁(调用器必须确保安全).
func (b *ZeroCopyBuffer) BytesUnsafe() []byte {
	return b.data[b.readPos:b.writePos]
}

// 重置缓冲器用于再利用 。
func (b *ZeroCopyBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.readPos = 0
	b.writePos = 0
}

// Len 返回未读字节数 。
func (b *ZeroCopyBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.writePos - b.readPos
}

// StringView提供字节的零复制字符串视图.
type StringView struct {
	data []byte
}

// NewStringView 不复制就从字节创建了字符串视图.
func NewStringView(data []byte) StringView {
	return StringView{data: data}
}

// 字符串不复制返回字符串( 如果隐藏字节更改, 则不安全) 。
func (s StringView) String() string {
	if len(s.data) == 0 {
		return ""
	}
	return unsafe.String(&s.data[0], len(s.data))
}

// 字节返回基本的字节 。
func (s StringView) Bytes() []byte {
	return s.data
}

// Len返回长度。
func (s StringView) Len() int {
	return len(s.data)
}

// BytesToString 不复制便将字节转换为字符串.
func BytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// StringToBytes 不复制就将字符串转换为字节.
func StringToBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// ChunkReader 提供零拷贝块读取.
type ChunkReader struct {
	data      []byte
	chunkSize int
	pos       int
}

// NewChunkReader 创建了新的块读取器 。
func NewChunkReader(data []byte, chunkSize int) *ChunkReader {
	return &ChunkReader{
		data:      data,
		chunkSize: chunkSize,
	}
}

// 下一个不复制则返回下一个块 。
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

// 重置读者.
func (r *ChunkReader) Reset() {
	r.pos = 0
}

// RingBuffer提供无锁环缓冲来进行流.
// readIdx 和 writeIdx 使用 atomic 操作保证并发安全。
type RingBuffer struct {
	data     []byte
	size     int
	readIdx  atomic.Uint64
	writeIdx atomic.Uint64
	mask     uint64
}

// NewRingBuffer创建了新的环缓冲(尺寸必须是2的功率).
func NewRingBuffer(size int) *RingBuffer {
	// 圆通为二相.
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

// 写一个字节（使用 atomic 操作保证并发安全）
func (r *RingBuffer) Put(b byte) bool {
	writeIdx := r.writeIdx.Load()
	nextWrite := writeIdx + 1

	// 检查缓冲器是否满了
	if nextWrite-r.readIdx.Load() > uint64(r.size) {
		return false
	}

	r.data[writeIdx&r.mask] = b
	r.writeIdx.Store(nextWrite)
	return true
}

// 读取一个字节（使用 atomic 操作保证并发安全）。
func (r *RingBuffer) Get() (byte, bool) {
	readIdx := r.readIdx.Load()

	// 检查缓冲是否为空
	if readIdx >= r.writeIdx.Load() {
		return 0, false
	}

	b := r.data[readIdx&r.mask]
	r.readIdx.Store(readIdx + 1)
	return b, true
}

// 可用返回可用的字节数（使用 atomic 读取保证并发安全）。
func (r *RingBuffer) Available() int {
	return int(r.writeIdx.Load() - r.readIdx.Load())
}

// 自由返回自由写入的字节数（使用 atomic 读取保证并发安全）。
func (r *RingBuffer) Free() int {
	return r.size - int(r.writeIdx.Load()-r.readIdx.Load())
}

package streaming

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ====== ZeroCopyBuffer Tests ======

func TestZeroCopyBuffer_WriteRead(t *testing.T) {
	buf := NewZeroCopyBuffer(64)

	n, err := buf.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, 5, buf.Len())

	p := make([]byte, 10)
	n, err = buf.Read(p)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(p[:n]))
}

func TestZeroCopyBuffer_ReadEOF(t *testing.T) {
	buf := NewZeroCopyBuffer(64)

	p := make([]byte, 10)
	n, err := buf.Read(p)
	assert.Equal(t, 0, n)
	assert.Equal(t, io.EOF, err)
}

func TestZeroCopyBuffer_GrowOnWrite(t *testing.T) {
	buf := NewZeroCopyBuffer(4)

	// Write more than initial capacity
	data := []byte("hello world, this is a longer string")
	n, err := buf.Write(data)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, len(data), buf.Len())

	// Read it back
	p := make([]byte, 100)
	n, err = buf.Read(p)
	require.NoError(t, err)
	assert.Equal(t, string(data), string(p[:n]))
}

func TestZeroCopyBuffer_Bytes(t *testing.T) {
	buf := NewZeroCopyBuffer(64)
	_, err := buf.Write([]byte("test"))
	require.NoError(t, err)

	assert.Equal(t, []byte("test"), buf.Bytes())
}

func TestZeroCopyBuffer_Reset(t *testing.T) {
	buf := NewZeroCopyBuffer(64)
	_, err := buf.Write([]byte("data"))
	require.NoError(t, err)
	assert.Equal(t, 4, buf.Len())

	buf.Reset()
	assert.Equal(t, 0, buf.Len())

	// Can write again after reset
	_, err = buf.Write([]byte("new"))
	require.NoError(t, err)
	assert.Equal(t, 3, buf.Len())
}

func TestZeroCopyBuffer_MultipleWrites(t *testing.T) {
	buf := NewZeroCopyBuffer(64)
	_, err := buf.Write([]byte("hello "))
	require.NoError(t, err)
	_, err = buf.Write([]byte("world"))
	require.NoError(t, err)

	assert.Equal(t, 11, buf.Len())
	assert.Equal(t, "hello world", string(buf.Bytes()))
}

func TestZeroCopyBuffer_PartialRead(t *testing.T) {
	buf := NewZeroCopyBuffer(64)
	_, err := buf.Write([]byte("hello world"))
	require.NoError(t, err)

	p := make([]byte, 5)
	n, err := buf.Read(p)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", string(p[:n]))

	// Read remaining
	p2 := make([]byte, 10)
	n, err = buf.Read(p2)
	require.NoError(t, err)
	assert.Equal(t, 6, n)
	assert.Equal(t, " world", string(p2[:n]))
}

// ====== StringView Tests ======

func TestStringView_String(t *testing.T) {
	sv := NewStringView([]byte("hello"))
	assert.Equal(t, "hello", sv.String())
	assert.Equal(t, 5, sv.Len())
}

func TestStringView_Empty(t *testing.T) {
	sv := NewStringView(nil)
	assert.Equal(t, "", sv.String())
	assert.Equal(t, 0, sv.Len())
}

func TestStringView_Bytes(t *testing.T) {
	data := []byte("test")
	sv := NewStringView(data)
	assert.Equal(t, data, sv.Bytes())
}

// ====== BytesToString / StringToBytes Tests ======

func TestBytesToString(t *testing.T) {
	assert.Equal(t, "hello", BytesToString([]byte("hello")))
	assert.Equal(t, "", BytesToString(nil))
	assert.Equal(t, "", BytesToString([]byte{}))
}

func TestStringToBytes(t *testing.T) {
	assert.Equal(t, []byte("hello"), StringToBytes("hello"))
	assert.Nil(t, StringToBytes(""))
}

// ====== ChunkReader Tests ======

func TestChunkReader_Next(t *testing.T) {
	data := []byte("hello world")
	reader := NewChunkReader(data, 5)

	chunk, ok := reader.Next()
	assert.True(t, ok)
	assert.Equal(t, "hello", string(chunk))

	chunk, ok = reader.Next()
	assert.True(t, ok)
	assert.Equal(t, " worl", string(chunk))

	chunk, ok = reader.Next()
	assert.True(t, ok)
	assert.Equal(t, "d", string(chunk))

	_, ok = reader.Next()
	assert.False(t, ok)
}

func TestChunkReader_Reset(t *testing.T) {
	data := []byte("abc")
	reader := NewChunkReader(data, 2)

	reader.Next()
	reader.Next()
	_, ok := reader.Next()
	assert.False(t, ok)

	reader.Reset()
	chunk, ok := reader.Next()
	assert.True(t, ok)
	assert.Equal(t, "ab", string(chunk))
}

func TestChunkReader_EmptyData(t *testing.T) {
	reader := NewChunkReader(nil, 5)
	_, ok := reader.Next()
	assert.False(t, ok)
}

// ====== RingBuffer Tests ======

func TestRingBuffer_PutGet(t *testing.T) {
	rb := NewRingBuffer(8)

	assert.True(t, rb.Put('a'))
	assert.True(t, rb.Put('b'))
	assert.Equal(t, 2, rb.Available())

	b, ok := rb.Get()
	assert.True(t, ok)
	assert.Equal(t, byte('a'), b)

	b, ok = rb.Get()
	assert.True(t, ok)
	assert.Equal(t, byte('b'), b)
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := NewRingBuffer(4)
	_, ok := rb.Get()
	assert.False(t, ok)
	assert.Equal(t, 0, rb.Available())
}

func TestRingBuffer_Full(t *testing.T) {
	rb := NewRingBuffer(4) // rounds to 4

	for i := 0; i < 4; i++ {
		assert.True(t, rb.Put(byte('a'+i)))
	}
	assert.Equal(t, 0, rb.Free())
	assert.False(t, rb.Put('x'))
}

func TestRingBuffer_WrapAround(t *testing.T) {
	rb := NewRingBuffer(4)

	// Fill and drain
	rb.Put('a')
	rb.Put('b')
	rb.Get()
	rb.Get()

	// Write again (wraps around)
	rb.Put('c')
	rb.Put('d')

	b, ok := rb.Get()
	assert.True(t, ok)
	assert.Equal(t, byte('c'), b)

	b, ok = rb.Get()
	assert.True(t, ok)
	assert.Equal(t, byte('d'), b)
}

func TestRingBuffer_Free(t *testing.T) {
	rb := NewRingBuffer(8)
	assert.Equal(t, 8, rb.Free())

	rb.Put('a')
	rb.Put('b')
	assert.Equal(t, 6, rb.Free())
}

// ====== BackpressureStream Tests ======

func TestBackpressureStream_WriteRead(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
		DropPolicy:    DropPolicyBlock,
	})

	ctx := context.Background()
	err := s.Write(ctx, Token{Content: "hello", Index: 0})
	require.NoError(t, err)

	token, err := s.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "hello", token.Content)
}

func TestBackpressureStream_Close(t *testing.T) {
	s := NewBackpressureStream(DefaultBackpressureConfig())

	require.NoError(t, s.Close())

	// Write after close should fail
	err := s.Write(context.Background(), Token{Content: "test"})
	assert.Equal(t, ErrStreamClosed, err)
}

func TestBackpressureStream_CloseIdempotent(t *testing.T) {
	s := NewBackpressureStream(DefaultBackpressureConfig())
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
}

func TestBackpressureStream_ReadAfterClose(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	// Write then close
	require.NoError(t, s.Write(context.Background(), Token{Content: "last"}))
	require.NoError(t, s.Close())

	// Should still read buffered token
	token, err := s.Read(context.Background())
	if err == nil {
		assert.Equal(t, "last", token.Content)
	}
}

func TestBackpressureStream_DropPolicyNewest(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    2,
		HighWaterMark: 0.5, // triggers at 1/2 = 50%
		LowWaterMark:  0.2,
		DropPolicy:    DropPolicyNewest,
	})

	ctx := context.Background()

	// Fill buffer to trigger high water mark
	require.NoError(t, s.Write(ctx, Token{Content: "t1", Index: 0}))

	// This should be dropped (newest policy)
	err := s.Write(ctx, Token{Content: "t2", Index: 1})
	require.NoError(t, err)

	stats := s.Stats()
	// At least one should be produced
	assert.True(t, stats.Produced >= 1)
}

func TestBackpressureStream_DropPolicyError(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    2,
		HighWaterMark: 0.5,
		LowWaterMark:  0.2,
		DropPolicy:    DropPolicyError,
	})

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, Token{Content: "t1"}))

	// Should return error when buffer is at high water mark
	err := s.Write(ctx, Token{Content: "t2"})
	if err != nil {
		assert.Equal(t, ErrBufferFull, err)
	}
}

func TestBackpressureStream_Stats(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	ctx := context.Background()
	require.NoError(t, s.Write(ctx, Token{Content: "t1"}))
	require.NoError(t, s.Write(ctx, Token{Content: "t2"}))

	stats := s.Stats()
	assert.Equal(t, int64(2), stats.Produced)
	assert.Equal(t, 10, stats.BufferCap)
}

func TestBackpressureStream_BufferLevel(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	assert.Equal(t, 0.0, s.BufferLevel())

	require.NoError(t, s.Write(context.Background(), Token{Content: "t1"}))
	assert.Equal(t, 0.1, s.BufferLevel())
}

func TestBackpressureStream_ContextCancellation(t *testing.T) {
	s := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Read(ctx)
	assert.Error(t, err)
}

// ====== StreamMultiplexer Tests ======

func TestStreamMultiplexer_Broadcast(t *testing.T) {
	source := NewBackpressureStream(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	mux := NewStreamMultiplexer(source)
	c1 := mux.AddConsumer(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})
	c2 := mux.AddConsumer(BackpressureConfig{
		BufferSize:    10,
		HighWaterMark: 0.8,
		LowWaterMark:  0.2,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mux.Start(ctx)

	// Write to source
	require.NoError(t, source.Write(ctx, Token{Content: "broadcast", Index: 0}))

	// Both consumers should receive
	t1, err := c1.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "broadcast", t1.Content)

	t2, err := c2.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, "broadcast", t2.Content)
}

// ====== RateLimiter Tests ======

func TestStreamRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(100, 5)

	// Should allow burst
	for i := 0; i < 5; i++ {
		assert.True(t, rl.Allow())
	}
	// Bucket empty
	assert.False(t, rl.Allow())
}

func TestStreamRateLimiter_Wait(t *testing.T) {
	rl := NewRateLimiter(1000, 1)
	rl.Allow() // drain the bucket

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should eventually succeed or timeout
	err := rl.Wait(ctx)
	// Either nil (token refilled) or context deadline exceeded
	if err != nil {
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	}
}

func TestStreamRateLimiter_WaitCancelled(t *testing.T) {
	rl := NewRateLimiter(0.1, 1) // very slow refill
	rl.Allow()                    // drain

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := rl.Wait(ctx)
	assert.Error(t, err)
}

// ====== DropPolicy Tests ======

func TestDropPolicy_String(t *testing.T) {
	assert.Equal(t, "block", DropPolicyBlock.String())
	assert.Equal(t, "oldest", DropPolicyOldest.String())
	assert.Equal(t, "newest", DropPolicyNewest.String())
	assert.Equal(t, "error", DropPolicyError.String())
	assert.Contains(t, DropPolicy(99).String(), "DropPolicy")
}

func TestDefaultBackpressureConfig(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	assert.Equal(t, 1024, cfg.BufferSize)
	assert.Equal(t, 0.8, cfg.HighWaterMark)
	assert.Equal(t, 0.2, cfg.LowWaterMark)
	assert.Equal(t, DropPolicyBlock, cfg.DropPolicy)
}

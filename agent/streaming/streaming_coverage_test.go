package streaming

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- StreamManager ---

func TestStreamManager_CreateGetClose_Coverage(t *testing.T) {
	mgr := NewStreamManager(zap.NewNop())
	conn := &mockConn{isAliveFn: func() bool { return true }}
	handler := &mockHandler{}

	stream := mgr.CreateStream(DefaultStreamConfig(), handler, conn, nil)
	assert.NotEmpty(t, stream.ID)

	got, ok := mgr.GetStream(stream.ID)
	assert.True(t, ok)
	assert.Equal(t, stream.ID, got.ID)

	err := mgr.CloseStream(stream.ID)
	assert.NoError(t, err)

	_, ok = mgr.GetStream(stream.ID)
	assert.False(t, ok)
}

func TestStreamManager_CloseNonexistent_Coverage(t *testing.T) {
	mgr := NewStreamManager(zap.NewNop())
	err := mgr.CloseStream("nonexistent")
	assert.NoError(t, err)
}

// --- AudioStreamAdapter ---

func TestAudioStreamAdapter_SendAudio_Coverage(t *testing.T) {
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	stream := NewBidirectionalStream(config, handler, conn, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	adapter := NewAudioStreamAdapter(stream, 16000, 1)

	t.Run("send without encoder", func(t *testing.T) {
		err := adapter.SendAudio([]byte{1, 2, 3})
		assert.NoError(t, err)
	})

	t.Run("send with encoder", func(t *testing.T) {
		adapter.encoder = &mockEncoder{fn: func(pcm []byte) ([]byte, error) {
			return append([]byte{0xFF}, pcm...), nil
		}}
		err := adapter.SendAudio([]byte{1, 2, 3})
		assert.NoError(t, err)
	})

	t.Run("send with encoder error", func(t *testing.T) {
		adapter.encoder = &mockEncoder{fn: func(pcm []byte) ([]byte, error) {
			return nil, errors.New("encode error")
		}}
		err := adapter.SendAudio([]byte{1, 2, 3})
		assert.Error(t, err)
	})

	cancel()
	stream.Close()
}

func TestAudioStreamAdapter_ReceiveAudio_Coverage(t *testing.T) {
	chunks := make(chan *StreamChunk, 10)
	chunks <- &StreamChunk{Type: StreamTypeAudio, Data: []byte{1, 2, 3}}
	chunks <- &StreamChunk{Type: StreamTypeText, Text: "skip me"}
	chunks <- &StreamChunk{Type: StreamTypeAudio, Data: []byte{4, 5, 6}}

	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case c := <-chunks:
				return c, nil
			}
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	stream := NewBidirectionalStream(config, handler, conn, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	adapter := NewAudioStreamAdapter(stream, 16000, 1)
	audioCh := adapter.ReceiveAudio()

	// Should receive first audio chunk
	select {
	case data := <-audioCh:
		assert.Equal(t, []byte{1, 2, 3}, data)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for audio")
	}

	cancel()
	stream.Close()
}

func TestAudioStreamAdapter_ReceiveAudio_WithDecoder(t *testing.T) {
	chunks := make(chan *StreamChunk, 10)
	chunks <- &StreamChunk{Type: StreamTypeAudio, Data: []byte{1, 2, 3}}

	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case c := <-chunks:
				return c, nil
			}
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	stream := NewBidirectionalStream(config, handler, conn, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	adapter := NewAudioStreamAdapter(stream, 16000, 1)
	adapter.decoder = &mockDecoder{fn: func(data []byte) ([]byte, error) {
		return append(data, 0xFF), nil
	}}
	audioCh := adapter.ReceiveAudio()

	select {
	case data := <-audioCh:
		assert.Equal(t, []byte{1, 2, 3, 0xFF}, data)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}

	cancel()
	stream.Close()
}

// --- processHeartbeat ---

func TestBidirectionalStream_Heartbeat(t *testing.T) {
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	config.HeartbeatInterval = 50 * time.Millisecond
	config.HeartbeatTimeout = 200 * time.Millisecond
	config.EnableHeartbeat = true

	stream := NewBidirectionalStream(config, handler, conn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	// Let heartbeat run a few cycles
	time.Sleep(200 * time.Millisecond)

	cancel()
	stream.Close()
}

// --- monitorErrors ---

func TestBidirectionalStream_MonitorErrors(t *testing.T) {
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	config.EnableHeartbeat = false

	stream := NewBidirectionalStream(config, handler, conn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	// Send an error to the error channel
	stream.errChan <- errors.New("test error")
	time.Sleep(50 * time.Millisecond)

	cancel()
	stream.Close()
}

// --- processOutbound with connection write ---

func TestBidirectionalStream_ProcessOutbound(t *testing.T) {
	written := make(chan StreamChunk, 10)
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		writeFn: func(ctx context.Context, chunk StreamChunk) error {
			written <- chunk
			return nil
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	config.EnableHeartbeat = false

	stream := NewBidirectionalStream(config, handler, conn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	err := stream.Send(StreamChunk{Type: StreamTypeText, Text: "hello"})
	require.NoError(t, err)

	select {
	case chunk := <-written:
		assert.Equal(t, "hello", chunk.Text)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for outbound chunk")
	}

	cancel()
	stream.Close()
}

// --- StreamSession ---

func TestStreamSession_RecordSentReceived_Coverage(t *testing.T) {
	stream := NewBidirectionalStream(DefaultStreamConfig(), &mockHandler{}, nil, nil, nil)
	session := NewStreamSession(stream)

	session.RecordSent(100)
	session.RecordSent(200)
	assert.Equal(t, int64(300), session.BytesSent)
	assert.Equal(t, int64(2), session.ChunksSent)

	session.RecordReceived(50)
	assert.Equal(t, int64(50), session.BytesRecv)
	assert.Equal(t, int64(1), session.ChunksRecv)
}

// --- StreamWriter ---

func TestStreamWriter_Write_Coverage(t *testing.T) {
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	handler := &mockHandler{}
	config := DefaultStreamConfig()
	config.EnableHeartbeat = false

	stream := NewBidirectionalStream(config, handler, conn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	writer := &StreamWriter{stream: stream}
	n, err := writer.Write([]byte("hello"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)

	cancel()
	stream.Close()
}

// --- mock types ---

type mockEncoder struct {
	fn func([]byte) ([]byte, error)
}

func (e *mockEncoder) Encode(pcm []byte) ([]byte, error) { return e.fn(pcm) }

type mockDecoder struct {
	fn func([]byte) ([]byte, error)
}

func (d *mockDecoder) Decode(data []byte) ([]byte, error) { return d.fn(data) }

// Verify StreamWriter implements io.Writer
var _ io.Writer = (*StreamWriter)(nil)

// --- tryReconnect ---

func TestBidirectionalStream_TryReconnect_NoFactory(t *testing.T) {
	stream := NewBidirectionalStream(DefaultStreamConfig(), &mockHandler{}, nil, nil, nil)
	result := stream.tryReconnect(context.Background())
	assert.False(t, result)
}

func TestBidirectionalStream_TryReconnect_MaxAttempts(t *testing.T) {
	config := DefaultStreamConfig()
	config.MaxReconnects = 1
	config.ReconnectDelay = time.Millisecond

	failCount := 0
	factory := func() (StreamConnection, error) {
		failCount++
		return nil, errors.New("connection failed")
	}

	stream := NewBidirectionalStream(config, &mockHandler{}, nil, factory, nil)
	result := stream.tryReconnect(context.Background())
	assert.False(t, result)
	assert.Equal(t, StateError, stream.GetState())
}

func TestBidirectionalStream_TryReconnect_Success(t *testing.T) {
	config := DefaultStreamConfig()
	config.MaxReconnects = 3
	config.ReconnectDelay = time.Millisecond

	newConn := &mockConn{isAliveFn: func() bool { return true }}
	factory := func() (StreamConnection, error) {
		return newConn, nil
	}

	stream := NewBidirectionalStream(config, &mockHandler{}, nil, factory, nil)
	result := stream.tryReconnect(context.Background())
	assert.True(t, result)
	assert.Equal(t, StateConnected, stream.GetState())
}

// --- processOutbound with write error and reconnect ---

func TestBidirectionalStream_ProcessOutbound_WriteError(t *testing.T) {
	writeCount := 0
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		writeFn: func(ctx context.Context, chunk StreamChunk) error {
			writeCount++
			if writeCount == 1 {
				return errors.New("write failed")
			}
			return nil
		},
	}

	config := DefaultStreamConfig()
	config.EnableHeartbeat = false
	config.MaxReconnects = 1
	config.ReconnectDelay = time.Millisecond

	newConn := &mockConn{
		isAliveFn: func() bool { return true },
		writeFn:   func(ctx context.Context, chunk StreamChunk) error { return nil },
	}
	factory := func() (StreamConnection, error) { return newConn, nil }

	stream := NewBidirectionalStream(config, &mockHandler{}, conn, factory, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	err := stream.Send(StreamChunk{Type: StreamTypeText, Text: "hello"})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)
	cancel()
	stream.Close()
}

// --- processOutbound with handler error ---

func TestBidirectionalStream_ProcessOutbound_HandlerError(t *testing.T) {
	conn := &mockConn{
		isAliveFn: func() bool { return true },
		readFn: func(ctx context.Context) (*StreamChunk, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
		writeFn: func(ctx context.Context, chunk StreamChunk) error { return nil },
	}

	handler := &mockHandler{
		onOutboundFn: func(ctx context.Context, chunk StreamChunk) error {
			return errors.New("handler error")
		},
	}

	config := DefaultStreamConfig()
	config.EnableHeartbeat = false

	stream := NewBidirectionalStream(config, handler, conn, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, stream.Start(ctx))

	err := stream.Send(StreamChunk{Type: StreamTypeText, Text: "hello"})
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)
	cancel()
	stream.Close()
}


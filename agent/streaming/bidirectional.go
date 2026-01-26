// Package streaming provides bidirectional real-time streaming for audio/text.
package streaming

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StreamType defines the type of stream content.
type StreamType string

const (
	StreamTypeText  StreamType = "text"
	StreamTypeAudio StreamType = "audio"
	StreamTypeVideo StreamType = "video"
	StreamTypeMixed StreamType = "mixed"
)

// StreamChunk represents a chunk of streaming data.
type StreamChunk struct {
	ID        string         `json:"id"`
	Type      StreamType     `json:"type"`
	Data      []byte         `json:"data"`
	Text      string         `json:"text,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Sequence  int64          `json:"sequence"`
	IsFinal   bool           `json:"is_final"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// StreamConfig configures bidirectional streaming.
type StreamConfig struct {
	BufferSize     int           `json:"buffer_size"`
	MaxLatencyMS   int           `json:"max_latency_ms"`
	SampleRate     int           `json:"sample_rate"`
	Channels       int           `json:"channels"`
	EnableVAD      bool          `json:"enable_vad"`
	ChunkDuration  time.Duration `json:"chunk_duration"`
	ReconnectDelay time.Duration `json:"reconnect_delay"`
}

// DefaultStreamConfig returns default streaming configuration.
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		BufferSize:     1024,
		MaxLatencyMS:   200,
		SampleRate:     16000,
		Channels:       1,
		EnableVAD:      true,
		ChunkDuration:  100 * time.Millisecond,
		ReconnectDelay: time.Second,
	}
}

// BidirectionalStream manages real-time bidirectional communication.
type BidirectionalStream struct {
	ID       string
	Config   StreamConfig
	State    StreamState
	inbound  chan StreamChunk
	outbound chan StreamChunk
	handler  StreamHandler
	logger   *zap.Logger
	mu       sync.RWMutex
	done     chan struct{}
	sequence int64
}

// StreamState represents the stream state.
type StreamState string

const (
	StateDisconnected StreamState = "disconnected"
	StateConnecting   StreamState = "connecting"
	StateConnected    StreamState = "connected"
	StateStreaming    StreamState = "streaming"
	StatePaused       StreamState = "paused"
	StateError        StreamState = "error"
)

// StreamHandler processes stream data.
type StreamHandler interface {
	OnInbound(ctx context.Context, chunk StreamChunk) (*StreamChunk, error)
	OnOutbound(ctx context.Context, chunk StreamChunk) error
	OnStateChange(state StreamState)
}

// NewBidirectionalStream creates a new bidirectional stream.
func NewBidirectionalStream(config StreamConfig, handler StreamHandler, logger *zap.Logger) *BidirectionalStream {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BidirectionalStream{
		ID:       fmt.Sprintf("stream_%d", time.Now().UnixNano()),
		Config:   config,
		State:    StateDisconnected,
		inbound:  make(chan StreamChunk, config.BufferSize),
		outbound: make(chan StreamChunk, config.BufferSize),
		handler:  handler,
		logger:   logger.With(zap.String("component", "bidirectional_stream")),
		done:     make(chan struct{}),
	}
}

// Start begins the bidirectional stream.
func (s *BidirectionalStream) Start(ctx context.Context) error {
	s.setState(StateConnecting)
	s.logger.Info("starting bidirectional stream")

	s.setState(StateConnected)

	// Start processing goroutines
	go s.processInbound(ctx)
	go s.processOutbound(ctx)

	s.setState(StateStreaming)
	return nil
}

// Send sends data to the outbound stream.
func (s *BidirectionalStream) Send(chunk StreamChunk) error {
	s.mu.Lock()
	s.sequence++
	chunk.Sequence = s.sequence
	s.mu.Unlock()

	if chunk.Timestamp.IsZero() {
		chunk.Timestamp = time.Now()
	}

	select {
	case s.outbound <- chunk:
		return nil
	default:
		return fmt.Errorf("outbound buffer full")
	}
}

// Receive returns the inbound channel for receiving data.
func (s *BidirectionalStream) Receive() <-chan StreamChunk {
	return s.inbound
}

// Close closes the stream.
func (s *BidirectionalStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateDisconnected {
		return nil
	}

	close(s.done)
	s.State = StateDisconnected
	s.logger.Info("stream closed")
	return nil
}

func (s *BidirectionalStream) setState(state StreamState) {
	s.mu.Lock()
	s.State = state
	s.mu.Unlock()
	if s.handler != nil {
		s.handler.OnStateChange(state)
	}
}

func (s *BidirectionalStream) processInbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case chunk := <-s.outbound:
			if s.handler != nil {
				response, err := s.handler.OnInbound(ctx, chunk)
				if err != nil {
					s.logger.Error("inbound handler error", zap.Error(err))
					continue
				}
				if response != nil {
					select {
					case s.inbound <- *response:
					default:
						s.logger.Warn("inbound buffer full")
					}
				}
			}
		}
	}
}

func (s *BidirectionalStream) processOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}

// GetState returns the current stream state.
func (s *BidirectionalStream) GetState() StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// StreamSession manages a complete streaming session.
type StreamSession struct {
	ID         string
	Stream     *BidirectionalStream
	StartTime  time.Time
	EndTime    time.Time
	BytesSent  int64
	BytesRecv  int64
	ChunksSent int64
	ChunksRecv int64
	mu         sync.Mutex
}

// NewStreamSession creates a new stream session.
func NewStreamSession(stream *BidirectionalStream) *StreamSession {
	return &StreamSession{
		ID:        fmt.Sprintf("session_%d", time.Now().UnixNano()),
		Stream:    stream,
		StartTime: time.Now(),
	}
}

// RecordSent records sent data.
func (s *StreamSession) RecordSent(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesSent += bytes
	s.ChunksSent++
}

// RecordReceived records received data.
func (s *StreamSession) RecordReceived(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesRecv += bytes
	s.ChunksRecv++
}

// StreamManager manages multiple streams.
type StreamManager struct {
	streams map[string]*BidirectionalStream
	logger  *zap.Logger
	mu      sync.RWMutex
}

// NewStreamManager creates a new stream manager.
func NewStreamManager(logger *zap.Logger) *StreamManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StreamManager{
		streams: make(map[string]*BidirectionalStream),
		logger:  logger.With(zap.String("component", "stream_manager")),
	}
}

// CreateStream creates a new stream.
func (m *StreamManager) CreateStream(config StreamConfig, handler StreamHandler) *BidirectionalStream {
	stream := NewBidirectionalStream(config, handler, m.logger)
	m.mu.Lock()
	m.streams[stream.ID] = stream
	m.mu.Unlock()
	return stream
}

// GetStream retrieves a stream by ID.
func (m *StreamManager) GetStream(id string) (*BidirectionalStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stream, ok := m.streams[id]
	return stream, ok
}

// CloseStream closes and removes a stream.
func (m *StreamManager) CloseStream(id string) error {
	m.mu.Lock()
	stream, ok := m.streams[id]
	if ok {
		delete(m.streams, id)
	}
	m.mu.Unlock()

	if ok {
		return stream.Close()
	}
	return nil
}

// AudioStreamAdapter adapts audio streams for bidirectional communication.
type AudioStreamAdapter struct {
	stream     *BidirectionalStream
	sampleRate int
	channels   int
	encoder    AudioEncoder
	decoder    AudioDecoder
}

// AudioEncoder encodes audio data.
type AudioEncoder interface {
	Encode(pcm []byte) ([]byte, error)
}

// AudioDecoder decodes audio data.
type AudioDecoder interface {
	Decode(data []byte) ([]byte, error)
}

// NewAudioStreamAdapter creates a new audio stream adapter.
func NewAudioStreamAdapter(stream *BidirectionalStream, sampleRate, channels int) *AudioStreamAdapter {
	return &AudioStreamAdapter{
		stream:     stream,
		sampleRate: sampleRate,
		channels:   channels,
	}
}

// SendAudio sends audio data.
func (a *AudioStreamAdapter) SendAudio(pcm []byte) error {
	data := pcm
	if a.encoder != nil {
		var err error
		data, err = a.encoder.Encode(pcm)
		if err != nil {
			return err
		}
	}
	return a.stream.Send(StreamChunk{
		Type: StreamTypeAudio,
		Data: data,
		Metadata: map[string]any{
			"sample_rate": a.sampleRate,
			"channels":    a.channels,
		},
	})
}

// ReceiveAudio returns decoded audio chunks.
func (a *AudioStreamAdapter) ReceiveAudio() <-chan []byte {
	out := make(chan []byte, 100)
	go func() {
		defer close(out)
		for chunk := range a.stream.Receive() {
			if chunk.Type != StreamTypeAudio {
				continue
			}
			data := chunk.Data
			if a.decoder != nil {
				var err error
				data, err = a.decoder.Decode(chunk.Data)
				if err != nil {
					continue
				}
			}
			out <- data
		}
	}()
	return out
}

// TextStreamAdapter adapts text streams.
type TextStreamAdapter struct {
	stream *BidirectionalStream
}

// NewTextStreamAdapter creates a new text stream adapter.
func NewTextStreamAdapter(stream *BidirectionalStream) *TextStreamAdapter {
	return &TextStreamAdapter{stream: stream}
}

// SendText sends text data.
func (t *TextStreamAdapter) SendText(text string, isFinal bool) error {
	return t.stream.Send(StreamChunk{
		Type:    StreamTypeText,
		Text:    text,
		IsFinal: isFinal,
	})
}

// ReceiveText returns text chunks.
func (t *TextStreamAdapter) ReceiveText() <-chan string {
	out := make(chan string, 100)
	go func() {
		defer close(out)
		for chunk := range t.stream.Receive() {
			if chunk.Type == StreamTypeText && chunk.Text != "" {
				out <- chunk.Text
			}
		}
	}()
	return out
}

// StreamReader wraps a stream as io.Reader.
type StreamReader struct {
	stream *BidirectionalStream
	buffer []byte
}

// NewStreamReader creates a new stream reader.
func NewStreamReader(stream *BidirectionalStream) *StreamReader {
	return &StreamReader{stream: stream}
}

func (r *StreamReader) Read(p []byte) (n int, err error) {
	if len(r.buffer) > 0 {
		n = copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		return n, nil
	}

	chunk, ok := <-r.stream.Receive()
	if !ok {
		return 0, io.EOF
	}

	n = copy(p, chunk.Data)
	if n < len(chunk.Data) {
		r.buffer = chunk.Data[n:]
	}
	return n, nil
}

// StreamWriter wraps a stream as io.Writer.
type StreamWriter struct {
	stream *BidirectionalStream
}

// NewStreamWriter creates a new stream writer.
func NewStreamWriter(stream *BidirectionalStream) *StreamWriter {
	return &StreamWriter{stream: stream}
}

func (w *StreamWriter) Write(p []byte) (n int, err error) {
	err = w.stream.Send(StreamChunk{
		Type: StreamTypeText,
		Data: p,
	})
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

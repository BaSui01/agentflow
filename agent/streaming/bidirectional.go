package streaming

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.uber.org/zap"
)

// StreamType 定义了流内容的类型.
type StreamType string

const (
	StreamTypeText  StreamType = "text"
	StreamTypeAudio StreamType = "audio"
	StreamTypeVideo StreamType = "video"
	StreamTypeMixed StreamType = "mixed"
)

// StreamChunk代表了一整批流数据.
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

// StreamConnection 底层流式连接接口（WebSocket、gRPC stream 等）
type StreamConnection interface {
	// ReadChunk 从连接读取一个数据块（阻塞直到有数据或出错）
	ReadChunk(ctx context.Context) (*StreamChunk, error)
	// WriteChunk 向连接写入一个数据块
	WriteChunk(ctx context.Context, chunk StreamChunk) error
	// Close 关闭连接
	Close() error
	// IsAlive 检查连接是否存活
	IsAlive() bool
}

// StreamConfig 配置双向流.
type StreamConfig struct {
	BufferSize     int           `json:"buffer_size"`
	MaxLatencyMS   int           `json:"max_latency_ms"`
	SampleRate     int           `json:"sample_rate"`
	Channels       int           `json:"channels"`
	EnableVAD      bool          `json:"enable_vad"`
	ChunkDuration  time.Duration `json:"chunk_duration"`
	ReconnectDelay time.Duration `json:"reconnect_delay"`
	// 新增字段
	HeartbeatInterval time.Duration `json:"heartbeat_interval"` // 心跳间隔，默认 30s
	HeartbeatTimeout  time.Duration `json:"heartbeat_timeout"`  // 心跳超时，默认 10s
	MaxReconnects     int           `json:"max_reconnects"`     // 最大重连次数，默认 5
	EnableHeartbeat   bool          `json:"enable_heartbeat"`   // 是否启用心跳
}

// 默认 StreamConfig 返回默认流化配置 。
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		BufferSize:        1024,
		MaxLatencyMS:      200,
		SampleRate:        16000,
		Channels:          1,
		EnableVAD:         true,
		ChunkDuration:     100 * time.Millisecond,
		ReconnectDelay:    time.Second,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxReconnects:     5,
		EnableHeartbeat:   true,
	}
}

// 双向结构管理实时双向通信.
type BidirectionalStream struct {
	ID       string
	Config   StreamConfig
	State    StreamState
	inbound  chan StreamChunk
	outbound chan StreamChunk
	handler  StreamHandler
	conn     StreamConnection // 新增：底层连接
	logger   *zap.Logger
	mu       sync.RWMutex
	done     chan struct{}
	sequence int64
	// 新增字段
	connFactory    func() (StreamConnection, error) // 连接工厂，用于重连
	reconnectCount int
	lastHeartbeat  time.Time
	errChan        chan error // 内部错误通道
}

// 流州代表流州.
type StreamState string

const (
	StateDisconnected StreamState = "disconnected"
	StateConnecting   StreamState = "connecting"
	StateConnected    StreamState = "connected"
	StateStreaming    StreamState = "streaming"
	StatePaused       StreamState = "paused"
	StateError        StreamState = "error"
)

// StreamHandler处理流数据.
type StreamHandler interface {
	OnInbound(ctx context.Context, chunk StreamChunk) (*StreamChunk, error)
	OnOutbound(ctx context.Context, chunk StreamChunk) error
	OnStateChange(state StreamState)
}

// NewBiFireStream 创建了新的双向流.
func NewBidirectionalStream(
	config StreamConfig,
	handler StreamHandler,
	conn StreamConnection,
	connFactory func() (StreamConnection, error),
	logger *zap.Logger,
) *BidirectionalStream {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BidirectionalStream{
		ID:          fmt.Sprintf("stream_%d", time.Now().UnixNano()),
		Config:      config,
		State:       StateDisconnected,
		inbound:     make(chan StreamChunk, config.BufferSize),
		outbound:    make(chan StreamChunk, config.BufferSize),
		handler:     handler,
		conn:        conn,
		connFactory: connFactory,
		logger:      logger.With(zap.String("component", "bidirectional_stream")),
		done:        make(chan struct{}),
		errChan:     make(chan error, 16),
	}
}

// 开始双向流 。
func (s *BidirectionalStream) Start(ctx context.Context) error {
	s.setState(StateConnecting)
	s.logger.Info("starting bidirectional stream")

	// 验证连接
	if s.conn == nil && s.connFactory != nil {
		conn, err := s.connFactory()
		if err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to establish connection: %w", err)
		}
		s.conn = conn
	}
	if s.conn == nil {
		s.setState(StateError)
		return fmt.Errorf("no connection available")
	}

	s.setState(StateConnected)

	s.mu.Lock()
	s.lastHeartbeat = time.Now()
	s.mu.Unlock()

	// 启动处理协程
	go s.processInbound(ctx)
	go s.processOutbound(ctx)
	go s.processHeartbeat(ctx)
	go s.monitorErrors(ctx)

	s.setState(StateStreaming)
	return nil
}

// monitorErrors 监控内部错误
func (s *BidirectionalStream) monitorErrors(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case err := <-s.errChan:
			s.logger.Warn("stream error detected", zap.Error(err))
			// 连续错误可以触发状态变更
			if s.GetState() == StateError {
				return
			}
		}
	}
}

// 发送数据到外出流.
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

// 接收输入通道以接收数据 。
func (s *BidirectionalStream) Receive() <-chan StreamChunk {
	return s.inbound
}

// 关上溪口.
func (s *BidirectionalStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateDisconnected {
		return nil
	}

	close(s.done)
	s.State = StateDisconnected

	// 关闭底层连接
	var connErr error
	if s.conn != nil {
		connErr = s.conn.Close()
	}

	// 排空 channel
	close(s.inbound)
	close(s.outbound)

	s.logger.Info("stream closed")
	return connErr
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
	defer s.logger.Debug("processInbound exited")
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		// 从底层连接读取数据
		chunk, err := s.conn.ReadChunk(ctx)
		if err != nil {
			// 检查是否是正常关闭
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				return
			default:
			}

			s.logger.Error("connection read error", zap.Error(err))
			s.errChan <- fmt.Errorf("inbound read error: %w", err)

			// 尝试重连
			if s.tryReconnect(ctx) {
				continue
			}
			return
		}

		if chunk == nil {
			continue
		}

		// 更新心跳时间
		s.mu.Lock()
		s.lastHeartbeat = time.Now()
		s.mu.Unlock()

		// 跳过心跳包
		if chunk.Type == "heartbeat" {
			continue
		}

		// 调用 handler 处理入站数据
		if s.handler != nil {
			response, err := s.handler.OnInbound(ctx, *chunk)
			if err != nil {
				s.logger.Error("inbound handler error", zap.Error(err))
				continue
			}
			if response != nil {
				select {
				case s.inbound <- *response:
				case <-s.done:
					return
				default:
					s.logger.Warn("inbound buffer full, dropping chunk",
						zap.Int64("sequence", response.Sequence))
				}
			}
		} else {
			// 没有 handler 时直接写入 inbound channel
			select {
			case s.inbound <- *chunk:
			case <-s.done:
				return
			default:
				s.logger.Warn("inbound buffer full, dropping chunk",
					zap.Int64("sequence", chunk.Sequence))
			}
		}
	}
}

func (s *BidirectionalStream) processOutbound(ctx context.Context) {
	defer s.logger.Debug("processOutbound exited")
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case chunk := <-s.outbound:
			// 调用 handler 预处理出站数据
			if s.handler != nil {
				if err := s.handler.OnOutbound(ctx, chunk); err != nil {
					s.logger.Error("outbound handler error",
						zap.Error(err),
						zap.Int64("sequence", chunk.Sequence))
					continue
				}
			}

			// 写入底层连接
			if err := s.conn.WriteChunk(ctx, chunk); err != nil {
				s.logger.Error("connection write error", zap.Error(err))
				s.errChan <- fmt.Errorf("outbound write error: %w", err)

				// 尝试重连后重发
				if s.tryReconnect(ctx) {
					// 重连成功，重新发送当前 chunk
					if retryErr := s.conn.WriteChunk(ctx, chunk); retryErr != nil {
						s.logger.Error("retry write failed after reconnect", zap.Error(retryErr))
					}
					continue
				}
				return
			}
		}
	}
}

// processHeartbeat 定期发送心跳并检测超时
func (s *BidirectionalStream) processHeartbeat(ctx context.Context) {
	if !s.Config.EnableHeartbeat {
		return
	}

	ticker := time.NewTicker(s.Config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			// 发送心跳
			heartbeat := StreamChunk{
				Type:      "heartbeat",
				Timestamp: time.Now(),
				Metadata:  map[string]any{"ping": true},
			}
			if err := s.conn.WriteChunk(ctx, heartbeat); err != nil {
				s.logger.Warn("heartbeat send failed", zap.Error(err))
				s.errChan <- fmt.Errorf("heartbeat failed: %w", err)
			}

			// 检查对端心跳超时
			s.mu.RLock()
			lastBeat := s.lastHeartbeat
			s.mu.RUnlock()

			if !lastBeat.IsZero() && time.Since(lastBeat) > s.Config.HeartbeatTimeout+s.Config.HeartbeatInterval {
				s.logger.Warn("heartbeat timeout detected",
					zap.Duration("since_last", time.Since(lastBeat)))
				s.errChan <- fmt.Errorf("heartbeat timeout: last=%v", lastBeat)

				// 尝试重连
				if !s.tryReconnect(ctx) {
					s.setState(StateError)
					return
				}
			}
		}
	}
}

// tryReconnect 尝试重新建立连接
func (s *BidirectionalStream) tryReconnect(ctx context.Context) bool {
	if s.connFactory == nil {
		s.logger.Error("no connection factory, cannot reconnect")
		return false
	}

	s.mu.Lock()
	if s.reconnectCount >= s.Config.MaxReconnects {
		s.mu.Unlock()
		s.logger.Error("max reconnect attempts reached",
			zap.Int("attempts", s.reconnectCount))
		s.setState(StateError)
		return false
	}
	s.reconnectCount++
	attempt := s.reconnectCount
	s.mu.Unlock()

	s.setState(StateConnecting)
	s.logger.Info("attempting reconnect",
		zap.Int("attempt", attempt),
		zap.Int("max", s.Config.MaxReconnects))

	// 指数退避
	delay := s.Config.ReconnectDelay * time.Duration(1<<uint(attempt-1))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	select {
	case <-ctx.Done():
		return false
	case <-s.done:
		return false
	case <-time.After(delay):
	}

	// 关闭旧连接
	if s.conn != nil {
		_ = s.conn.Close()
	}

	// 创建新连接
	newConn, err := s.connFactory()
	if err != nil {
		s.logger.Error("reconnect failed", zap.Error(err), zap.Int("attempt", attempt))
		return s.tryReconnect(ctx) // 递归重试
	}

	s.mu.Lock()
	s.conn = newConn
	s.lastHeartbeat = time.Now()
	s.mu.Unlock()

	s.setState(StateConnected)
	s.logger.Info("reconnected successfully", zap.Int("attempt", attempt))

	// 重置重连计数
	s.mu.Lock()
	s.reconnectCount = 0
	s.mu.Unlock()

	return true
}

// GetState 返回当前流状态 。
func (s *BidirectionalStream) GetState() StreamState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.State
}

// 串流会管理完整的串流会话。
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

// NewStream Session 创建了新流会话.
func NewStreamSession(stream *BidirectionalStream) *StreamSession {
	return &StreamSession{
		ID:        fmt.Sprintf("session_%d", time.Now().UnixNano()),
		Stream:    stream,
		StartTime: time.Now(),
	}
}

// 记录发送数据。
func (s *StreamSession) RecordSent(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesSent += bytes
	s.ChunksSent++
}

// 记录收到数据。
func (s *StreamSession) RecordReceived(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesRecv += bytes
	s.ChunksRecv++
}

// StreamManager管理多条流.
type StreamManager struct {
	streams map[string]*BidirectionalStream
	logger  *zap.Logger
	mu      sync.RWMutex
}

// NewStreamManager创建了新流管理器.
func NewStreamManager(logger *zap.Logger) *StreamManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &StreamManager{
		streams: make(map[string]*BidirectionalStream),
		logger:  logger.With(zap.String("component", "stream_manager")),
	}
}

// 创建 Stream 创建一个新流 。
func (m *StreamManager) CreateStream(
	config StreamConfig,
	handler StreamHandler,
	conn StreamConnection,
	connFactory func() (StreamConnection, error),
) *BidirectionalStream {
	stream := NewBidirectionalStream(config, handler, conn, connFactory, m.logger)
	m.mu.Lock()
	m.streams[stream.ID] = stream
	m.mu.Unlock()
	return stream
}

// Get Stream通过ID检索出一条流.
func (m *StreamManager) GetStream(id string) (*BidirectionalStream, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stream, ok := m.streams[id]
	return stream, ok
}

// 关闭 Stream 关闭并去除一串流.
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

// AudioStreamAdapter 调整音频流用于双向通信.
type AudioStreamAdapter struct {
	stream     *BidirectionalStream
	sampleRate int
	channels   int
	encoder    AudioEncoder
	decoder    AudioDecoder
}

// 音频编码器编码音频数据.
type AudioEncoder interface {
	Encode(pcm []byte) ([]byte, error)
}

// AudioDecoder解码音频数据.
type AudioDecoder interface {
	Decode(data []byte) ([]byte, error)
}

// 新AudioStreamAdapter创建了一个新的音频流适配器.
func NewAudioStreamAdapter(stream *BidirectionalStream, sampleRate, channels int) *AudioStreamAdapter {
	return &AudioStreamAdapter{
		stream:     stream,
		sampleRate: sampleRate,
		channels:   channels,
	}
}

// SendAudio发送音频数据.
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

// DuiceAudio返回已解码的音频块 。
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

// TextStreamAdapter 适应文本流.
type TextStreamAdapter struct {
	stream *BidirectionalStream
}

// 新TextStreamAdapter创建了新的文本流适配器.
func NewTextStreamAdapter(stream *BidirectionalStream) *TextStreamAdapter {
	return &TextStreamAdapter{stream: stream}
}

// 发送文本数据 。
func (t *TextStreamAdapter) SendText(text string, isFinal bool) error {
	return t.stream.Send(StreamChunk{
		Type:    StreamTypeText,
		Text:    text,
		IsFinal: isFinal,
	})
}

// 接收文本返回文本块 。
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

// StreamReader将一流包裹为io. 读者.
type StreamReader struct {
	stream *BidirectionalStream
	buffer []byte
}

// NewStream Reader创建了新流读取器.
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

// StreamWriter)将一流包裹为io. 编剧.
type StreamWriter struct {
	stream *BidirectionalStream
}

// NewStreamWriter创建了新流作家.
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

// 软件包语音提供实时语音代理能力.
package voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// VoiceConfig 配置语音代理 。
type VoiceConfig struct {
	STTProvider      string        `json:"stt_provider"`      // deepgram, assemblyai, whisper
	TTSProvider      string        `json:"tts_provider"`      // elevenlabs, openai, azure
	SampleRate       int           `json:"sample_rate"`       // 16000, 24000, 48000
	MaxLatencyMS     int           `json:"max_latency_ms"`    // Target latency
	VADEnabled       bool          `json:"vad_enabled"`       // Voice Activity Detection
	InterruptEnabled bool          `json:"interrupt_enabled"` // Allow interruptions
	BufferDuration   time.Duration `json:"buffer_duration"`
}

// 默认 VoiceConfig 为低延迟返回优化默认值。
func DefaultVoiceConfig() VoiceConfig {
	return VoiceConfig{
		STTProvider:      "deepgram",
		TTSProvider:      "elevenlabs",
		SampleRate:       16000,
		MaxLatencyMS:     300,
		VADEnabled:       true,
		InterruptEnabled: true,
		BufferDuration:   100 * time.Millisecond,
	}
}

// AudioChunk代表了一大块音频数据.
type AudioChunk struct {
	Data       []byte    `json:"data"`
	SampleRate int       `json:"sample_rate"`
	Channels   int       `json:"channels"`
	Timestamp  time.Time `json:"timestamp"`
	IsFinal    bool      `json:"is_final"`
}

// TranscriptEvent代表一个语音对文本事件.
type TranscriptEvent struct {
	Text       string    `json:"text"`
	IsFinal    bool      `json:"is_final"`
	Confidence float64   `json:"confidence"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Timestamp  time.Time `json:"timestamp"`
}

// SpeechEvent 代表文字对语音事件.
type SpeechEvent struct {
	Audio     []byte    `json:"audio"`
	Text      string    `json:"text"`
	IsFinal   bool      `json:"is_final"`
	Timestamp time.Time `json:"timestamp"`
}

// 语音状态代表语音代理的当前状态.
type VoiceState string

const (
	StateIdle        VoiceState = "idle"
	StateListening   VoiceState = "listening"
	StateProcessing  VoiceState = "processing"
	StateSpeaking    VoiceState = "speaking"
	StateInterrupted VoiceState = "interrupted"
)

// STTProvider定义了语音到文本接口.
type STTProvider interface {
	StartStream(ctx context.Context, sampleRate int) (STTStream, error)
	Name() string
}

// STTstream代表流传的STT会话.
type STTStream interface {
	Send(chunk AudioChunk) error
	Receive() <-chan TranscriptEvent
	Close() error
}

// TTS Provider定义了文本到语音界面.
type TTSProvider interface {
	Synthesize(ctx context.Context, text string) (<-chan SpeechEvent, error)
	SynthesizeStream(ctx context.Context, textChan <-chan string) (<-chan SpeechEvent, error)
	Name() string
}

// LLMHandler为语音处理LLM交互.
type LLMHandler interface {
	ProcessStream(ctx context.Context, input string) (<-chan string, error)
}

// VoiceAgent执行实时语音代理.
type VoiceAgent struct {
	config VoiceConfig
	stt    STTProvider
	tts    TTSProvider
	llm    LLMHandler
	logger *zap.Logger

	state   VoiceState
	stateMu sync.RWMutex

	// 计量
	metrics   VoiceMetrics
	metricsMu sync.Mutex
}

// 语音计量跟踪语音代理性能.
type VoiceMetrics struct {
	TotalSessions     int64         `json:"total_sessions"`
	AverageLatency    time.Duration `json:"average_latency"`
	P95Latency        time.Duration `json:"p95_latency"`
	InterruptionCount int64         `json:"interruption_count"`
	TotalAudioSeconds float64       `json:"total_audio_seconds"`
}

// NewVoiceAgent创建了新的语音代理.
func NewVoiceAgent(config VoiceConfig, stt STTProvider, tts TTSProvider, llm LLMHandler, logger *zap.Logger) *VoiceAgent {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &VoiceAgent{
		config: config,
		stt:    stt,
		tts:    tts,
		llm:    llm,
		logger: logger,
		state:  StateIdle,
	}
}

// 开始语音对话
func (v *VoiceAgent) Start(ctx context.Context) (*VoiceSession, error) {
	v.setState(StateListening)

	session := &VoiceSession{
		ID:         fmt.Sprintf("voice_%d", time.Now().UnixNano()),
		agent:      v,
		startTime:  time.Now(),
		audioChan:  make(chan AudioChunk, 100),
		textChan:   make(chan string, 10),
		speechChan: make(chan SpeechEvent, 100),
		doneChan:   make(chan struct{}),
	}

	// 启动 STT 流
	sttStream, err := v.stt.StartStream(ctx, v.config.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("failed to start STT: %w", err)
	}
	session.sttStream = sttStream

	// 开始处理去例程
	go session.processAudio(ctx)
	go session.processTranscripts(ctx)
	go session.processSpeech(ctx)

	v.metricsMu.Lock()
	v.metrics.TotalSessions++
	v.metricsMu.Unlock()

	return session, nil
}

func (v *VoiceAgent) setState(state VoiceState) {
	v.stateMu.Lock()
	defer v.stateMu.Unlock()
	v.state = state
}

// GetState 返回当前状态 。
func (v *VoiceAgent) GetState() VoiceState {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return v.state
}

// GetMetrics 返回当前度量衡 。
func (v *VoiceAgent) GetMetrics() VoiceMetrics {
	v.metricsMu.Lock()
	defer v.metricsMu.Unlock()
	return v.metrics
}

// 语音会议代表了积极的语音对话。
type VoiceSession struct {
	ID         string
	agent      *VoiceAgent
	sttStream  STTStream
	startTime  time.Time
	audioChan  chan AudioChunk
	textChan   chan string
	speechChan chan SpeechEvent
	doneChan   chan struct{}
	mu         sync.Mutex
	closed     bool
}

// SendAudio向会话发送音频数据.
func (s *VoiceSession) SendAudio(chunk AudioChunk) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return fmt.Errorf("session closed")
	}
	s.mu.Unlock()

	select {
	case s.audioChan <- chunk:
		return nil
	default:
		return fmt.Errorf("audio buffer full")
	}
}

// 接收Speech返回接收合成语音的信道 。
func (s *VoiceSession) ReceiveSpeech() <-chan SpeechEvent {
	return s.speechChan
}

// 关闭会话 。
func (s *VoiceSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	close(s.doneChan)
	close(s.audioChan)

	if s.sttStream != nil {
		s.sttStream.Close()
	}

	s.agent.setState(StateIdle)
	return nil
}

func (s *VoiceSession) processAudio(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.doneChan:
			return
		case chunk, ok := <-s.audioChan:
			if !ok {
				return
			}
			if err := s.sttStream.Send(chunk); err != nil {
				s.agent.logger.Error("failed to send audio", zap.Error(err))
			}
		}
	}
}

func (s *VoiceSession) processTranscripts(ctx context.Context) {
	transcriptChan := s.sttStream.Receive()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.doneChan:
			return
		case transcript, ok := <-transcriptChan:
			if !ok {
				return
			}

			if transcript.IsFinal && transcript.Text != "" {
				s.agent.setState(StateProcessing)
				s.agent.logger.Debug("transcript received",
					zap.String("text", transcript.Text),
					zap.Float64("confidence", transcript.Confidence))

				// 通过 LLM 处理
				go s.processLLMResponse(ctx, transcript.Text)
			}
		}
	}
}

func (s *VoiceSession) processLLMResponse(ctx context.Context, input string) {
	responseChan, err := s.agent.llm.ProcessStream(ctx, input)
	if err != nil {
		s.agent.logger.Error("LLM processing failed", zap.Error(err))
		return
	}

	s.agent.setState(StateSpeaking)

	// 对 TTS 的流 LLM 响应
	speechChan, err := s.agent.tts.SynthesizeStream(ctx, responseChan)
	if err != nil {
		s.agent.logger.Error("TTS synthesis failed", zap.Error(err))
		return
	}

	for speech := range speechChan {
		select {
		case <-ctx.Done():
			return
		case <-s.doneChan:
			return
		case s.speechChan <- speech:
		}
	}

	s.agent.setState(StateListening)
}

func (s *VoiceSession) processSpeech(ctx context.Context) {
	// 启用时处理中断
	if !s.agent.config.InterruptEnabled {
		return
	}

	// 代理服务器发言时用户语音监视器
	// 这将与 VAD 整合以检测中断
}

// 中断当前发言 。
func (s *VoiceSession) Interrupt() {
	s.agent.setState(StateInterrupted)
	s.agent.metricsMu.Lock()
	s.agent.metrics.InterruptionCount++
	s.agent.metricsMu.Unlock()
	s.agent.logger.Debug("speech interrupted")
}

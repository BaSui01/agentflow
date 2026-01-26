// Package voice provides real-time voice agent capabilities.
package voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// VoiceConfig configures the voice agent.
type VoiceConfig struct {
	STTProvider      string        `json:"stt_provider"`      // deepgram, assemblyai, whisper
	TTSProvider      string        `json:"tts_provider"`      // elevenlabs, openai, azure
	SampleRate       int           `json:"sample_rate"`       // 16000, 24000, 48000
	MaxLatencyMS     int           `json:"max_latency_ms"`    // Target latency
	VADEnabled       bool          `json:"vad_enabled"`       // Voice Activity Detection
	InterruptEnabled bool          `json:"interrupt_enabled"` // Allow interruptions
	BufferDuration   time.Duration `json:"buffer_duration"`
}

// DefaultVoiceConfig returns optimized defaults for low latency.
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

// AudioChunk represents a chunk of audio data.
type AudioChunk struct {
	Data       []byte    `json:"data"`
	SampleRate int       `json:"sample_rate"`
	Channels   int       `json:"channels"`
	Timestamp  time.Time `json:"timestamp"`
	IsFinal    bool      `json:"is_final"`
}

// TranscriptEvent represents a speech-to-text event.
type TranscriptEvent struct {
	Text       string    `json:"text"`
	IsFinal    bool      `json:"is_final"`
	Confidence float64   `json:"confidence"`
	StartTime  float64   `json:"start_time"`
	EndTime    float64   `json:"end_time"`
	Timestamp  time.Time `json:"timestamp"`
}

// SpeechEvent represents a text-to-speech event.
type SpeechEvent struct {
	Audio     []byte    `json:"audio"`
	Text      string    `json:"text"`
	IsFinal   bool      `json:"is_final"`
	Timestamp time.Time `json:"timestamp"`
}

// VoiceState represents the current state of the voice agent.
type VoiceState string

const (
	StateIdle        VoiceState = "idle"
	StateListening   VoiceState = "listening"
	StateProcessing  VoiceState = "processing"
	StateSpeaking    VoiceState = "speaking"
	StateInterrupted VoiceState = "interrupted"
)

// STTProvider defines the speech-to-text interface.
type STTProvider interface {
	StartStream(ctx context.Context, sampleRate int) (STTStream, error)
	Name() string
}

// STTStream represents a streaming STT session.
type STTStream interface {
	Send(chunk AudioChunk) error
	Receive() <-chan TranscriptEvent
	Close() error
}

// TTSProvider defines the text-to-speech interface.
type TTSProvider interface {
	Synthesize(ctx context.Context, text string) (<-chan SpeechEvent, error)
	SynthesizeStream(ctx context.Context, textChan <-chan string) (<-chan SpeechEvent, error)
	Name() string
}

// LLMHandler handles LLM interactions for voice.
type LLMHandler interface {
	ProcessStream(ctx context.Context, input string) (<-chan string, error)
}

// VoiceAgent implements a real-time voice agent.
type VoiceAgent struct {
	config VoiceConfig
	stt    STTProvider
	tts    TTSProvider
	llm    LLMHandler
	logger *zap.Logger

	state   VoiceState
	stateMu sync.RWMutex

	// Metrics
	metrics   VoiceMetrics
	metricsMu sync.Mutex
}

// VoiceMetrics tracks voice agent performance.
type VoiceMetrics struct {
	TotalSessions     int64         `json:"total_sessions"`
	AverageLatency    time.Duration `json:"average_latency"`
	P95Latency        time.Duration `json:"p95_latency"`
	InterruptionCount int64         `json:"interruption_count"`
	TotalAudioSeconds float64       `json:"total_audio_seconds"`
}

// NewVoiceAgent creates a new voice agent.
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

// Start starts a voice conversation session.
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

	// Start STT stream
	sttStream, err := v.stt.StartStream(ctx, v.config.SampleRate)
	if err != nil {
		return nil, fmt.Errorf("failed to start STT: %w", err)
	}
	session.sttStream = sttStream

	// Start processing goroutines
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

// GetState returns the current state.
func (v *VoiceAgent) GetState() VoiceState {
	v.stateMu.RLock()
	defer v.stateMu.RUnlock()
	return v.state
}

// GetMetrics returns current metrics.
func (v *VoiceAgent) GetMetrics() VoiceMetrics {
	v.metricsMu.Lock()
	defer v.metricsMu.Unlock()
	return v.metrics
}

// VoiceSession represents an active voice conversation.
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

// SendAudio sends audio data to the session.
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

// ReceiveSpeech returns the channel for receiving synthesized speech.
func (s *VoiceSession) ReceiveSpeech() <-chan SpeechEvent {
	return s.speechChan
}

// Close closes the session.
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

				// Process through LLM
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

	// Stream LLM response to TTS
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
	// Handle interruptions if enabled
	if !s.agent.config.InterruptEnabled {
		return
	}

	// Monitor for user speech during agent speaking
	// This would integrate with VAD to detect interruptions
}

// Interrupt interrupts the current speech.
func (s *VoiceSession) Interrupt() {
	s.agent.setState(StateInterrupted)
	s.agent.metricsMu.Lock()
	s.agent.metrics.InterruptionCount++
	s.agent.metricsMu.Unlock()
	s.agent.logger.Debug("speech interrupted")
}

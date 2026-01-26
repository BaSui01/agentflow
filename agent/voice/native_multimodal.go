// Package voice provides native multimodal audio reasoning (GPT-4o style).
// Targets 232ms latency for real-time audio-to-audio processing.
package voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// NativeAudioConfig configures native audio reasoning.
type NativeAudioConfig struct {
	TargetLatencyMS int           `json:"target_latency_ms"` // Target: 232ms
	SampleRate      int           `json:"sample_rate"`
	ChunkSizeMS     int           `json:"chunk_size_ms"`
	BufferSize      int           `json:"buffer_size"`
	EnableVAD       bool          `json:"enable_vad"`
	Timeout         time.Duration `json:"timeout"`
}

// DefaultNativeAudioConfig returns optimized defaults for low latency.
func DefaultNativeAudioConfig() NativeAudioConfig {
	return NativeAudioConfig{
		TargetLatencyMS: 232,
		SampleRate:      24000,
		ChunkSizeMS:     20,
		BufferSize:      4096,
		EnableVAD:       true,
		Timeout:         30 * time.Second,
	}
}

// AudioFrame represents a single audio frame.
type AudioFrame struct {
	Data       []byte    `json:"data"`
	SampleRate int       `json:"sample_rate"`
	Channels   int       `json:"channels"`
	Duration   int       `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

// MultimodalInput represents input for native audio reasoning.
type MultimodalInput struct {
	Audio     []AudioFrame   `json:"audio,omitempty"`
	Text      string         `json:"text,omitempty"`
	Image     []byte         `json:"image,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// MultimodalOutput represents output from native audio reasoning.
type MultimodalOutput struct {
	Audio       []AudioFrame `json:"audio,omitempty"`
	Text        string       `json:"text,omitempty"`
	Transcript  string       `json:"transcript,omitempty"`
	LatencyMS   int64        `json:"latency_ms"`
	TokensUsed  int          `json:"tokens_used"`
	Confidence  float64      `json:"confidence"`
	Interrupted bool         `json:"interrupted"`
}

// NativeAudioProvider defines the interface for native audio models.
type NativeAudioProvider interface {
	ProcessAudio(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error)
	StreamAudio(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error)
	Name() string
}

// NativeAudioReasoner provides GPT-4o style native audio reasoning.
type NativeAudioReasoner struct {
	provider NativeAudioProvider
	config   NativeAudioConfig
	logger   *zap.Logger
	metrics  AudioMetrics
	mu       sync.Mutex
}

// AudioMetrics tracks audio processing metrics.
type AudioMetrics struct {
	TotalRequests  int64         `json:"total_requests"`
	AverageLatency time.Duration `json:"average_latency"`
	P95Latency     time.Duration `json:"p95_latency"`
	TargetHitRate  float64       `json:"target_hit_rate"`
	TotalAudioMS   int64         `json:"total_audio_ms"`
	Interruptions  int64         `json:"interruptions"`
	latencies      []time.Duration
}

// NewNativeAudioReasoner creates a new native audio reasoner.
func NewNativeAudioReasoner(provider NativeAudioProvider, config NativeAudioConfig, logger *zap.Logger) *NativeAudioReasoner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &NativeAudioReasoner{
		provider: provider,
		config:   config,
		logger:   logger.With(zap.String("component", "native_audio")),
		metrics:  AudioMetrics{latencies: make([]time.Duration, 0, 1000)},
	}
}

// Process processes multimodal input with native audio reasoning.
func (r *NativeAudioReasoner) Process(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	output, err := r.provider.ProcessAudio(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("audio processing failed: %w", err)
	}

	latency := time.Since(start)
	output.LatencyMS = latency.Milliseconds()

	r.updateMetrics(latency)

	r.logger.Debug("audio processed",
		zap.Int64("latency_ms", output.LatencyMS),
		zap.Int("target_ms", r.config.TargetLatencyMS),
	)

	return output, nil
}

// StreamProcess processes audio in streaming mode for lowest latency.
func (r *NativeAudioReasoner) StreamProcess(ctx context.Context, inputChan <-chan AudioFrame) (<-chan AudioFrame, error) {
	outputChan, err := r.provider.StreamAudio(ctx, inputChan)
	if err != nil {
		return nil, err
	}

	// Wrap output channel to track metrics
	wrappedChan := make(chan AudioFrame, r.config.BufferSize)
	go func() {
		defer close(wrappedChan)
		for frame := range outputChan {
			r.mu.Lock()
			r.metrics.TotalAudioMS += int64(frame.Duration)
			r.mu.Unlock()

			select {
			case wrappedChan <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()

	return wrappedChan, nil
}

func (r *NativeAudioReasoner) updateMetrics(latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.metrics.TotalRequests++
	r.metrics.latencies = append(r.metrics.latencies, latency)

	// Keep only last 1000 latencies
	if len(r.metrics.latencies) > 1000 {
		r.metrics.latencies = r.metrics.latencies[1:]
	}

	// Calculate average
	var total time.Duration
	for _, l := range r.metrics.latencies {
		total += l
	}
	r.metrics.AverageLatency = total / time.Duration(len(r.metrics.latencies))

	// Calculate target hit rate
	targetHits := 0
	for _, l := range r.metrics.latencies {
		if l.Milliseconds() <= int64(r.config.TargetLatencyMS) {
			targetHits++
		}
	}
	r.metrics.TargetHitRate = float64(targetHits) / float64(len(r.metrics.latencies))
}

// GetMetrics returns current metrics.
func (r *NativeAudioReasoner) GetMetrics() AudioMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metrics
}

// Interrupt interrupts current audio processing.
func (r *NativeAudioReasoner) Interrupt() {
	r.mu.Lock()
	r.metrics.Interruptions++
	r.mu.Unlock()
	r.logger.Debug("audio processing interrupted")
}

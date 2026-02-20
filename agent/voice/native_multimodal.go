package voice

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 原生AudioConfig配置了本土音频推理.
type NativeAudioConfig struct {
	TargetLatencyMS int           `json:"target_latency_ms"` // Target: 232ms
	SampleRate      int           `json:"sample_rate"`
	ChunkSizeMS     int           `json:"chunk_size_ms"`
	BufferSize      int           `json:"buffer_size"`
	EnableVAD       bool          `json:"enable_vad"`
	Timeout         time.Duration `json:"timeout"`
}

// 默认 NativeAudioConfig 返回低延迟的优化默认值。
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

// AudioFrame代表单一音频帧.
type AudioFrame struct {
	Data       []byte    `json:"data"`
	SampleRate int       `json:"sample_rate"`
	Channels   int       `json:"channels"`
	Duration   int       `json:"duration_ms"`
	Timestamp  time.Time `json:"timestamp"`
}

// 多式联运输入代表了本土音频推理的输入.
type MultimodalInput struct {
	Audio     []AudioFrame   `json:"audio,omitempty"`
	Text      string         `json:"text,omitempty"`
	Image     []byte         `json:"image,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// 多式联运输出代表了本地音频推理的输出.
type MultimodalOutput struct {
	Audio       []AudioFrame `json:"audio,omitempty"`
	Text        string       `json:"text,omitempty"`
	Transcript  string       `json:"transcript,omitempty"`
	LatencyMS   int64        `json:"latency_ms"`
	TokensUsed  int          `json:"tokens_used"`
	Confidence  float64      `json:"confidence"`
	Interrupted bool         `json:"interrupted"`
}

// 土著AudioProvider定义了本地音频模型的界面.
type NativeAudioProvider interface {
	ProcessAudio(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error)
	StreamAudio(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error)
	Name() string
}

// 土著AudioReasoner提供GPT-4o风格的本土音频推理.
type NativeAudioReasoner struct {
	provider NativeAudioProvider
	config   NativeAudioConfig
	logger   *zap.Logger
	metrics  AudioMetrics
	mu       sync.Mutex
}

// AudioMetrics追踪音频处理度量衡.
type AudioMetrics struct {
	TotalRequests  int64         `json:"total_requests"`
	AverageLatency time.Duration `json:"average_latency"`
	P95Latency     time.Duration `json:"p95_latency"`
	TargetHitRate  float64       `json:"target_hit_rate"`
	TotalAudioMS   int64         `json:"total_audio_ms"`
	Interruptions  int64         `json:"interruptions"`
	latencies      []time.Duration
}

// NewNativeAudioReasoner创造出一个新的本土音频理性.
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

// 进程用本地音频推理处理多模式输入.
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

// StreamProcess在流化模式下处理音频,以达到最小的延迟.
func (r *NativeAudioReasoner) StreamProcess(ctx context.Context, inputChan <-chan AudioFrame) (<-chan AudioFrame, error) {
	outputChan, err := r.provider.StreamAudio(ctx, inputChan)
	if err != nil {
		return nil, err
	}

	// 环绕输出通道以跟踪度量
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

	// 仅保留上千个滞期
	if len(r.metrics.latencies) > 1000 {
		r.metrics.latencies = r.metrics.latencies[1:]
	}

	// 计算平均值
	var total time.Duration
	for _, l := range r.metrics.latencies {
		total += l
	}
	r.metrics.AverageLatency = total / time.Duration(len(r.metrics.latencies))

	// 计算目标命中率
	targetHits := 0
	for _, l := range r.metrics.latencies {
		if l.Milliseconds() <= int64(r.config.TargetLatencyMS) {
			targetHits++
		}
	}
	r.metrics.TargetHitRate = float64(targetHits) / float64(len(r.metrics.latencies))
}

// GetMetrics 返回当前度量衡 。
func (r *NativeAudioReasoner) GetMetrics() AudioMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metrics
}

// 中断中断当前音频处理.
func (r *NativeAudioReasoner) Interrupt() {
	r.mu.Lock()
	r.metrics.Interruptions++
	r.mu.Unlock()
	r.logger.Debug("audio processing interrupted")
}

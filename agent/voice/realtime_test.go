package voice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/testutil"
)

// --- Inline mocks (function callback pattern) ---

type mockSTTStream struct {
	sendFn    func(chunk AudioChunk) error
	receiveFn func() <-chan TranscriptEvent
	closeFn   func() error
}

func (m *mockSTTStream) Send(chunk AudioChunk) error {
	if m.sendFn != nil {
		return m.sendFn(chunk)
	}
	return nil
}

func (m *mockSTTStream) Receive() <-chan TranscriptEvent {
	if m.receiveFn != nil {
		return m.receiveFn()
	}
	ch := make(chan TranscriptEvent)
	close(ch)
	return ch
}

func (m *mockSTTStream) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

type mockSTTProvider struct {
	startStreamFn func(ctx context.Context, sampleRate int) (STTStream, error)
	nameFn        func() string
}

func (m *mockSTTProvider) StartStream(ctx context.Context, sampleRate int) (STTStream, error) {
	if m.startStreamFn != nil {
		return m.startStreamFn(ctx, sampleRate)
	}
	return &mockSTTStream{}, nil
}

func (m *mockSTTProvider) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock-stt"
}

type mockTTSProvider struct {
	synthesizeFn       func(ctx context.Context, text string) (<-chan SpeechEvent, error)
	synthesizeStreamFn func(ctx context.Context, textChan <-chan string) (<-chan SpeechEvent, error)
	nameFn             func() string
}

func (m *mockTTSProvider) Synthesize(ctx context.Context, text string) (<-chan SpeechEvent, error) {
	if m.synthesizeFn != nil {
		return m.synthesizeFn(ctx, text)
	}
	ch := make(chan SpeechEvent)
	close(ch)
	return ch, nil
}

func (m *mockTTSProvider) SynthesizeStream(ctx context.Context, textChan <-chan string) (<-chan SpeechEvent, error) {
	if m.synthesizeStreamFn != nil {
		return m.synthesizeStreamFn(ctx, textChan)
	}
	ch := make(chan SpeechEvent)
	close(ch)
	return ch, nil
}

func (m *mockTTSProvider) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock-tts"
}

type mockLLMHandler struct {
	processStreamFn func(ctx context.Context, input string) (<-chan string, error)
}

func (m *mockLLMHandler) ProcessStream(ctx context.Context, input string) (<-chan string, error) {
	if m.processStreamFn != nil {
		return m.processStreamFn(ctx, input)
	}
	ch := make(chan string)
	close(ch)
	return ch, nil
}

type mockNativeAudioProvider struct {
	processAudioFn func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error)
	streamAudioFn  func(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error)
	nameFn         func() string
}

func (m *mockNativeAudioProvider) ProcessAudio(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
	if m.processAudioFn != nil {
		return m.processAudioFn(ctx, input)
	}
	return &MultimodalOutput{Text: "default"}, nil
}

func (m *mockNativeAudioProvider) StreamAudio(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error) {
	if m.streamAudioFn != nil {
		return m.streamAudioFn(ctx, input)
	}
	ch := make(chan AudioFrame)
	close(ch)
	return ch, nil
}

func (m *mockNativeAudioProvider) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock-native-audio"
}

// --- VoiceAgent tests ---

func TestVoiceAgent_New(t *testing.T) {
	tests := []struct {
		name          string
		logger        *zap.Logger
		expectedState VoiceState
	}{
		{
			name:          "with nil logger defaults to nop",
			logger:        nil,
			expectedState: StateIdle,
		},
		{
			name:          "with explicit logger",
			logger:        zap.NewNop(),
			expectedState: StateIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewVoiceAgent(
				DefaultVoiceConfig(),
				&mockSTTProvider{},
				&mockTTSProvider{},
				&mockLLMHandler{},
				tt.logger,
			)
			require.NotNil(t, agent)
			assert.Equal(t, tt.expectedState, agent.GetState())
		})
	}
}

func TestVoiceAgent_GetState(t *testing.T) {
	agent := NewVoiceAgent(
		DefaultVoiceConfig(),
		&mockSTTProvider{},
		&mockTTSProvider{},
		&mockLLMHandler{},
		nil,
	)

	assert.Equal(t, StateIdle, agent.GetState())

	agent.setState(StateListening)
	assert.Equal(t, StateListening, agent.GetState())

	agent.setState(StateProcessing)
	assert.Equal(t, StateProcessing, agent.GetState())
}

func TestVoiceAgent_GetMetrics(t *testing.T) {
	agent := NewVoiceAgent(
		DefaultVoiceConfig(),
		&mockSTTProvider{},
		&mockTTSProvider{},
		&mockLLMHandler{},
		nil,
	)

	metrics := agent.GetMetrics()
	assert.Equal(t, int64(0), metrics.TotalSessions)
	assert.Equal(t, int64(0), metrics.InterruptionCount)
	assert.Equal(t, float64(0), metrics.TotalAudioSeconds)
}

func TestVoiceSession_Close(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return &mockSTTStream{}, nil
		},
	}
	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)

	session, err := agent.Start(ctx)
	require.NoError(t, err)
	require.NotNil(t, session)

	err = session.Close()
	assert.NoError(t, err)
	assert.True(t, session.closed)
	assert.Equal(t, StateIdle, agent.GetState())
}

func TestVoiceSession_Close_Double(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return &mockSTTStream{}, nil
		},
	}
	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)

	session, err := agent.Start(ctx)
	require.NoError(t, err)

	err = session.Close()
	assert.NoError(t, err)

	// Second close should not panic and return nil
	err = session.Close()
	assert.NoError(t, err)
}

func TestVoiceSession_SendAudio_Closed(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return &mockSTTStream{}, nil
		},
	}
	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)

	session, err := agent.Start(ctx)
	require.NoError(t, err)

	session.Close()

	err = session.SendAudio(AudioChunk{Data: []byte("audio")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session closed")
}

func TestVoiceSession_Interrupt(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return &mockSTTStream{}, nil
		},
	}
	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)

	session, err := agent.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	session.Interrupt()
	assert.Equal(t, StateInterrupted, agent.GetState())

	metrics := agent.GetMetrics()
	assert.Equal(t, int64(1), metrics.InterruptionCount)
}

// --- NativeAudioReasoner tests ---

func TestNativeAudioReasoner_Process(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			return &MultimodalOutput{
				Text:       "transcribed text",
				TokensUsed: 42,
				Confidence: 0.95,
			}, nil
		},
	}

	cfg := DefaultNativeAudioConfig()
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	input := MultimodalInput{
		Text:      "hello",
		Timestamp: time.Now(),
	}

	output, err := reasoner.Process(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, "transcribed text", output.Text)
	assert.Equal(t, 42, output.TokensUsed)
	assert.GreaterOrEqual(t, output.LatencyMS, int64(0))
}

func TestNativeAudioReasoner_Process_Error(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			return nil, errors.New("provider failure")
		},
	}

	reasoner := NewNativeAudioReasoner(provider, DefaultNativeAudioConfig(), nil)

	_, err := reasoner.Process(ctx, MultimodalInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audio processing failed")
}

func TestNativeAudioReasoner_GetMetrics(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			return &MultimodalOutput{Text: "ok"}, nil
		},
	}

	cfg := DefaultNativeAudioConfig()
	cfg.TargetLatencyMS = 5000 // high target so all requests hit
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	// Process a few requests to populate metrics
	for i := 0; i < 3; i++ {
		_, err := reasoner.Process(ctx, MultimodalInput{Text: "test"})
		require.NoError(t, err)
	}

	metrics := reasoner.GetMetrics()
	assert.Equal(t, int64(3), metrics.TotalRequests)
	assert.Greater(t, metrics.AverageLatency, time.Duration(0))
	assert.InDelta(t, 1.0, metrics.TargetHitRate, 0.01, "all requests should hit the high target")
}

func TestNativeAudioReasoner_Interrupt(t *testing.T) {
	reasoner := NewNativeAudioReasoner(&mockNativeAudioProvider{}, DefaultNativeAudioConfig(), nil)

	reasoner.Interrupt()
	reasoner.Interrupt()

	metrics := reasoner.GetMetrics()
	assert.Equal(t, int64(2), metrics.Interruptions)
}

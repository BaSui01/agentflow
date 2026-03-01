package voice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/BaSui01/agentflow/testutil"
)

// --- VoiceAgent additional tests ---

func TestVoiceAgent_Start_STTFailure(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return nil, errors.New("stt connection failed")
		},
	}

	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)
	session, err := agent.Start(ctx)
	assert.Error(t, err)
	assert.Nil(t, session)
	assert.Contains(t, err.Error(), "failed to start STT")
}

func TestVoiceAgent_Start_IncrementsSessions(t *testing.T) {
	ctx := testutil.TestContext(t)

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return &mockSTTStream{}, nil
		},
	}

	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)

	session1, err := agent.Start(ctx)
	require.NoError(t, err)
	session1.Close()

	session2, err := agent.Start(ctx)
	require.NoError(t, err)
	session2.Close()

	metrics := agent.GetMetrics()
	assert.Equal(t, int64(2), metrics.TotalSessions)
}

func TestVoiceSession_SendAudio_BufferFull(t *testing.T) {
	ctx := testutil.TestContext(t)

	// Use a blocking STT stream that never consumes, so the audio channel fills up
	blockCh := make(chan struct{})
	blockingStream := &mockSTTStream{
		sendFn: func(chunk AudioChunk) error {
			// Block until test cleanup
			<-blockCh
			return nil
		},
	}
	t.Cleanup(func() { close(blockCh) })

	stt := &mockSTTProvider{
		startStreamFn: func(ctx context.Context, sampleRate int) (STTStream, error) {
			return blockingStream, nil
		},
	}

	agent := NewVoiceAgent(DefaultVoiceConfig(), stt, &mockTTSProvider{}, &mockLLMHandler{}, nil)
	session, err := agent.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { session.Close() })

	// The processAudio goroutine picks up the first chunk and blocks on Send.
	// After that, the channel has capacity for 100 more items.
	// We send 101 items total: 1 gets picked up by processAudio, 100 fill the buffer,
	// and the 102nd should fail.
	var lastErr error
	for i := 0; i < 200; i++ {
		lastErr = session.SendAudio(AudioChunk{Data: []byte("audio")})
		if lastErr != nil {
			break
		}
	}
	require.Error(t, lastErr)
	assert.Contains(t, lastErr.Error(), "audio buffer full")
}

func TestVoiceSession_ReceiveSpeech(t *testing.T) {
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

	ch := session.ReceiveSpeech()
	assert.NotNil(t, ch)
}

func TestVoiceSession_ID_NotEmpty(t *testing.T) {
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

	assert.NotEmpty(t, session.ID)
	assert.Contains(t, session.ID, "voice_")
}

// --- DefaultVoiceConfig tests ---

func TestDefaultVoiceConfig(t *testing.T) {
	cfg := DefaultVoiceConfig()
	assert.Equal(t, "deepgram", cfg.STTProvider)
	assert.Equal(t, "elevenlabs", cfg.TTSProvider)
	assert.Equal(t, 16000, cfg.SampleRate)
	assert.Equal(t, 300, cfg.MaxLatencyMS)
	assert.True(t, cfg.VADEnabled)
	assert.True(t, cfg.InterruptEnabled)
	assert.Equal(t, 100*time.Millisecond, cfg.BufferDuration)
}

// --- DefaultNativeAudioConfig tests ---

func TestDefaultNativeAudioConfig(t *testing.T) {
	cfg := DefaultNativeAudioConfig()
	assert.Equal(t, 232, cfg.TargetLatencyMS)
	assert.Equal(t, 24000, cfg.SampleRate)
	assert.Equal(t, 20, cfg.ChunkSizeMS)
	assert.Equal(t, 4096, cfg.BufferSize)
	assert.True(t, cfg.EnableVAD)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
}

// --- NativeAudioReasoner StreamProcess tests ---

func TestNativeAudioReasoner_StreamProcess(t *testing.T) {
	ctx := testutil.TestContext(t)

	outputFrames := []AudioFrame{
		{Data: []byte("frame1"), Duration: 20},
		{Data: []byte("frame2"), Duration: 20},
	}

	provider := &mockNativeAudioProvider{
		streamAudioFn: func(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error) {
			ch := make(chan AudioFrame, len(outputFrames))
			go func() {
				defer close(ch)
				for _, f := range outputFrames {
					ch <- f
				}
			}()
			return ch, nil
		},
	}

	cfg := DefaultNativeAudioConfig()
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	inputCh := make(chan AudioFrame)
	close(inputCh)

	outputCh, err := reasoner.StreamProcess(ctx, inputCh)
	require.NoError(t, err)

	var received []AudioFrame
	for frame := range outputCh {
		received = append(received, frame)
	}

	assert.Len(t, received, 2)

	metrics := reasoner.GetMetrics()
	assert.Equal(t, int64(40), metrics.TotalAudioMS)
}

func TestNativeAudioReasoner_StreamProcess_Error(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		streamAudioFn: func(ctx context.Context, input <-chan AudioFrame) (<-chan AudioFrame, error) {
			return nil, errors.New("stream failed")
		},
	}

	reasoner := NewNativeAudioReasoner(provider, DefaultNativeAudioConfig(), nil)

	inputCh := make(chan AudioFrame)
	close(inputCh)

	_, err := reasoner.StreamProcess(ctx, inputCh)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stream failed")
}

// --- NativeAudioReasoner Process with context timeout ---

func TestNativeAudioReasoner_Process_ContextTimeout(t *testing.T) {
	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	cfg := DefaultNativeAudioConfig()
	cfg.Timeout = 50 * time.Millisecond
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	ctx := context.Background()
	_, err := reasoner.Process(ctx, MultimodalInput{Text: "test"})
	assert.Error(t, err)
}

// --- NativeAudioReasoner metrics latency window ---

func TestNativeAudioReasoner_MetricsLatencyWindow(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			return &MultimodalOutput{Text: "ok"}, nil
		},
	}

	cfg := DefaultNativeAudioConfig()
	cfg.TargetLatencyMS = 10000
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	// Process many requests to test the latency window cap (1000)
	for i := 0; i < 5; i++ {
		_, err := reasoner.Process(ctx, MultimodalInput{Text: "test"})
		require.NoError(t, err)
	}

	metrics := reasoner.GetMetrics()
	assert.Equal(t, int64(5), metrics.TotalRequests)
	assert.Greater(t, metrics.AverageLatency, time.Duration(0))
}

// --- VoiceAgent state transitions ---

func TestVoiceAgent_StateTransitions(t *testing.T) {
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

	agent.setState(StateSpeaking)
	assert.Equal(t, StateSpeaking, agent.GetState())

	agent.setState(StateInterrupted)
	assert.Equal(t, StateInterrupted, agent.GetState())

	agent.setState(StateIdle)
	assert.Equal(t, StateIdle, agent.GetState())
}

// --- NativeAudioReasoner concurrent Process ---

func TestNativeAudioReasoner_ConcurrentProcess(t *testing.T) {
	ctx := testutil.TestContext(t)

	provider := &mockNativeAudioProvider{
		processAudioFn: func(ctx context.Context, input MultimodalInput) (*MultimodalOutput, error) {
			return &MultimodalOutput{Text: "ok"}, nil
		},
	}

	cfg := DefaultNativeAudioConfig()
	cfg.TargetLatencyMS = 10000
	reasoner := NewNativeAudioReasoner(provider, cfg, nil)

	done := make(chan struct{})
	const n = 20

	for i := 0; i < n; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, err := reasoner.Process(ctx, MultimodalInput{Text: "test"})
			require.NoError(t, err)
		}()
	}

	for i := 0; i < n; i++ {
		<-done
	}

	metrics := reasoner.GetMetrics()
	assert.Equal(t, int64(n), metrics.TotalRequests)
}


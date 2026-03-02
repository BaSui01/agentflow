package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Config tests ---

func TestDefaultOpenAITTSConfig(t *testing.T) {
	cfg := DefaultOpenAITTSConfig()
	assert.Equal(t, "https://api.openai.com", cfg.BaseURL)
	assert.Equal(t, "tts-1-hd", cfg.Model)
	assert.Equal(t, "alloy", cfg.Voice)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

func TestDefaultOpenAISTTConfig(t *testing.T) {
	cfg := DefaultOpenAISTTConfig()
	assert.Equal(t, "https://api.openai.com", cfg.BaseURL)
	assert.Equal(t, "whisper-1", cfg.Model)
	assert.Equal(t, 120*time.Second, cfg.Timeout)
}

func TestDefaultElevenLabsConfig(t *testing.T) {
	cfg := DefaultElevenLabsConfig()
	assert.Equal(t, "https://api.elevenlabs.io", cfg.BaseURL)
	assert.Equal(t, "eleven_multilingual_v2", cfg.Model)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
}

func TestDefaultDeepgramConfig(t *testing.T) {
	cfg := DefaultDeepgramConfig()
	assert.Equal(t, "https://api.deepgram.com", cfg.BaseURL)
	assert.Equal(t, "nova-2", cfg.Model)
	assert.Equal(t, 120*time.Second, cfg.Timeout)
}

// --- OpenAI TTS Provider tests ---

func TestNewOpenAITTSProvider(t *testing.T) {
	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "openai-tts", p.Name())
	assert.Equal(t, "https://api.openai.com", p.cfg.BaseURL)
	assert.Equal(t, "tts-1-hd", p.cfg.Model)
	assert.Equal(t, "alloy", p.cfg.Voice)
}

func TestOpenAITTSProvider_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/speech", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req openAITTSRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "Hello world", req.Input)
		assert.Equal(t, "tts-1-hd", req.Model)

		_, _ = w.Write([]byte("fake-audio-data"))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	resp, err := p.Synthesize(context.Background(), &TTSRequest{Text: "Hello world"})
	require.NoError(t, err)
	assert.Equal(t, "openai-tts", resp.Provider)
	assert.Equal(t, "tts-1-hd", resp.Model)
	assert.Equal(t, "mp3", resp.Format)
	assert.Equal(t, 11, resp.CharCount)

	data, err := io.ReadAll(resp.Audio)
	require.NoError(t, err)
	resp.Audio.Close()
	assert.Equal(t, "fake-audio-data", string(data))
}

func TestOpenAITTSProvider_Synthesize_CustomParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAITTSRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "custom-model", req.Model)
		assert.Equal(t, "nova", req.Voice)
		assert.Equal(t, "opus", req.ResponseFormat)
		assert.Equal(t, 1.5, req.Speed)
		_, _ = w.Write([]byte("audio"))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Synthesize(context.Background(), &TTSRequest{
		Text:           "test",
		Model:          "custom-model",
		Voice:          "nova",
		ResponseFormat: "opus",
		Speed:          1.5,
	})
	require.NoError(t, err)
}

func TestOpenAITTSProvider_Synthesize_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Synthesize(context.Background(), &TTSRequest{Text: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "openai tts error")
}

func TestOpenAITTSProvider_SynthesizeToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("audio-bytes"))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	outPath := filepath.Join(dir, "output.mp3")

	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	err := p.SynthesizeToFile(context.Background(), &TTSRequest{Text: "test"}, outPath)
	require.NoError(t, err)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Equal(t, "audio-bytes", string(data))
}

func TestOpenAITTSProvider_ListVoices(t *testing.T) {
	p := NewOpenAITTSProvider(OpenAITTSConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	voices, err := p.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Len(t, voices, 6)
	assert.Equal(t, "alloy", voices[0].ID)
}

// --- ElevenLabs Provider tests ---

func TestNewElevenLabsProvider(t *testing.T) {
	p := NewElevenLabsProvider(ElevenLabsConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "elevenlabs", p.Name())
	assert.Equal(t, "https://api.elevenlabs.io", p.cfg.BaseURL)
	assert.Equal(t, "eleven_multilingual_v2", p.cfg.Model)
}

func TestElevenLabsProvider_Synthesize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/text-to-speech/")
		assert.Equal(t, "test-key", r.Header.Get("xi-api-key"))

		var req elevenLabsTTSRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "Hello", req.Text)

		_, _ = w.Write([]byte("eleven-audio"))
	}))
	t.Cleanup(srv.Close)

	p := NewElevenLabsProvider(ElevenLabsConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	resp, err := p.Synthesize(context.Background(), &TTSRequest{Text: "Hello"})
	require.NoError(t, err)
	assert.Equal(t, "elevenlabs", resp.Provider)
	assert.Equal(t, "mp3", resp.Format)

	data, err := io.ReadAll(resp.Audio)
	require.NoError(t, err)
	resp.Audio.Close()
	assert.Equal(t, "eleven-audio", string(data))
}

func TestElevenLabsProvider_Synthesize_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewElevenLabsProvider(ElevenLabsConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad", BaseURL: srv.URL}})
	_, err := p.Synthesize(context.Background(), &TTSRequest{Text: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "elevenlabs error")
}

func TestElevenLabsProvider_ListVoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/voices", r.URL.Path)
		resp := elevenLabsVoicesResponse{
			Voices: []elevenLabsVoice{
				{
					VoiceID:    "v1",
					Name:       "Rachel",
					PreviewURL: "https://example.com/preview.mp3",
				},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewElevenLabsProvider(ElevenLabsConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	voices, err := p.ListVoices(context.Background())
	require.NoError(t, err)
	assert.Len(t, voices, 1)
	assert.Equal(t, "v1", voices[0].ID)
	assert.Equal(t, "Rachel", voices[0].Name)
}

func TestElevenLabsProvider_ListVoices_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"forbidden"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewElevenLabsProvider(ElevenLabsConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.ListVoices(context.Background())
	assert.Error(t, err)
}

// --- OpenAI STT Provider tests ---

func TestNewOpenAISTTProvider(t *testing.T) {
	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "openai-stt", p.Name())
	assert.Equal(t, "whisper-1", p.cfg.Model)
	assert.Equal(t, []string{"flac", "m4a", "mp3", "mp4", "mpeg", "mpga", "oga", "ogg", "wav", "webm"}, p.SupportedFormats())
}

func TestOpenAISTTProvider_Transcribe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/audio/transcriptions", r.URL.Path)
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")

		resp := whisperResponse{
			Text:     "Hello world",
			Language: "en",
			Duration: 2.5,
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Transcribe(context.Background(), &STTRequest{
		Audio: bytes.NewReader([]byte("fake-audio")),
	})
	require.NoError(t, err)
	assert.Equal(t, "openai-stt", resp.Provider)
	assert.Equal(t, "Hello world", resp.Text)
	assert.Equal(t, "en", resp.Language)
}

func TestOpenAISTTProvider_Transcribe_NoAudio(t *testing.T) {
	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.Transcribe(context.Background(), &STTRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "audio input is required")
}

func TestOpenAISTTProvider_Transcribe_WithSegmentsAndWords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := whisperResponse{
			Text: "Hello world",
			Segments: []struct {
				ID               int     `json:"id"`
				Start            float64 `json:"start"`
				End              float64 `json:"end"`
				Text             string  `json:"text"`
				AvgLogprob       float64 `json:"avg_logprob,omitempty"`
				CompressionRatio float64 `json:"compression_ratio,omitempty"`
				NoSpeechProb     float64 `json:"no_speech_prob,omitempty"`
			}{
				{ID: 0, Start: 0.0, End: 1.0, Text: "Hello"},
				{ID: 1, Start: 1.0, End: 2.0, Text: "world"},
			},
			Words: []struct {
				Word  string  `json:"word"`
				Start float64 `json:"start"`
				End   float64 `json:"end"`
			}{
				{Word: "Hello", Start: 0.0, End: 0.5},
				{Word: "world", Start: 0.5, End: 1.0},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Transcribe(context.Background(), &STTRequest{
		Audio: bytes.NewReader([]byte("audio")),
	})
	require.NoError(t, err)
	assert.Len(t, resp.Segments, 2)
	assert.Len(t, resp.Words, 2)
	assert.Equal(t, "Hello", resp.Words[0].Word)
}

func TestOpenAISTTProvider_Transcribe_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	_, err := p.Transcribe(context.Background(), &STTRequest{
		Audio: bytes.NewReader([]byte("audio")),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "whisper error")
}

func TestOpenAISTTProvider_TranscribeFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := whisperResponse{Text: "transcribed"}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("fake-audio"), 0644))

	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.TranscribeFile(context.Background(), audioPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "transcribed", resp.Text)
}

func TestOpenAISTTProvider_TranscribeFile_NotFound(t *testing.T) {
	p := NewOpenAISTTProvider(OpenAISTTConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.TranscribeFile(context.Background(), "/nonexistent/audio.mp3", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

// --- Deepgram Provider tests ---

func TestNewDeepgramProvider(t *testing.T) {
	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key"}})
	assert.Equal(t, "deepgram", p.Name())
	assert.Equal(t, "nova-2", p.cfg.Model)
	assert.Contains(t, p.SupportedFormats(), "mp3")
}

func TestDeepgramProvider_Transcribe_WithAudio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1/listen")
		assert.Equal(t, "Token test-key", r.Header.Get("Authorization"))

		resp := map[string]any{
			"metadata": map[string]any{"duration": 3.5},
			"results": map[string]any{
				"channels": []any{
					map[string]any{
						"alternatives": []any{
							map[string]any{
								"transcript": "Hello world",
								"confidence": 0.95,
								"words": []any{
									map[string]any{"word": "Hello", "start": 0.0, "end": 0.5, "confidence": 0.98},
								},
							},
						},
					},
				},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "test-key", BaseURL: srv.URL}})
	resp, err := p.Transcribe(context.Background(), &STTRequest{
		Audio: bytes.NewReader([]byte("audio-data")),
	})
	require.NoError(t, err)
	assert.Equal(t, "deepgram", resp.Provider)
	assert.Equal(t, "Hello world", resp.Text)
	assert.Equal(t, 0.95, resp.Confidence)
}

func TestDeepgramProvider_Transcribe_WithURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]string
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "https://example.com/audio.mp3", body["url"])

		resp := map[string]any{
			"metadata": map[string]any{"duration": 1.0},
			"results":  map[string]any{"channels": []any{}},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.Transcribe(context.Background(), &STTRequest{
		AudioURL: "https://example.com/audio.mp3",
	})
	require.NoError(t, err)
	assert.Equal(t, "deepgram", resp.Provider)
}

func TestDeepgramProvider_Transcribe_NoInput(t *testing.T) {
	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.Transcribe(context.Background(), &STTRequest{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "audio input or URL is required")
}

func TestDeepgramProvider_Transcribe_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"err_msg":"unauthorized"}`))
	}))
	t.Cleanup(srv.Close)

	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "bad", BaseURL: srv.URL}})
	_, err := p.Transcribe(context.Background(), &STTRequest{
		Audio: bytes.NewReader([]byte("audio")),
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deepgram error")
}

func TestDeepgramProvider_TranscribeFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"metadata": map[string]any{"duration": 1.0},
			"results": map[string]any{
				"channels": []any{
					map[string]any{
						"alternatives": []any{
							map[string]any{"transcript": "file content", "confidence": 0.9},
						},
					},
				},
			},
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	audioPath := filepath.Join(dir, "test.mp3")
	require.NoError(t, os.WriteFile(audioPath, []byte("audio"), 0644))

	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k", BaseURL: srv.URL}})
	resp, err := p.TranscribeFile(context.Background(), audioPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "file content", resp.Text)
}

func TestDeepgramProvider_TranscribeFile_NotFound(t *testing.T) {
	p := NewDeepgramProvider(DeepgramConfig{BaseProviderConfig: providers.BaseProviderConfig{APIKey: "k"}})
	_, err := p.TranscribeFile(context.Background(), "/nonexistent/audio.mp3", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open file")
}

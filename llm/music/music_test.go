package music

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Config Tests ---

func TestDefaultSunoConfig(t *testing.T) {
	cfg := DefaultSunoConfig()
	assert.Equal(t, "https://api.sunoapi.com/v1", cfg.BaseURL)
	assert.Equal(t, "suno-v5", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

func TestDefaultMiniMaxMusicConfig(t *testing.T) {
	cfg := DefaultMiniMaxMusicConfig()
	assert.Equal(t, "https://api.minimax.io", cfg.BaseURL)
	assert.Equal(t, "music-01", cfg.Model)
	assert.Equal(t, 300*time.Second, cfg.Timeout)
}

// --- Constructor Tests ---

func TestNewSunoProvider(t *testing.T) {
	p := NewSunoProvider(SunoConfig{APIKey: "key"})
	require.NotNil(t, p)
	assert.Equal(t, "https://api.sunoapi.com/v1", p.cfg.BaseURL)
	assert.Equal(t, "suno-v5", p.cfg.Model)
}

func TestNewSunoProvider_CustomConfig(t *testing.T) {
	p := NewSunoProvider(SunoConfig{
		APIKey:  "key",
		BaseURL: "https://custom.suno.com",
		Model:   "custom-model",
		Timeout: 10 * time.Second,
	})
	assert.Equal(t, "https://custom.suno.com", p.cfg.BaseURL)
	assert.Equal(t, "custom-model", p.cfg.Model)
}

func TestSunoProvider_Name(t *testing.T) {
	assert.Equal(t, "suno", NewSunoProvider(SunoConfig{}).Name())
}

func TestNewMiniMaxProvider(t *testing.T) {
	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "key"})
	require.NotNil(t, p)
	assert.Equal(t, "https://api.minimax.io", p.cfg.BaseURL)
	assert.Equal(t, "music-01", p.cfg.Model)
}

func TestNewMiniMaxProvider_CustomConfig(t *testing.T) {
	p := NewMiniMaxProvider(MiniMaxMusicConfig{
		APIKey:  "key",
		BaseURL: "https://custom.minimax.com",
		Model:   "custom-model",
		Timeout: 10 * time.Second,
	})
	assert.Equal(t, "https://custom.minimax.com", p.cfg.BaseURL)
	assert.Equal(t, "custom-model", p.cfg.Model)
}

func TestMiniMaxProvider_Name(t *testing.T) {
	assert.Equal(t, "minimax-music", NewMiniMaxProvider(MiniMaxMusicConfig{}).Name())
}

// --- Interface Compliance ---

func TestSunoProvider_ImplementsMusicProvider(t *testing.T) {
	var _ MusicProvider = (*SunoProvider)(nil)
}

func TestMiniMaxProvider_ImplementsMusicProvider(t *testing.T) {
	var _ MusicProvider = (*MiniMaxProvider)(nil)
}

// --- Suno Generate Tests ---

func TestSunoProvider_Generate_Completed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req sunoRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "a happy song", req.Prompt)
		assert.Equal(t, "pop", req.Style)

		resp := sunoResponse{
			Status: "completed",
			Data: []struct {
				ID       string  `json:"id"`
				AudioURL string  `json:"audio_url"`
				Duration float64 `json:"duration"`
				Title    string  `json:"title"`
				Lyrics   string  `json:"lyrics"`
				Style    string  `json:"style"`
			}{
				{ID: "track-1", AudioURL: "https://example.com/track1.mp3", Duration: 180.0, Title: "Happy Song", Style: "pop"},
				{ID: "track-2", AudioURL: "https://example.com/track2.mp3", Duration: 120.0, Title: "Happy Song v2", Style: "pop"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	p := NewSunoProvider(SunoConfig{APIKey: "test-key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "a happy song",
		Style:  "pop",
	})
	require.NoError(t, err)
	assert.Equal(t, "suno", result.Provider)
	assert.Equal(t, "suno-v5", result.Model)
	require.Len(t, result.Tracks, 2)
	assert.Equal(t, "track-1", result.Tracks[0].ID)
	assert.Equal(t, "https://example.com/track1.mp3", result.Tracks[0].URL)
	assert.Equal(t, 180.0, result.Tracks[0].Duration)
	assert.Equal(t, 2, result.Usage.TracksGenerated)
	assert.InDelta(t, 300.0, result.Usage.DurationSeconds, 0.01)
}

func TestSunoProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, err := w.Write([]byte("rate limited"))
		require.NoError(t, err)
	}))
	defer server.Close()

	p := NewSunoProvider(SunoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=429")
}

func TestSunoProvider_Generate_CustomModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req sunoRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "custom-model", req.Model)

		resp := sunoResponse{Status: "completed"}
		w.Header().Set("Content-Type", "application/json")
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	p := NewSunoProvider(SunoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "custom-model",
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-model", result.Model)
}

func TestSunoProvider_Generate_Pending_ThenCompleted(t *testing.T) {
	// First call returns pending, poll returns completed
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := sunoResponse{TaskID: "task-1", Status: "pending"}
			err := json.NewEncoder(w).Encode(resp)
			require.NoError(t, err)
			return
		}
		// GET poll
		resp := sunoResponse{
			Status: "completed",
			Data: []struct {
				ID       string  `json:"id"`
				AudioURL string  `json:"audio_url"`
				Duration float64 `json:"duration"`
				Title    string  `json:"title"`
				Lyrics   string  `json:"lyrics"`
				Style    string  `json:"style"`
			}{
				{ID: "track-1", AudioURL: "https://example.com/track.mp3", Duration: 60.0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewSunoProvider(SunoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	require.NoError(t, err)
	require.Len(t, result.Tracks, 1)
}

func TestSunoProvider_Generate_PollFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := sunoResponse{TaskID: "task-fail", Status: "pending"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := sunoResponse{Status: "failed"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewSunoProvider(SunoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

// --- MiniMax Generate Tests ---

func TestMiniMaxProvider_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req miniMaxMusicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "music-01", req.Model)
		assert.Equal(t, "a jazz tune", req.Prompt)

		resp := miniMaxMusicResponse{}
		resp.BaseResp.StatusCode = 0
		resp.Data.Audio = "base64audiodata"
		resp.ExtraInfo.AudioLength = 120.5
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "test-key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "a jazz tune"})
	require.NoError(t, err)
	assert.Equal(t, "minimax-music", result.Provider)
	assert.Equal(t, "music-01", result.Model)
	require.Len(t, result.Tracks, 1)
	assert.Equal(t, "base64audiodata", result.Tracks[0].B64Audio)
	assert.InDelta(t, 120.5, result.Tracks[0].Duration, 0.01)
	assert.Equal(t, 1, result.Usage.TracksGenerated)
	assert.InDelta(t, 120.5, result.Usage.DurationSeconds, 0.01)
}

func TestMiniMaxProvider_Generate_WithReferenceAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req miniMaxMusicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "ref-audio-data", req.ReferenceAudio)

		resp := miniMaxMusicResponse{}
		resp.BaseResp.StatusCode = 0
		resp.Data.Audio = "audio"
		resp.ExtraInfo.AudioLength = 60.0
		err = json.NewEncoder(w).Encode(resp)
		require.NoError(t, err)
	}))
	defer server.Close()

	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	result, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:         "test",
		ReferenceAudio: "ref-audio-data",
	})
	require.NoError(t, err)
	require.Len(t, result.Tracks, 1)
}

func TestMiniMaxProvider_Generate_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := miniMaxMusicResponse{}
		resp.BaseResp.StatusCode = 1001
		resp.BaseResp.StatusMsg = "invalid request"
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid request")
}

func TestMiniMaxProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, err := w.Write([]byte("server error"))
		require.NoError(t, err)
	}))
	defer server.Close()

	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=500")
}

func TestMiniMaxProvider_Generate_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	p := NewMiniMaxProvider(MiniMaxMusicConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
}

package threed

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

func TestDefaultMeshyConfig(t *testing.T) {
	cfg := DefaultMeshyConfig()
	assert.Equal(t, "https://api.meshy.ai/v2", cfg.BaseURL)
	assert.Equal(t, "meshy-4", cfg.Model)
	assert.Equal(t, 600*time.Second, cfg.Timeout)
}

func TestDefaultTripoConfig(t *testing.T) {
	cfg := DefaultTripoConfig()
	assert.Equal(t, "https://api.tripo3d.ai/v2", cfg.BaseURL)
	assert.Equal(t, "tripo-v2.5", cfg.Model)
	assert.Equal(t, 600*time.Second, cfg.Timeout)
}

// --- Constructor Tests ---

func TestNewMeshyProvider(t *testing.T) {
	p := NewMeshyProvider(MeshyConfig{APIKey: "key"})
	require.NotNil(t, p)
	assert.Equal(t, "https://api.meshy.ai/v2", p.cfg.BaseURL)
}

func TestNewMeshyProvider_CustomConfig(t *testing.T) {
	p := NewMeshyProvider(MeshyConfig{
		APIKey:  "key",
		BaseURL: "https://custom.api.com",
		Timeout: 10 * time.Second,
	})
	assert.Equal(t, "https://custom.api.com", p.cfg.BaseURL)
}

func TestMeshyProvider_Name(t *testing.T) {
	assert.Equal(t, "meshy", NewMeshyProvider(MeshyConfig{}).Name())
}

func TestNewTripoProvider(t *testing.T) {
	p := NewTripoProvider(TripoConfig{APIKey: "key"})
	require.NotNil(t, p)
	assert.Equal(t, "https://api.tripo3d.ai/v2", p.cfg.BaseURL)
}

func TestNewTripoProvider_CustomConfig(t *testing.T) {
	p := NewTripoProvider(TripoConfig{
		APIKey:  "key",
		BaseURL: "https://custom.tripo.com",
		Timeout: 10 * time.Second,
	})
	assert.Equal(t, "https://custom.tripo.com", p.cfg.BaseURL)
}

func TestTripoProvider_Name(t *testing.T) {
	assert.Equal(t, "tripo3d", NewTripoProvider(TripoConfig{}).Name())
}

// --- Interface Compliance ---

func TestMeshyProvider_ImplementsThreeDProvider(t *testing.T) {
	var _ ThreeDProvider = (*MeshyProvider)(nil)
}

func TestTripoProvider_ImplementsThreeDProvider(t *testing.T) {
	var _ ThreeDProvider = (*TripoProvider)(nil)
}

// --- Meshy HTTP Error Tests (no polling) ---

func TestMeshyProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=500")
}

func TestMeshyProvider_Generate_ImageHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad"))
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{ImageURL: "https://example.com/img.png"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=400")
}

// --- Tripo HTTP Error Tests (no polling) ---

func TestTripoProvider_Generate_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	p := NewTripoProvider(TripoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status=400")
}

func TestTripoProvider_Generate_ErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tripoResponse{Code: 1001}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewTripoProvider(TripoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error code")
}

// --- Meshy Polling Tests (shared server, immediate success) ---

// newMeshySuccessServer returns a server that accepts POST for task creation
// and returns SUCCEEDED on the first GET poll.
func newMeshySuccessServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := meshyTaskResponse{Result: "task-1"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// GET — always return SUCCEEDED
		resp := meshyTaskResponse{
			Status:       "SUCCEEDED",
			ThumbnailURL: "https://example.com/thumb.png",
			ModelURLs: struct {
				GLB  string `json:"glb"`
				FBX  string `json:"fbx"`
				OBJ  string `json:"obj"`
				USDZ string `json:"usdz"`
			}{
				GLB:  "https://example.com/model.glb",
				FBX:  "https://example.com/model.fbx",
				OBJ:  "https://example.com/model.obj",
				USDZ: "https://example.com/model.usdz",
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestMeshyProvider_Generate_TextTo3D(t *testing.T) {
	server := newMeshySuccessServer(t)
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "test-key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := p.Generate(ctx, &GenerateRequest{Prompt: "a cute cat"})
	require.NoError(t, err)
	assert.Equal(t, "meshy", result.Provider)
	require.Len(t, result.Models, 1)
	assert.Equal(t, "glb", result.Models[0].Format)
	assert.Equal(t, "https://example.com/model.glb", result.Models[0].URL)
	assert.Equal(t, 1, result.Usage.ModelsGenerated)
}

func TestMeshyProvider_Generate_ImageTo3D(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			// Verify it hits image-to-3d endpoint
			assert.Equal(t, "/image-to-3d", r.URL.Path)
			resp := meshyTaskResponse{Result: "task-img"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := meshyTaskResponse{Status: "SUCCEEDED", ModelURLs: struct {
			GLB  string `json:"glb"`
			FBX  string `json:"fbx"`
			OBJ  string `json:"obj"`
			USDZ string `json:"usdz"`
		}{GLB: "https://example.com/model.glb"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test with ImageURL
	result, err := p.Generate(ctx, &GenerateRequest{ImageURL: "https://example.com/image.png"})
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
}

func TestMeshyProvider_Generate_HighQualityAndBase64(t *testing.T) {
	// Tests both high quality mode (refine) and base64 image input
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" && r.URL.Path == "/text-to-3d" {
			var req meshyTextTo3DRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Equal(t, "refine", req.Mode)
			resp := meshyTaskResponse{Result: "task-hq"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/image-to-3d" {
			var req meshyImageTo3DRequest
			json.NewDecoder(r.Body).Decode(&req)
			assert.Contains(t, req.ImageURL, "data:image/png;base64,")
			resp := meshyTaskResponse{Result: "task-b64"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := meshyTaskResponse{Status: "SUCCEEDED", ModelURLs: struct {
			GLB  string `json:"glb"`
			FBX  string `json:"fbx"`
			OBJ  string `json:"obj"`
			USDZ string `json:"usdz"`
		}{GLB: "https://example.com/model.glb"}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test high quality
	result, err := p.Generate(ctx, &GenerateRequest{Prompt: "test", Quality: "high"})
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/model.glb", result.Models[0].URL)
}

func TestMeshyProvider_Generate_PollFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := meshyTaskResponse{Result: "task-fail"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := meshyTaskResponse{Status: "FAILED"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

func TestMeshyProvider_Generate_PollContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := meshyTaskResponse{Result: "task-cancel"}
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := meshyTaskResponse{Status: "PENDING"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewMeshyProvider(MeshyConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
}

// --- Tripo Polling Tests ---

func newTripoSuccessServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := tripoResponse{}
			resp.Data.TaskID = "tripo-task-1"
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := tripoResponse{}
		resp.Data.Status = "success"
		resp.Data.Output.Model = "https://example.com/model.glb"
		resp.Data.Output.RenderedImage = "https://example.com/thumb.png"
		json.NewEncoder(w).Encode(resp)
	}))
}

func TestTripoProvider_Generate_TextToModel(t *testing.T) {
	server := newTripoSuccessServer(t)
	defer server.Close()

	p := NewTripoProvider(TripoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := p.Generate(ctx, &GenerateRequest{Prompt: "a robot"})
	require.NoError(t, err)
	assert.Equal(t, "tripo3d", result.Provider)
	require.Len(t, result.Models, 1)
	assert.Equal(t, "glb", result.Models[0].Format)
	assert.Equal(t, "https://example.com/model.glb", result.Models[0].URL)
	assert.Equal(t, "https://example.com/thumb.png", result.Models[0].ThumbnailURL)
}

func TestTripoProvider_Generate_ImageVariants(t *testing.T) {
	// Tests image_to_model (URL), image_to_model (base64), and multiview_to_model
	var lastType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			var req tripoRequest
			json.NewDecoder(r.Body).Decode(&req)
			lastType = req.Type
			resp := tripoResponse{}
			resp.Data.TaskID = "tripo-img"
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := tripoResponse{}
		resp.Data.Status = "success"
		resp.Data.Output.Model = "https://example.com/model.glb"
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewTripoProvider(TripoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test URL image
	_, err := p.Generate(ctx, &GenerateRequest{ImageURL: "https://example.com/img.png"})
	require.NoError(t, err)
	assert.Equal(t, "image_to_model", lastType)
}

func TestTripoProvider_Generate_PollFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			resp := tripoResponse{}
			resp.Data.TaskID = "tripo-fail"
			json.NewEncoder(w).Encode(resp)
			return
		}
		resp := tripoResponse{}
		resp.Data.Status = "failed"
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewTripoProvider(TripoConfig{APIKey: "key", BaseURL: server.URL})
	p.client = server.Client()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, &GenerateRequest{Prompt: "test"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
}

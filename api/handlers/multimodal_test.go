package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/video"
	"github.com/BaSui01/agentflow/types"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockImageProvider struct {
	generateCalled bool
	editCalled     bool
	lastEditReq    *image.EditRequest
}

func (m *mockImageProvider) Generate(ctx context.Context, req *image.GenerateRequest) (*image.GenerateResponse, error) {
	m.generateCalled = true
	return &image.GenerateResponse{
		Provider: "mock-image",
		Model:    "mock",
		Images: []image.ImageData{
			{URL: "https://example.com/image.png"},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockImageProvider) Edit(ctx context.Context, req *image.EditRequest) (*image.GenerateResponse, error) {
	m.editCalled = true
	m.lastEditReq = req
	return &image.GenerateResponse{
		Provider: "mock-image",
		Model:    "mock",
		Images: []image.ImageData{
			{URL: "https://example.com/edited.png"},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockImageProvider) CreateVariation(ctx context.Context, req *image.VariationRequest) (*image.GenerateResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockImageProvider) Name() string { return "mock-image" }

func (m *mockImageProvider) SupportedSizes() []string { return []string{"1024x1024"} }

type mockVideoProvider struct {
	lastReq *video.GenerateRequest
}

func (m *mockVideoProvider) Analyze(ctx context.Context, req *video.AnalyzeRequest) (*video.AnalyzeResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockVideoProvider) Generate(ctx context.Context, req *video.GenerateRequest) (*video.GenerateResponse, error) {
	m.lastReq = req
	return &video.GenerateResponse{
		Provider: "mock-video",
		Model:    "mock",
		Videos: []video.VideoData{
			{URL: "https://example.com/video.mp4"},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockVideoProvider) Name() string { return "mock-video" }

func (m *mockVideoProvider) SupportedFormats() []video.VideoFormat {
	return []video.VideoFormat{video.VideoFormatMP4}
}

func (m *mockVideoProvider) SupportsGeneration() bool { return true }

type mockLLMProvider struct{}

func (m *mockLLMProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	return &llm.ChatResponse{
		ID:    "mock",
		Model: "gpt-4o-mini",
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: `{"goal":"x","shots":[{"id":1,"purpose":"p","visual":"v","action":"a","camera":"c","duration_sec":3}]}`,
				},
			},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m *mockLLMProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *mockLLMProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *mockLLMProvider) Name() string { return "mock-llm" }

func (m *mockLLMProvider) SupportsNativeFunctionCalling() bool { return true }

func (m *mockLLMProvider) ListModels(ctx context.Context) ([]llm.Model, error) { return nil, nil }

func (m *mockLLMProvider) Endpoints() llm.ProviderEndpoints { return llm.ProviderEndpoints{} }

type failingReferenceStore struct{}

func (s *failingReferenceStore) Save(asset *referenceAsset) error {
	_ = asset
	return errors.New("reference store unavailable")
}

func (s *failingReferenceStore) Get(id string) (*referenceAsset, bool) {
	_ = id
	return nil, false
}

func (s *failingReferenceStore) Delete(id string) {
	_ = id
}

func (s *failingReferenceStore) Cleanup(expireBefore time.Time) {
	_ = expireBefore
}

func TestMultimodalHandler_ImageReferenceFlow(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	vdo := &mockVideoProvider{}
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{"mock": img},
		map[string]video.Provider{"runway": vdo},
		"mock",
		"runway",
		nil,
		0,
		0,
		nil,
		logger,
	)

	refID := uploadTestReference(t, h)

	body := map[string]any{
		"prompt":       "a cat",
		"provider":     "mock",
		"reference_id": refID,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/image", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandleImage(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, img.editCalled)
	assert.False(t, img.generateCalled)
}

func TestMultimodalHandler_VideoReferenceFlow(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	vdo := &mockVideoProvider{}
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{"mock": img},
		map[string]video.Provider{"runway": vdo},
		"mock",
		"runway",
		nil,
		0,
		0,
		nil,
		logger,
	)

	refID := uploadTestReference(t, h)

	body := map[string]any{
		"prompt":       "a moving camera scene",
		"provider":     "runway",
		"reference_id": refID,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/video", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandleVideo(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, vdo.lastReq)
	assert.True(t, strings.HasPrefix(vdo.lastReq.ImageURL, "data:image/"))
}

func TestMultimodalHandler_PlanUnknownField(t *testing.T) {
	logger := zap.NewNop()
	h := NewMultimodalHandlerWithProviders(
		&mockLLMProvider{},
		map[string]image.Provider{},
		map[string]video.Provider{},
		"",
		"",
		nil,
		0,
		0,
		nil,
		logger,
	)

	raw := []byte(`{"prompt":"test","unknown_field":"x"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/plan", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandlePlan(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMultimodalHandler_Capabilities(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	vdo := &mockVideoProvider{}
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{"mock": img},
		map[string]video.Provider{"runway": vdo},
		"mock",
		"runway",
		nil,
		0,
		0,
		nil,
		logger,
	)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/multimodal/capabilities", nil)
	h.HandleCapabilities(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMultimodalHandler_DefaultProviderRespected(t *testing.T) {
	h := NewMultimodalHandlerFromConfig(MultimodalHandlerConfig{
		OpenAIAPIKey:         "openai-key",
		GoogleAPIKey:         "google-key",
		DefaultImageProvider: "gemini",
		DefaultVideoProvider: "veo",
	}, zap.NewNop())

	imageProvider, err := h.resolveImageProvider("")
	require.NoError(t, err)
	assert.Equal(t, "gemini", imageProvider)

	videoProvider, err := h.resolveVideoProvider("")
	require.NoError(t, err)
	assert.Equal(t, "veo", videoProvider)
}

func TestMultimodalHandler_UploadReferenceStoreFailure(t *testing.T) {
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{},
		map[string]video.Provider{},
		"",
		"",
		nil,
		0,
		0,
		&failingReferenceStore{},
		zap.NewNop(),
	)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", "ref.png"))
	hdr.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(hdr)
	require.NoError(t, err)
	onePixelPNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+XfMsAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(onePixelPNG)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/references", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.HandleUploadReference(w, r)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestMultimodalHandler_ImageRejectsPrivateReferenceURL(t *testing.T) {
	img := &mockImageProvider{}
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{"mock": img},
		map[string]video.Provider{},
		"mock",
		"",
		nil,
		0,
		0,
		nil,
		zap.NewNop(),
	)

	body := map[string]any{
		"prompt":              "a cat",
		"provider":            "mock",
		"reference_image_url": "http://127.0.0.1/internal.png",
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/image", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	h.HandleImage(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.False(t, img.editCalled)
	assert.False(t, img.generateCalled)
}

func TestMultimodalHandler_VideoRejectsPrivateReferenceURL(t *testing.T) {
	vdo := &mockVideoProvider{}
	h := NewMultimodalHandlerWithProviders(
		nil,
		map[string]image.Provider{},
		map[string]video.Provider{"runway": vdo},
		"",
		"runway",
		nil,
		0,
		0,
		nil,
		zap.NewNop(),
	)

	body := map[string]any{
		"prompt":              "a moving camera scene",
		"provider":            "runway",
		"reference_image_url": "http://127.0.0.1/internal.png",
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/video", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	h.HandleVideo(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, vdo.lastReq)
}

func TestValidatePublicReferenceImageURL(t *testing.T) {
	_, err := validatePublicReferenceImageURL(context.Background(), "http://127.0.0.1/internal.png")
	require.Error(t, err)

	url, err := validatePublicReferenceImageURL(context.Background(), "https://8.8.8.8/path.png")
	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/path.png", url)
}

func TestIsDisallowedReferenceIP(t *testing.T) {
	assert.True(t, isDisallowedReferenceIP(net.ParseIP("127.0.0.1")))
	assert.True(t, isDisallowedReferenceIP(net.ParseIP("10.0.0.1")))
	assert.True(t, isDisallowedReferenceIP(net.ParseIP("100.64.0.10")))
	assert.True(t, isDisallowedReferenceIP(net.ParseIP("169.254.1.1")))
	assert.True(t, isDisallowedReferenceIP(net.ParseIP("::1")))
	assert.False(t, isDisallowedReferenceIP(net.ParseIP("8.8.8.8")))
}

func TestMemoryReferenceStore_Cleanup(t *testing.T) {
	store := NewMemoryReferenceStore()
	oldRef := &referenceAsset{
		ID:        "old",
		CreatedAt: time.Now().Add(-3 * time.Hour),
	}
	newRef := &referenceAsset{
		ID:        "new",
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.Save(oldRef))
	require.NoError(t, store.Save(newRef))

	store.Cleanup(time.Now().Add(-2 * time.Hour))

	_, okOld := store.Get("old")
	_, okNew := store.Get("new")
	assert.False(t, okOld)
	assert.True(t, okNew)
}

func TestRedisReferenceStore_SaveGetDelete(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisReferenceStore(client, "agentflow:test:mm", 2*time.Hour, zap.NewNop())
	asset := &referenceAsset{
		ID:        "ref_1",
		FileName:  "ref.png",
		MimeType:  "image/png",
		Size:      3,
		CreatedAt: time.Now().UTC(),
		Data:      []byte{1, 2, 3},
	}
	require.NoError(t, store.Save(asset))

	got, ok := store.Get("ref_1")
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, asset.ID, got.ID)
	assert.Equal(t, asset.FileName, got.FileName)
	assert.Equal(t, asset.MimeType, got.MimeType)
	assert.Equal(t, asset.Size, got.Size)
	assert.WithinDuration(t, asset.CreatedAt, got.CreatedAt, time.Second)
	assert.Equal(t, asset.Data, got.Data)

	store.Delete("ref_1")
	_, ok = store.Get("ref_1")
	assert.False(t, ok)
}

func TestRedisReferenceStore_TTLExpiry(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()

	store := NewRedisReferenceStore(client, "agentflow:test:mm", 2*time.Second, zap.NewNop())
	require.NoError(t, store.Save(&referenceAsset{
		ID:        "ref_2",
		FileName:  "ref.png",
		MimeType:  "image/png",
		Size:      3,
		CreatedAt: time.Now().UTC(),
		Data:      []byte{1, 2, 3},
	}))

	assert.True(t, mr.Exists("agentflow:test:mm:ref_2"))
	mr.FastForward(3 * time.Second)

	_, ok := store.Get("ref_2")
	assert.False(t, ok)
}

func uploadTestReference(t *testing.T, h *MultimodalHandler) string {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", "ref.png"))
	hdr.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(hdr)
	require.NoError(t, err)
	onePixelPNG, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+XfMsAAAAASUVORK5CYII=")
	require.NoError(t, err)
	_, err = part.Write(onePixelPNG)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/references", &buf)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.HandleUploadReference(w, r)
	require.Equalf(t, http.StatusOK, w.Code, "upload response: %s", w.Body.String())

	var resp Response
	err = json.NewDecoder(w.Body).Decode(&resp)
	require.NoError(t, err)
	require.True(t, resp.Success)

	raw, err := json.Marshal(resp.Data)
	require.NoError(t, err)
	var payload map[string]any
	err = json.Unmarshal(raw, &payload)
	require.NoError(t, err)

	refID, ok := payload["reference_id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, refID)
	return refID
}

package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/internal/usecase"
	"github.com/BaSui01/agentflow/llm/capabilities"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llm "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/pkg/storage"
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

type scriptedMultimodalChatProvider struct{}

func (m *scriptedMultimodalChatProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	_ = ctx
	content := "single response"
	model := req.Model
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}
	if len(req.Messages) > 0 {
		first := req.Messages[0].Content
		switch {
		case strings.Contains(first, "orchestration planner"):
			content = "1. gather facts\n2. produce answer"
		case strings.Contains(first, "executor agent"):
			content = "final answer"
		}
	}
	return &llm.ChatResponse{
		ID:    "scripted",
		Model: model,
		Choices: []llm.ChatChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    types.RoleAssistant,
					Content: content,
				},
			},
		},
		CreatedAt: time.Now(),
	}, nil
}

func (m *scriptedMultimodalChatProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch := make(chan llm.StreamChunk)
	close(ch)
	return ch, nil
}

func (m *scriptedMultimodalChatProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return &llm.HealthStatus{Healthy: true}, nil
}

func (m *scriptedMultimodalChatProvider) Name() string { return "scripted-multimodal-chat" }

func (m *scriptedMultimodalChatProvider) SupportsNativeFunctionCalling() bool { return true }

func (m *scriptedMultimodalChatProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	return nil, nil
}

func (m *scriptedMultimodalChatProvider) Endpoints() llm.ProviderEndpoints {
	return llm.ProviderEndpoints{}
}

type failingReferenceStore struct{}

func (s *failingReferenceStore) Save(asset *storage.ReferenceAsset) error {
	_ = asset
	return errors.New("reference store unavailable")
}

func (s *failingReferenceStore) Get(id string) (*storage.ReferenceAsset, bool) {
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
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         logger,
		imageProviders: map[string]image.Provider{"mock": img},
		videoProviders: map[string]video.Provider{"runway": vdo},
		defaultImage:   "mock",
		defaultVideo:   "runway",
	})

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

func TestConvertAPIMessages_PreservesMultimodalFields(t *testing.T) {
	reasoning := "reasoning"
	refusal := "refusal"
	messages := []api.Message{
		{
			Role:             "user",
			Content:          "describe this",
			ReasoningContent: &reasoning,
			ThinkingBlocks:   []types.ThinkingBlock{{Thinking: "step 1"}},
			Refusal:          &refusal,
			IsToolError:      true,
			Images: []api.ImageContent{
				{Type: "url", URL: "https://example.com/image.png"},
			},
			Videos: []types.VideoContent{
				{URL: "https://example.com/video.mp4"},
			},
			Annotations: []types.Annotation{
				{Type: "url_citation", URL: "https://example.com"},
			},
			Metadata:  map[string]any{"k": "v"},
			Timestamp: time.Now(),
		},
	}

	converted := convertAPIMessages(messages)
	require.Len(t, converted, 1)
	assert.Equal(t, "describe this", converted[0].Content)
	require.NotNil(t, converted[0].ReasoningContent)
	assert.Equal(t, reasoning, *converted[0].ReasoningContent)
	require.NotNil(t, converted[0].Refusal)
	assert.Equal(t, refusal, *converted[0].Refusal)
	assert.True(t, converted[0].IsToolError)
	require.Len(t, converted[0].Images, 1)
	assert.Equal(t, "https://example.com/image.png", converted[0].Images[0].URL)
	require.Len(t, converted[0].Videos, 1)
	assert.Equal(t, "https://example.com/video.mp4", converted[0].Videos[0].URL)
	require.Len(t, converted[0].Annotations, 1)
	assert.Equal(t, "https://example.com", converted[0].Annotations[0].URL)
}

func TestMultimodalHandler_ImageStream(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         logger,
		imageProviders: map[string]image.Provider{"mock": img},
		defaultImage:   "mock",
	})

	body := map[string]any{
		"prompt": "a cat",
		"stream": true,
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/image", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandleImage(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	bodyStr := w.Body.String()
	assert.Contains(t, bodyStr, "event: image_generation.started")
	assert.Contains(t, bodyStr, "event: image_generation.completed")
	assert.Contains(t, bodyStr, "event: image_generation.done")
	assert.Contains(t, bodyStr, "data: [DONE]")
	assert.Contains(t, bodyStr, "https://example.com/image.png")
	assert.Contains(t, bodyStr, "\"type\":\"image_generation.completed\"")
	assert.True(t, img.generateCalled)
}

func TestMultimodalHandler_VideoReferenceFlow(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	vdo := &mockVideoProvider{}
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         logger,
		imageProviders: map[string]image.Provider{"mock": img},
		videoProviders: map[string]video.Provider{"runway": vdo},
		defaultImage:   "mock",
		defaultVideo:   "runway",
	})

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
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:       logger,
		chatProvider: &mockLLMProvider{},
	})

	raw := []byte(`{"prompt":"test","unknown_field":"x"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/plan", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandlePlan(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMultimodalHandler_PlanSuccess(t *testing.T) {
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:       zap.NewNop(),
		chatProvider: &mockLLMProvider{},
	})

	raw := []byte(`{"prompt":"make a launch teaser","shot_count":2}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/plan", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandlePlan(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"plan"`)
	assert.Contains(t, w.Body.String(), `"goal":"x"`)
}

func TestMultimodalHandler_ChatAgentMode(t *testing.T) {
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:       zap.NewNop(),
		chatProvider: &scriptedMultimodalChatProvider{},
	})

	raw := []byte(`{"message":"draft a multimodal launch response","agent_mode":true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/chat", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")

	h.HandleChat(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"mode":"agent"`)
	assert.Contains(t, w.Body.String(), `"planner_output":"1. gather facts\n2. produce answer"`)
	assert.Contains(t, w.Body.String(), `"final_text":"final answer"`)
}

func TestMultimodalHandler_Capabilities(t *testing.T) {
	logger := zap.NewNop()
	img := &mockImageProvider{}
	vdo := &mockVideoProvider{}
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         logger,
		imageProviders: map[string]image.Provider{"mock": img},
		videoProviders: map[string]video.Provider{"runway": vdo},
		defaultImage:   "mock",
		defaultVideo:   "runway",
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/multimodal/capabilities", nil)
	h.HandleCapabilities(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestMultimodalHandler_DefaultProviderRespected(t *testing.T) {
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         zap.NewNop(),
		imageProviders: map[string]image.Provider{"gemini": &mockImageProvider{}},
		videoProviders: map[string]video.Provider{"veo": &mockVideoProvider{}},
		defaultImage:   "gemini",
		defaultVideo:   "veo",
	})

	imageProvider, err := h.resolveImageProvider("")
	require.NoError(t, err)
	assert.Equal(t, "gemini", imageProvider)

	videoProvider, err := h.resolveVideoProvider("")
	require.NoError(t, err)
	assert.Equal(t, "veo", videoProvider)
}

func TestMultimodalHandler_UploadReferenceStoreFailure(t *testing.T) {
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         zap.NewNop(),
		referenceStore: &failingReferenceStore{},
	})

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
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         zap.NewNop(),
		imageProviders: map[string]image.Provider{"mock": img},
		defaultImage:   "mock",
	})

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
	h := newMultimodalHandlerForTest(multimodalHandlerTestConfig{
		logger:         zap.NewNop(),
		videoProviders: map[string]video.Provider{"runway": vdo},
		defaultVideo:   "runway",
	})

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
	_, err := multimodal.ValidatePublicReferenceImageURL(context.Background(), "http://127.0.0.1/internal.png")
	require.Error(t, err)

	url, err := multimodal.ValidatePublicReferenceImageURL(context.Background(), "https://8.8.8.8/path.png")
	require.NoError(t, err)
	assert.Equal(t, "https://8.8.8.8/path.png", url)
}

func TestMemoryReferenceStore_Cleanup(t *testing.T) {
	store := storage.NewMemoryReferenceStore()
	oldRef := &storage.ReferenceAsset{
		ID:        "old",
		CreatedAt: time.Now().Add(-3 * time.Hour),
	}
	newRef := &storage.ReferenceAsset{
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

	store := storage.NewRedisReferenceStore(client, "agentflow:test:mm", 2*time.Hour, zap.NewNop())
	asset := &storage.ReferenceAsset{
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

	store := storage.NewRedisReferenceStore(client, "agentflow:test:mm", 2*time.Second, zap.NewNop())
	require.NoError(t, store.Save(&storage.ReferenceAsset{
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

type multimodalHandlerTestConfig struct {
	logger           *zap.Logger
	chatProvider     llm.Provider
	imageProviders   map[string]image.Provider
	videoProviders   map[string]video.Provider
	defaultImage     string
	defaultVideo     string
	referenceMaxSize int64
	referenceTTL     time.Duration
	referenceStore   storage.ReferenceStore
	defaultChatModel string
}

func newMultimodalHandlerForTest(cfg multimodalHandlerTestConfig) *MultimodalHandler {
	logger := cfg.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	if cfg.imageProviders == nil {
		cfg.imageProviders = map[string]image.Provider{}
	}
	if cfg.videoProviders == nil {
		cfg.videoProviders = map[string]video.Provider{}
	}
	if cfg.referenceMaxSize <= 0 {
		cfg.referenceMaxSize = defaultReferenceBytes
	}
	if cfg.referenceTTL <= 0 {
		cfg.referenceTTL = defaultReferenceTTL
	}
	if cfg.referenceStore == nil {
		cfg.referenceStore = storage.NewMemoryReferenceStore()
	}
	if strings.TrimSpace(cfg.defaultChatModel) == "" {
		cfg.defaultChatModel = "gpt-4o-mini"
	}

	router := multimodal.NewRouter()
	imageNames := make([]string, 0, len(cfg.imageProviders))
	for name := range cfg.imageProviders {
		imageNames = append(imageNames, name)
	}
	sort.Strings(imageNames)
	videoNames := make([]string, 0, len(cfg.videoProviders))
	for name := range cfg.videoProviders {
		videoNames = append(videoNames, name)
	}
	sort.Strings(videoNames)

	defaultImage := strings.TrimSpace(cfg.defaultImage)
	if defaultImage == "" && len(imageNames) > 0 {
		defaultImage = imageNames[0]
	}
	defaultVideo := strings.TrimSpace(cfg.defaultVideo)
	if defaultVideo == "" && len(videoNames) > 0 {
		defaultVideo = videoNames[0]
	}
	for _, name := range imageNames {
		router.RegisterImage(name, cfg.imageProviders[name], name == defaultImage)
	}
	for _, name := range videoNames {
		router.RegisterVideo(name, cfg.videoProviders[name], name == defaultVideo)
	}

	gateway := llmgateway.New(llmgateway.Config{
		ChatProvider: cfg.chatProvider,
		Capabilities: capabilities.NewEntry(router),
		Logger:       logger,
	})

	resolveImage := func(provider string) (string, error) {
		name := strings.TrimSpace(provider)
		if name == "" {
			name = defaultImage
		}
		if name == "" {
			return "", fmt.Errorf("no default image provider available")
		}
		if _, err := router.Image(name); err != nil {
			return "", fmt.Errorf("image provider %q not found", name)
		}
		return name, nil
	}
	resolveVideo := func(provider string) (string, error) {
		name := strings.TrimSpace(provider)
		if name == "" {
			name = defaultVideo
		}
		if name == "" {
			return "", fmt.Errorf("no default video provider available")
		}
		if _, err := router.Video(name); err != nil {
			return "", fmt.Errorf("video provider %q not found", name)
		}
		return name, nil
	}

	service := usecase.NewDefaultMultimodalService(
		usecase.MultimodalRuntime{
			Gateway:              gateway,
			Pipeline:             &multimodal.DefaultPromptPipeline{},
			ResolveImageProvider: resolveImage,
			ResolveVideoProvider: resolveVideo,
			ReferenceStore:       cfg.referenceStore,
			ReferenceTTL:         cfg.referenceTTL,
			ReferenceMaxSize:     cfg.referenceMaxSize,
			ChatEnabled:          cfg.chatProvider != nil,
			DefaultChatModel:     cfg.defaultChatModel,
		},
	)

	handler, err := NewMultimodalHandler(service, logger)
	if err != nil {
		panic(err)
	}
	handler.ApplyRuntimeDeps(MultimodalHandlerRuntimeDeps{
		DefaultImageProvider: defaultImage,
		DefaultVideoProvider: defaultVideo,
		ImageProviders:       imageNames,
		VideoProviders:       videoNames,
		ReferenceMaxSize:     cfg.referenceMaxSize,
		ReferenceTTL:         cfg.referenceTTL,
		ReferenceStore:       cfg.referenceStore,
		ChatEnabled:          cfg.chatProvider != nil,
		ResolveImageProvider: resolveImage,
		ResolveVideoProvider: resolveVideo,
		ImageStreamProvider: func(provider string) (image.StreamingProvider, bool) {
			p, err := router.Image(provider)
			if err != nil {
				return nil, false
			}
			sp, ok := p.(image.StreamingProvider)
			return sp, ok
		},
	})
	return handler
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

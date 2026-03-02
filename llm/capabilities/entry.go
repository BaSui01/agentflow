package capabilities

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/avatar"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/moderation"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/music"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/llm/capabilities/threed"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	"github.com/BaSui01/agentflow/types"
)

// Entry 是能力层统一入口，封装多模态路由并对上游暴露稳定调用面。
type Entry struct {
	router        *multimodal.Router
	toolExecutor  llmtools.ToolExecutor
	avatarMu      sync.RWMutex
	avatar        map[string]avatar.Provider
	defaultAvatar string
}

// NewEntry 创建统一能力入口。若 router 为空则创建默认路由器。
func NewEntry(router *multimodal.Router) *Entry {
	if router == nil {
		router = multimodal.NewRouter()
	}
	return &Entry{
		router: router,
		avatar: make(map[string]avatar.Provider),
	}
}

// Router 返回底层多模态路由器，供注册 provider 时使用。
func (e *Entry) Router() *multimodal.Router {
	if e == nil {
		return nil
	}
	return e.router
}

// SetToolExecutor 设置 tools 能力执行器。
func (e *Entry) SetToolExecutor(executor llmtools.ToolExecutor) {
	if e == nil {
		return
	}
	e.toolExecutor = executor
}

// ToolExecutor 返回当前配置的 tools 执行器。
func (e *Entry) ToolExecutor() llmtools.ToolExecutor {
	if e == nil {
		return nil
	}
	return e.toolExecutor
}

// RegisterAvatar 注册 avatar provider。
func (e *Entry) RegisterAvatar(name string, provider avatar.Provider, isDefault bool) {
	if e == nil || provider == nil || name == "" {
		return
	}
	e.avatarMu.Lock()
	defer e.avatarMu.Unlock()
	e.avatar[name] = provider
	if isDefault || e.defaultAvatar == "" {
		e.defaultAvatar = name
	}
}

// Avatar 获取指定 avatar provider（为空时使用默认 provider）。
func (e *Entry) Avatar(name string) (avatar.Provider, error) {
	if e == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	e.avatarMu.RLock()
	defer e.avatarMu.RUnlock()
	if name == "" {
		name = e.defaultAvatar
	}
	if p, ok := e.avatar[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("avatar provider %q not found", name)
}

// Embedding 获取指定嵌入 provider（为空时使用默认 provider）。
func (e *Entry) Embedding(name string) (embedding.Provider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Embedding(name)
}

// Rerank 获取指定重排序 provider（为空时使用默认 provider）。
func (e *Entry) Rerank(name string) (rerank.Provider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Rerank(name)
}

// TTS 获取指定 TTS provider（为空时使用默认 provider）。
func (e *Entry) TTS(name string) (speech.TTSProvider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.TTS(name)
}

// STT 获取指定 STT provider（为空时使用默认 provider）。
func (e *Entry) STT(name string) (speech.STTProvider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.STT(name)
}

// Image 获取指定图像 provider（为空时使用默认 provider）。
func (e *Entry) Image(name string) (image.Provider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Image(name)
}

// Video 获取指定视频 provider（为空时使用默认 provider）。
func (e *Entry) Video(name string) (video.Provider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Video(name)
}

// Music 获取指定音乐 provider（为空时使用默认 provider）。
func (e *Entry) Music(name string) (music.MusicProvider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Music(name)
}

// ThreeD 获取指定 3D provider（为空时使用默认 provider）。
func (e *Entry) ThreeD(name string) (threed.ThreeDProvider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.ThreeD(name)
}

// Moderation 获取指定 moderation provider（为空时使用默认 provider）。
func (e *Entry) Moderation(name string) (moderation.ModerationProvider, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Moderation(name)
}

// Embed 调用嵌入能力。
func (e *Entry) Embed(ctx context.Context, req *embedding.EmbeddingRequest, providerName string) (*embedding.EmbeddingResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Embed(ctx, req, providerName)
}

// RerankDocs 调用重排序能力。
func (e *Entry) RerankDocs(ctx context.Context, req *rerank.RerankRequest, providerName string) (*rerank.RerankResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.RerankDocs(ctx, req, providerName)
}

// Synthesize 调用 TTS 能力。
func (e *Entry) Synthesize(ctx context.Context, req *speech.TTSRequest, providerName string) (*speech.TTSResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Synthesize(ctx, req, providerName)
}

// Transcribe 调用 STT 能力。
func (e *Entry) Transcribe(ctx context.Context, req *speech.STTRequest, providerName string) (*speech.STTResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Transcribe(ctx, req, providerName)
}

// GenerateImage 调用图像生成能力。
func (e *Entry) GenerateImage(ctx context.Context, req *image.GenerateRequest, providerName string) (*image.GenerateResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.GenerateImage(ctx, req, providerName)
}

// GenerateVideo 调用视频生成能力。
func (e *Entry) GenerateVideo(ctx context.Context, req *video.GenerateRequest, providerName string) (*video.GenerateResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.GenerateVideo(ctx, req, providerName)
}

// GenerateMusic 调用音乐生成能力。
func (e *Entry) GenerateMusic(ctx context.Context, req *music.GenerateRequest, providerName string) (*music.GenerateResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.GenerateMusic(ctx, req, providerName)
}

// Generate3D 调用 3D 生成能力。
func (e *Entry) Generate3D(ctx context.Context, req *threed.GenerateRequest, providerName string) (*threed.GenerateResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Generate3D(ctx, req, providerName)
}

// Moderate 调用内容审核能力。
func (e *Entry) Moderate(ctx context.Context, req *moderation.ModerationRequest, providerName string) (*moderation.ModerationResponse, error) {
	if e == nil || e.router == nil {
		return nil, fmt.Errorf("capabilities entry is not configured")
	}
	return e.router.Moderate(ctx, req, providerName)
}

// GenerateAvatar 调用 avatar 能力。
func (e *Entry) GenerateAvatar(ctx context.Context, req *avatar.GenerateRequest, providerName string) (*avatar.GenerateResponse, error) {
	p, err := e.Avatar(providerName)
	if err != nil {
		return nil, err
	}
	return p.Generate(ctx, req)
}

// ExecuteTools 调用 tools 执行能力。
func (e *Entry) ExecuteTools(ctx context.Context, calls []types.ToolCall) ([]types.ToolResult, error) {
	if e == nil || e.toolExecutor == nil {
		return nil, fmt.Errorf("tools executor is not configured")
	}
	return e.toolExecutor.Execute(ctx, calls), nil
}

// ExecuteTool 调用单个 tool。
func (e *Entry) ExecuteTool(ctx context.Context, call types.ToolCall) (types.ToolResult, error) {
	if e == nil || e.toolExecutor == nil {
		return types.ToolResult{}, fmt.Errorf("tools executor is not configured")
	}
	return e.toolExecutor.ExecuteOne(ctx, call), nil
}

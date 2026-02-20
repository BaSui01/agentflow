package multimodal

import (
	"context"
	"fmt"
	"sync"

	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/moderation"
	"github.com/BaSui01/agentflow/llm/music"
	"github.com/BaSui01/agentflow/llm/rerank"
	"github.com/BaSui01/agentflow/llm/speech"
	"github.com/BaSui01/agentflow/llm/threed"
	"github.com/BaSui01/agentflow/llm/video"
)

// 能力代表一种AI能力.
type Capability string

const (
	CapabilityEmbedding  Capability = "embedding"
	CapabilityRerank     Capability = "rerank"
	CapabilityTTS        Capability = "tts"
	CapabilitySTT        Capability = "stt"
	CapabilityImage      Capability = "image"
	CapabilityVideo      Capability = "video"
	CapabilityMusic      Capability = "music"
	CapabilityThreeD     Capability = "3d"
	CapabilityModeration Capability = "moderation"
)

// Router 向所有多模态提供者提供统一访问。
type Router struct {
	mu sync.RWMutex

	embeddingProviders  map[string]embedding.Provider
	rerankProviders     map[string]rerank.Provider
	ttsProviders        map[string]speech.TTSProvider
	sttProviders        map[string]speech.STTProvider
	imageProviders      map[string]image.Provider
	videoProviders      map[string]video.Provider
	musicProviders      map[string]music.MusicProvider
	threeDProviders     map[string]threed.ThreeDProvider
	moderationProviders map[string]moderation.ModerationProvider

	defaultEmbedding  string
	defaultRerank     string
	defaultTTS        string
	defaultSTT        string
	defaultImage      string
	defaultVideo      string
	defaultMusic      string
	defaultThreeD     string
	defaultModeration string
}

// 新路特创建了新的多模式路由器.
func NewRouter() *Router {
	return &Router{
		embeddingProviders:  make(map[string]embedding.Provider),
		rerankProviders:     make(map[string]rerank.Provider),
		ttsProviders:        make(map[string]speech.TTSProvider),
		sttProviders:        make(map[string]speech.STTProvider),
		imageProviders:      make(map[string]image.Provider),
		videoProviders:      make(map[string]video.Provider),
		musicProviders:      make(map[string]music.MusicProvider),
		threeDProviders:     make(map[string]threed.ThreeDProvider),
		moderationProviders: make(map[string]moderation.ModerationProvider),
	}
}

// ============================================================
// 登记方法
// ============================================================

// RegisterEmbedding 注册一个嵌入提供者.
func (r *Router) RegisterEmbedding(name string, provider embedding.Provider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.embeddingProviders[name] = provider
	if isDefault || r.defaultEmbedding == "" {
		r.defaultEmbedding = name
	}
}

// Register Rerank 注册一个重新排序的提供者 。
func (r *Router) RegisterRerank(name string, provider rerank.Provider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rerankProviders[name] = provider
	if isDefault || r.defaultRerank == "" {
		r.defaultRerank = name
	}
}

// RegisterTTS 注册一个 TTS 提供者.
func (r *Router) RegisterTTS(name string, provider speech.TTSProvider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ttsProviders[name] = provider
	if isDefault || r.defaultTTS == "" {
		r.defaultTTS = name
	}
}

// 注册STT 注册 STT 提供者 。
func (r *Router) RegisterSTT(name string, provider speech.STTProvider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sttProviders[name] = provider
	if isDefault || r.defaultSTT == "" {
		r.defaultSTT = name
	}
}

// 注册图像注册图像提供者 。
func (r *Router) RegisterImage(name string, provider image.Provider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.imageProviders[name] = provider
	if isDefault || r.defaultImage == "" {
		r.defaultImage = name
	}
}

// RegisterVideo 注册一个视频提供者。
func (r *Router) RegisterVideo(name string, provider video.Provider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.videoProviders[name] = provider
	if isDefault || r.defaultVideo == "" {
		r.defaultVideo = name
	}
}

// 注册Music 注册音乐提供者 。
func (r *Router) RegisterMusic(name string, provider music.MusicProvider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.musicProviders[name] = provider
	if isDefault || r.defaultMusic == "" {
		r.defaultMusic = name
	}
}

// RegisterThreeD 注册一个 3D 提供者.
func (r *Router) RegisterThreeD(name string, provider threed.ThreeDProvider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.threeDProviders[name] = provider
	if isDefault || r.defaultThreeD == "" {
		r.defaultThreeD = name
	}
}

// 登记册修改登记了一个温和提供方。
func (r *Router) RegisterModeration(name string, provider moderation.ModerationProvider, isDefault bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.moderationProviders[name] = provider
	if isDefault || r.defaultModeration == "" {
		r.defaultModeration = name
	}
}

// ============================================================
// 提供者获取方法
// ============================================================

// 嵌入通过名称或默认返回嵌入提供者 。
func (r *Router) Embedding(name string) (embedding.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultEmbedding
	}
	if p, ok := r.embeddingProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("embedding provider %q not found", name)
}

// Rerank 返回按名称或默认排序的提供者 。
func (r *Router) Rerank(name string) (rerank.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultRerank
	}
	if p, ok := r.rerankProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("rerank provider %q not found", name)
}

// TTS通过名称或默认返回一个 TTS 提供者.
func (r *Router) TTS(name string) (speech.TTSProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultTTS
	}
	if p, ok := r.ttsProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("TTS provider %q not found", name)
}

// STT 按名称或默认返回一个 STT 提供者 。
func (r *Router) STT(name string) (speech.STTProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultSTT
	}
	if p, ok := r.sttProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("STT provider %q not found", name)
}

// 图像通过名称或默认返回图像提供者 。
func (r *Router) Image(name string) (image.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultImage
	}
	if p, ok := r.imageProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("image provider %q not found", name)
}

// 视频通过名称或默认返回视频提供者.
func (r *Router) Video(name string) (video.Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultVideo
	}
	if p, ok := r.videoProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("video provider %q not found", name)
}

// 音乐通过名称或默认返回音乐提供者.
func (r *Router) Music(name string) (music.MusicProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultMusic
	}
	if p, ok := r.musicProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("music provider %q not found", name)
}

// 三维通过名称或默认返回一个三维提供者.
func (r *Router) ThreeD(name string) (threed.ThreeDProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultThreeD
	}
	if p, ok := r.threeDProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("3D provider %q not found", name)
}

// 中度通过名称或默认返回温和提供方 。
func (r *Router) Moderation(name string) (moderation.ModerationProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultModeration
	}
	if p, ok := r.moderationProviders[name]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("moderation provider %q not found", name)
}

// ============================================================
// 直接行动的方便方法
// ============================================================

// 嵌入使用默认或指定的提供者生成嵌入.
func (r *Router) Embed(ctx context.Context, req *embedding.EmbeddingRequest, providerName string) (*embedding.EmbeddingResponse, error) {
	p, err := r.Embedding(providerName)
	if err != nil {
		return nil, err
	}
	return p.Embed(ctx, req)
}

// 重新排序 Docs 使用默认或指定的提供者重新排序文档 。
func (r *Router) RerankDocs(ctx context.Context, req *rerank.RerankRequest, providerName string) (*rerank.RerankResponse, error) {
	p, err := r.Rerank(providerName)
	if err != nil {
		return nil, err
	}
	return p.Rerank(ctx, req)
}

// 合成大小使用默认或指定的提供者生成语音.
func (r *Router) Synthesize(ctx context.Context, req *speech.TTSRequest, providerName string) (*speech.TTSResponse, error) {
	p, err := r.TTS(providerName)
	if err != nil {
		return nil, err
	}
	return p.Synthesize(ctx, req)
}

// 使用默认或指定的提供者将语音转换为文本。
func (r *Router) Transcribe(ctx context.Context, req *speech.STTRequest, providerName string) (*speech.STTResponse, error) {
	p, err := r.STT(providerName)
	if err != nil {
		return nil, err
	}
	return p.Transcribe(ctx, req)
}

// 生成图像使用默认或指定的提供者生成图像.
func (r *Router) GenerateImage(ctx context.Context, req *image.GenerateRequest, providerName string) (*image.GenerateResponse, error) {
	p, err := r.Image(providerName)
	if err != nil {
		return nil, err
	}
	return p.Generate(ctx, req)
}

// 生成视频使用默认或指定的提供者生成.
func (r *Router) GenerateVideo(ctx context.Context, req *video.GenerateRequest, providerName string) (*video.GenerateResponse, error) {
	p, err := r.Video(providerName)
	if err != nil {
		return nil, err
	}
	return p.Generate(ctx, req)
}

// 生成音乐使用默认或指定的提供者生成音乐.
func (r *Router) GenerateMusic(ctx context.Context, req *music.GenerateRequest, providerName string) (*music.GenerateResponse, error) {
	p, err := r.Music(providerName)
	if err != nil {
		return nil, err
	}
	return p.Generate(ctx, req)
}

// 生成3D使用默认或指定的提供者生成3D模型.
func (r *Router) Generate3D(ctx context.Context, req *threed.GenerateRequest, providerName string) (*threed.GenerateResponse, error) {
	p, err := r.ThreeD(providerName)
	if err != nil {
		return nil, err
	}
	return p.Generate(ctx, req)
}

// 适度检查政策违规内容.
func (r *Router) Moderate(ctx context.Context, req *moderation.ModerationRequest, providerName string) (*moderation.ModerationResponse, error) {
	p, err := r.Moderation(providerName)
	if err != nil {
		return nil, err
	}
	return p.Moderate(ctx, req)
}

// ============================================================
// 使用方法
// ============================================================

// ListProviders按能力返回所有注册的提供者名称.
func (r *Router) ListProviders() map[Capability][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[Capability][]string)

	for name := range r.embeddingProviders {
		result[CapabilityEmbedding] = append(result[CapabilityEmbedding], name)
	}
	for name := range r.rerankProviders {
		result[CapabilityRerank] = append(result[CapabilityRerank], name)
	}
	for name := range r.ttsProviders {
		result[CapabilityTTS] = append(result[CapabilityTTS], name)
	}
	for name := range r.sttProviders {
		result[CapabilitySTT] = append(result[CapabilitySTT], name)
	}
	for name := range r.imageProviders {
		result[CapabilityImage] = append(result[CapabilityImage], name)
	}
	for name := range r.videoProviders {
		result[CapabilityVideo] = append(result[CapabilityVideo], name)
	}
	for name := range r.musicProviders {
		result[CapabilityMusic] = append(result[CapabilityMusic], name)
	}
	for name := range r.threeDProviders {
		result[CapabilityThreeD] = append(result[CapabilityThreeD], name)
	}
	for name := range r.moderationProviders {
		result[CapabilityModeration] = append(result[CapabilityModeration], name)
	}

	return result
}

// 如果具备能力,则进行能力检查。
func (r *Router) HasCapability(cap Capability) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch cap {
	case CapabilityEmbedding:
		return len(r.embeddingProviders) > 0
	case CapabilityRerank:
		return len(r.rerankProviders) > 0
	case CapabilityTTS:
		return len(r.ttsProviders) > 0
	case CapabilitySTT:
		return len(r.sttProviders) > 0
	case CapabilityImage:
		return len(r.imageProviders) > 0
	case CapabilityVideo:
		return len(r.videoProviders) > 0
	case CapabilityMusic:
		return len(r.musicProviders) > 0
	case CapabilityThreeD:
		return len(r.threeDProviders) > 0
	case CapabilityModeration:
		return len(r.moderationProviders) > 0
	}
	return false
}

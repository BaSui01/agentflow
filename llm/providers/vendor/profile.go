package vendor

import (
	"strings"

	speech "github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llm "github.com/BaSui01/agentflow/llm/core"
)

// Profile 按供应商聚合能力，避免按功能分散配置造成重复。
type Profile struct {
	Name string

	Chat      llm.Provider
	Embedding embedding.Provider
	Image     image.Provider
	Video     video.Provider
	TTS       speech.TTSProvider
	STT       speech.STTProvider

	// LanguageModels 按语言适配默认模型，如 {"zh":"gpt-5.4", "en":"gpt-5.4"}。
	LanguageModels map[string]string
}

// ModelForLanguage 返回语言适配模型；未命中时返回 fallback。
func (p *Profile) ModelForLanguage(language, fallback string) string {
	if p == nil || len(p.LanguageModels) == 0 {
		return fallback
	}
	lang := strings.ToLower(strings.TrimSpace(language))
	if lang == "" {
		return fallback
	}
	if m, ok := p.LanguageModels[lang]; ok && strings.TrimSpace(m) != "" {
		return m
	}
	primary := lang
	if idx := strings.Index(primary, "-"); idx > 0 {
		primary = primary[:idx]
	}
	if m, ok := p.LanguageModels[primary]; ok && strings.TrimSpace(m) != "" {
		return m
	}
	if m, ok := p.LanguageModels["default"]; ok && strings.TrimSpace(m) != "" {
		return m
	}
	return fallback
}

package multimodal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
)

// Processor 处理不同提供者的多模态内容转换.
type Processor struct {
	visionConfig VisionConfig
	audioConfig  AudioConfig
}

// NewProcessor 创建新的多模态处理器.
func NewProcessor(visionCfg VisionConfig, audioCfg AudioConfig) *Processor {
	return &Processor{
		visionConfig: visionCfg,
		audioConfig:  audioCfg,
	}
}

// DefaultProcessor 创建默认配置的处理器.
func DefaultProcessor() *Processor {
	return NewProcessor(DefaultVisionConfig(), DefaultAudioConfig())
}

// ConvertToProviderFormat 将多模态消息转换为提供者专用格式.
func (p *Processor) ConvertToProviderFormat(provider string, messages []MultimodalMessage) ([]llm.Message, error) {
	switch provider {
	case "openai":
		return p.convertToOpenAI(messages)
	case "anthropic":
		return p.convertToAnthropic(messages)
	case "gemini":
		return p.convertToGemini(messages)
	default:
		return p.convertToGeneric(messages)
	}
}

// convertToOpenAI 转换为 OpenAI 的多模态格式.
func (p *Processor) convertToOpenAI(messages []MultimodalMessage) ([]llm.Message, error) {
	var result []llm.Message

	for _, msg := range messages {
		var contentParts []map[string]interface{}

		for _, content := range msg.Contents {
			switch content.Type {
			case ContentTypeText:
				contentParts = append(contentParts, map[string]interface{}{
					"type": "text",
					"text": content.Text,
				})

			case ContentTypeImage:
				imageContent := map[string]interface{}{
					"type": "image_url",
				}
				if content.ImageURL != "" {
					imageContent["image_url"] = map[string]interface{}{
						"url": content.ImageURL,
					}
				} else if content.Data != "" {
					imageContent["image_url"] = map[string]interface{}{
						"url": fmt.Sprintf("data:%s;base64,%s", content.MediaType, content.Data),
					}
				}
				contentParts = append(contentParts, imageContent)

			case ContentTypeAudio:
				// OpenAI 音频输入格式
				audioContent := map[string]interface{}{
					"type": "input_audio",
				}
				if content.Data != "" {
					audioContent["input_audio"] = map[string]interface{}{
						"data":   content.Data,
						"format": extractFormat(content.MediaType),
					}
				}
				contentParts = append(contentParts, audioContent)
			}
		}

		// 将内容部分序列化到 JSON 的内容字段
		contentJSON, err := json.Marshal(contentParts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %w", err)
		}

		result = append(result, llm.Message{
			Role:    llm.Role(msg.Role),
			Content: string(contentJSON),
		})
	}

	return result, nil
}

// convertToAnthropic 转换为 Anthropic 的多模态格式.
func (p *Processor) convertToAnthropic(messages []MultimodalMessage) ([]llm.Message, error) {
	var result []llm.Message

	for _, msg := range messages {
		var contentParts []map[string]interface{}

		for _, content := range msg.Contents {
			switch content.Type {
			case ContentTypeText:
				contentParts = append(contentParts, map[string]interface{}{
					"type": "text",
					"text": content.Text,
				})

			case ContentTypeImage:
				imageContent := map[string]interface{}{
					"type": "image",
				}
				if content.Data != "" {
					imageContent["source"] = map[string]interface{}{
						"type":       "base64",
						"media_type": content.MediaType,
						"data":       content.Data,
					}
				} else if content.ImageURL != "" {
					imageContent["source"] = map[string]interface{}{
						"type": "url",
						"url":  content.ImageURL,
					}
				}
				contentParts = append(contentParts, imageContent)
			}
		}

		contentJSON, err := json.Marshal(contentParts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %w", err)
		}

		result = append(result, llm.Message{
			Role:    llm.Role(msg.Role),
			Content: string(contentJSON),
		})
	}

	return result, nil
}

// convertToGemini 转换为 Gemini 的多模态格式.
func (p *Processor) convertToGemini(messages []MultimodalMessage) ([]llm.Message, error) {
	var result []llm.Message

	for _, msg := range messages {
		var parts []map[string]interface{}

		for _, content := range msg.Contents {
			switch content.Type {
			case ContentTypeText:
				parts = append(parts, map[string]interface{}{
					"text": content.Text,
				})

			case ContentTypeImage:
				if content.Data != "" {
					parts = append(parts, map[string]interface{}{
						"inline_data": map[string]interface{}{
							"mime_type": content.MediaType,
							"data":      content.Data,
						},
					})
				} else if content.ImageURL != "" {
					parts = append(parts, map[string]interface{}{
						"file_data": map[string]interface{}{
							"file_uri":  content.ImageURL,
							"mime_type": content.MediaType,
						},
					})
				}

			case ContentTypeAudio:
				if content.Data != "" {
					parts = append(parts, map[string]interface{}{
						"inline_data": map[string]interface{}{
							"mime_type": content.MediaType,
							"data":      content.Data,
						},
					})
				}

			case ContentTypeVideo:
				if content.VideoURL != "" {
					parts = append(parts, map[string]interface{}{
						"file_data": map[string]interface{}{
							"file_uri":  content.VideoURL,
							"mime_type": "video/mp4",
						},
					})
				}
			}
		}

		contentJSON, err := json.Marshal(parts)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal content: %w", err)
		}

		result = append(result, llm.Message{
			Role:    llm.Role(msg.Role),
			Content: string(contentJSON),
		})
	}

	return result, nil
}

// convertToGeneric 转换为通用格式（仅文本回退）.
func (p *Processor) convertToGeneric(messages []MultimodalMessage) ([]llm.Message, error) {
	var result []llm.Message

	for _, msg := range messages {
		var textParts []string
		for _, content := range msg.Contents {
			if content.Type == ContentTypeText {
				textParts = append(textParts, content.Text)
			} else {
				textParts = append(textParts, fmt.Sprintf("[%s content: %s]", content.Type, content.FileName))
			}
		}

		result = append(result, llm.Message{
			Role:    llm.Role(msg.Role),
			Content: joinStrings(textParts, "\n"),
		})
	}

	return result, nil
}

func extractFormat(mediaType string) string {
	// 从 "audio/mp3" 等媒体类型提取格式 -> "mp3"
	if len(mediaType) > 6 && mediaType[:6] == "audio/" {
		return mediaType[6:]
	}
	if len(mediaType) > 6 && mediaType[:6] == "image/" {
		return mediaType[6:]
	}
	return mediaType
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

// MultimodalRequest 以多模态内容扩展聊天请求.
type MultimodalRequest struct {
	llm.ChatRequest
	MultimodalMessages []MultimodalMessage `json:"multimodal_messages,omitempty"`
}

// MultimodalProvider 将提供者包装在多模态支持下.
type MultimodalProvider struct {
	provider  llm.Provider
	processor *Processor
}

// NewMultimodalProvider 创建支持多模态的提供者包装.
func NewMultimodalProvider(provider llm.Provider, processor *Processor) *MultimodalProvider {
	if processor == nil {
		processor = DefaultProcessor()
	}
	return &MultimodalProvider{
		provider:  provider,
		processor: processor,
	}
}

// Completion 发出多模态完成请求.
func (m *MultimodalProvider) Completion(ctx context.Context, req *MultimodalRequest) (*llm.ChatResponse, error) {
	if len(req.MultimodalMessages) > 0 {
		messages, err := m.processor.ConvertToProviderFormat(m.provider.Name(), req.MultimodalMessages)
		if err != nil {
			return nil, fmt.Errorf("failed to convert multimodal messages: %w", err)
		}
		req.ChatRequest.Messages = messages
	}

	return m.provider.Completion(ctx, &req.ChatRequest)
}

// Stream 发送多模态流请求.
func (m *MultimodalProvider) Stream(ctx context.Context, req *MultimodalRequest) (<-chan llm.StreamChunk, error) {
	if len(req.MultimodalMessages) > 0 {
		messages, err := m.processor.ConvertToProviderFormat(m.provider.Name(), req.MultimodalMessages)
		if err != nil {
			return nil, fmt.Errorf("failed to convert multimodal messages: %w", err)
		}
		req.ChatRequest.Messages = messages
	}

	return m.provider.Stream(ctx, &req.ChatRequest)
}

// Name 返回基础提供者名称.
func (m *MultimodalProvider) Name() string {
	return m.provider.Name()
}

// SupportsMultimodal 检查提供者是否支持多模态输入.
func (m *MultimodalProvider) SupportsMultimodal() bool {
	// 检查已知支持多模态的提供者名称
	switch m.provider.Name() {
	case "openai", "anthropic", "gemini":
		return true
	default:
		return false
	}
}

// SupportedModalities 返回提供者支持的模态.
func (m *MultimodalProvider) SupportedModalities() []ContentType {
	switch m.provider.Name() {
	case "openai":
		return []ContentType{ContentTypeText, ContentTypeImage, ContentTypeAudio}
	case "anthropic":
		return []ContentType{ContentTypeText, ContentTypeImage}
	case "gemini":
		return []ContentType{ContentTypeText, ContentTypeImage, ContentTypeAudio, ContentTypeVideo}
	default:
		return []ContentType{ContentTypeText}
	}
}

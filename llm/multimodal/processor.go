// 套件多式为LLM供应商提供多式内容处理.
package multimodal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
)

// 处理器处理不同提供者的多模式内容转换.
type Processor struct {
	visionConfig VisionConfig
	audioConfig  AudioConfig
}

// 新处理器创建了一个新的多式处理器.
func NewProcessor(visionCfg VisionConfig, audioCfg AudioConfig) *Processor {
	return &Processor{
		visionConfig: visionCfg,
		audioConfig:  audioCfg,
	}
}

// 默认处理器创建了默认配置的处理器.
func DefaultProcessor() *Processor {
	return NewProcessor(DefaultVisionConfig(), DefaultAudioConfig())
}

// 转换 ToProviderFormat 将多式消息转换为提供者专用格式.
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

// 转换为OpenAI的多模式格式。
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

// 将 ToAnthropic 转换为 Anthropic 的多式格式。
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

// 将ToGemini转换为双子座多式格式.
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

// 转换 ToGeneric 转换为通用格式(仅文本倒置).
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
	// 从“ audio/ mp3” 等介质类型提取格式 - > “ mp3”
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

// 多式联运请求以多式联运内容扩展聊天请求。
type MultimodalRequest struct {
	llm.ChatRequest
	MultimodalMessages []MultimodalMessage `json:"multimodal_messages,omitempty"`
}

// 多式联运提供方将供应商包裹在多式联运支持下。
type MultimodalProvider struct {
	provider  llm.Provider
	processor *Processor
}

// 新的多式联运提供商创建了一种认识到多式联运提供者的包装。
func NewMultimodalProvider(provider llm.Provider, processor *Processor) *MultimodalProvider {
	if processor == nil {
		processor = DefaultProcessor()
	}
	return &MultimodalProvider{
		provider:  provider,
		processor: processor,
	}
}

// 完成后发出多式完成请求。
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

// 流发送多模式流请求 。
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

// 名称返回基本提供者名称 。
func (m *MultimodalProvider) Name() string {
	return m.provider.Name()
}

// 如果供应商支持多式联运输入,则支持多式联运检查。
func (m *MultimodalProvider) SupportsMultimodal() bool {
	// 检查已知多式联运支持的提供者名称
	switch m.provider.Name() {
	case "openai", "anthropic", "gemini":
		return true
	default:
		return false
	}
}

// 支持模式返回提供者支持的模式。
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

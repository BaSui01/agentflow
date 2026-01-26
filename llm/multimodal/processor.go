// Package multimodal provides multimodal content handling for LLM providers.
package multimodal

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/BaSui01/agentflow/llm"
)

// Processor handles multimodal content conversion for different providers.
type Processor struct {
	visionConfig VisionConfig
	audioConfig  AudioConfig
}

// NewProcessor creates a new multimodal processor.
func NewProcessor(visionCfg VisionConfig, audioCfg AudioConfig) *Processor {
	return &Processor{
		visionConfig: visionCfg,
		audioConfig:  audioCfg,
	}
}

// DefaultProcessor creates a processor with default configurations.
func DefaultProcessor() *Processor {
	return NewProcessor(DefaultVisionConfig(), DefaultAudioConfig())
}

// ConvertToProviderFormat converts multimodal messages to provider-specific format.
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

// convertToOpenAI converts to OpenAI's multimodal format.
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
				// OpenAI audio input format
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

		// Serialize content parts to JSON for the Content field
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

// convertToAnthropic converts to Anthropic's multimodal format.
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

// convertToGemini converts to Gemini's multimodal format.
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

// convertToGeneric converts to a generic format (text-only fallback).
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
	// Extract format from media type like "audio/mp3" -> "mp3"
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

// MultimodalRequest extends ChatRequest with multimodal content.
type MultimodalRequest struct {
	llm.ChatRequest
	MultimodalMessages []MultimodalMessage `json:"multimodal_messages,omitempty"`
}

// MultimodalProvider wraps a provider with multimodal support.
type MultimodalProvider struct {
	provider  llm.Provider
	processor *Processor
}

// NewMultimodalProvider creates a multimodal-aware provider wrapper.
func NewMultimodalProvider(provider llm.Provider, processor *Processor) *MultimodalProvider {
	if processor == nil {
		processor = DefaultProcessor()
	}
	return &MultimodalProvider{
		provider:  provider,
		processor: processor,
	}
}

// Completion sends a multimodal completion request.
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

// Stream sends a multimodal streaming request.
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

// Name returns the underlying provider name.
func (m *MultimodalProvider) Name() string {
	return m.provider.Name()
}

// SupportsMultimodal checks if the provider supports multimodal input.
func (m *MultimodalProvider) SupportsMultimodal() bool {
	// Check provider name for known multimodal support
	switch m.provider.Name() {
	case "openai", "anthropic", "gemini":
		return true
	default:
		return false
	}
}

// SupportedModalities returns the modalities supported by the provider.
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

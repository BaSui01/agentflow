package image

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	googlegenai "github.com/BaSui01/agentflow/llm/internal/googlegenai"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"google.golang.org/genai"
)

// ---------- 模型常量 ----------

const (
	// GeminiModel25FlashImage 是 Gemini 第一代图像模型（2025.08），仅支持 1K 分辨率.
	// 定价约 $0.039/张，生成耗时 2-3 秒.
	GeminiModel25FlashImage = "gemini-2.5-flash-image"

	// GeminiModel3ProImage 是 Gemini Pro 图像模型（2025.11），最高 4K，最强质量.
	// 支持 Search grounding、up to 14 张参考图、thinking 模式.
	// 定价约 $0.134（1K）/$0.24（4K），生成耗时 8-12 秒.
	GeminiModel3ProImage = "gemini-3-pro-image-preview"

	// GeminiModel31FlashImage 是 Gemini 3.1 Flash 图像模型（2026.02），最高 4K，性价比最优.
	// 定价约 $0.05（1K）/$0.15（4K），生成耗时 4-6 秒.
	GeminiModel31FlashImage = "gemini-3.1-flash-image-preview"
)

// ---------- Provider 结构体 ----------

// GeminiProvider 利用 Google Gemini 原生多模态能力实现图像生成.
// 实现了 Provider 与 StreamingProvider 接口.
type GeminiProvider struct {
	cfg    GeminiConfig
	client *http.Client
}

// NewGeminiProvider 创建新的 Gemini 图像提供者.
func NewGeminiProvider(cfg GeminiConfig) *GeminiProvider {
	if cfg.Model == "" {
		cfg.Model = GeminiModel3ProImage
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
	}
}

func (p *GeminiProvider) Name() string { return "gemini-image" }

func (p *GeminiProvider) sdkClient(ctx context.Context) (*genai.Client, error) {
	return googlegenai.NewClient(ctx, googlegenai.ClientConfig{
		APIKey:     p.cfg.APIKey,
		BaseURL:    p.cfg.BaseURL,
		Timeout:    p.cfg.Timeout,
		HTTPClient: p.client,
	})
}

// SupportedSizes 返回 Gemini 原生分辨率格式.
// imageSize 参数传 "1K"/"2K"/"4K"，或通过 Metadata["image_size"] 指定.
// gemini-2.5-flash-image 仅支持 1K；gemini-3-pro-image 和 gemini-3.1-flash-image 支持 1K/2K/4K.
func (p *GeminiProvider) SupportedSizes() []string {
	return []string{"1K", "2K", "4K"}
}

// ---------- REST 请求/响应结构体 ----------

type geminiPart struct {
	Text       string        `json:"text,omitempty"`
	InlineData *geminiInline `json:"inlineData,omitempty"`
}

type geminiInline struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

// geminiSystemInstruction 对应 REST body 顶层 system_instruction 字段（snake_case，Google REST 规范）.
type geminiSystemInstruction struct {
	Parts []geminiPart `json:"parts"`
}

// geminiSafetySetting 安全过滤设置.
type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// geminiImageConfig 对应 generationConfig.imageConfig.
type geminiImageConfig struct {
	ImageSize        string `json:"imageSize,omitempty"`
	AspectRatio      string `json:"aspectRatio,omitempty"`
	PersonGeneration string `json:"personGeneration,omitempty"`
}

// geminiThinkingConfig 对应 generationConfig.thinkingConfig.
type geminiThinkingConfig struct {
	ThinkingBudget int `json:"thinkingBudget,omitempty"`
}

// geminiGenerationConfig 对应完整 generationConfig 字段.
type geminiGenerationConfig struct {
	ResponseMimeType   string                `json:"responseMimeType,omitempty"`
	ResponseModalities []string              `json:"responseModalities,omitempty"`
	ImageConfig        *geminiImageConfig    `json:"imageConfig,omitempty"`
	ThinkingConfig     *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
	CandidateCount     int                   `json:"candidateCount,omitempty"`
}

// geminiImageRequest 是发往 generateContent 接口的完整请求体.
// system_instruction 使用 snake_case（Google REST 规范）.
type geminiImageRequest struct {
	Contents          []geminiContent          `json:"contents"`
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Tools             []map[string]interface{} `json:"tools,omitempty"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
	SafetySettings    []geminiSafetySetting    `json:"safetySettings,omitempty"`
}

type geminiImageResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text       string `json:"text,omitempty"`
				InlineData *struct {
					MimeType string `json:"mimeType"`
					Data     string `json:"data"`
				} `json:"inlineData,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// ---------- Metadata 键常量 ----------

const (
	// MetaImageSize 指定 Gemini 分辨率，取值 "1K"/"2K"/"4K"，优先级高于 req.Size.
	MetaImageSize = "image_size"
	// MetaAspectRatio 指定宽高比，如 "16:9"/"1:1"/"9:16" 等.
	MetaAspectRatio = "aspect_ratio"
	// MetaResponseModalities 逗号分隔的响应模态，如 "TEXT,IMAGE"/"IMAGE".
	MetaResponseModalities = "response_modalities"
	// MetaEnableSearch 为 "true" 时启用 Google Search grounding（仅 gemini-3-pro-image 支持）.
	MetaEnableSearch = "enable_search"
	// MetaSystemPrompt 系统提示词文本.
	MetaSystemPrompt = "system_prompt"
	// MetaThinkingBudget thinking token 预算，整数字符串，如 "1024".
	MetaThinkingBudget = "thinking_budget"
	// MetaPersonGeneration 人物生成策略："ALLOW_ALL"/"ALLOW_ADULT"/"ALLOW_NONE".
	MetaPersonGeneration = "person_generation"
	// MetaSafetyThreshold 安全过滤阈值，应用于所有 harm category.
	// 取值："BLOCK_NONE"/"BLOCK_FEW"/"BLOCK_SOME"/"BLOCK_MOST".
	MetaSafetyThreshold = "safety_threshold"
	// MetaCandidateCount 候选数量，整数字符串（默认 1）.
	MetaCandidateCount = "candidate_count"
)

// harmCategories 是 Gemini 内容安全过滤的完整 harm category 列表.
var harmCategories = []string{
	"HARM_CATEGORY_HARASSMENT",
	"HARM_CATEGORY_HATE_SPEECH",
	"HARM_CATEGORY_SEXUALLY_EXPLICIT",
	"HARM_CATEGORY_DANGEROUS_CONTENT",
	"HARM_CATEGORY_CIVIC_INTEGRITY",
}

// ---------- 辅助函数 ----------

// sizeToGemini 将通用 Size 字段映射到 Gemini imageSize 格式.
// 如已是 "1K"/"2K"/"4K" 则直接返回；如为像素格式则做转换.
func sizeToGemini(size string) string {
	switch strings.ToUpper(size) {
	case "1K":
		return "1K"
	case "2K":
		return "2K"
	case "4K":
		return "4K"
	case "1024X1024", "1024X768", "768X1024":
		return "1K"
	case "2048X2048", "2048X1536", "1536X2048", "1536X1536":
		return "2K"
	case "4096X4096", "4096X2160", "2160X4096":
		return "4K"
	}
	return ""
}

// buildGenConfig 根据请求元数据构建 generationConfig.
func buildGenConfig(req *GenerateRequest, defaultModalities []string) *geminiGenerationConfig {
	meta := req.Metadata

	// responseModalities
	modalities := defaultModalities
	if v := meta[MetaResponseModalities]; v != "" {
		parts := strings.Split(v, ",")
		modalities = make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(strings.ToUpper(p)); t != "" {
				modalities = append(modalities, t)
			}
		}
	}

	// imageConfig
	imgCfg := &geminiImageConfig{}
	populated := false

	if v := meta[MetaImageSize]; v != "" {
		imgCfg.ImageSize = strings.ToUpper(v)
		populated = true
	} else if s := sizeToGemini(req.Size); s != "" {
		imgCfg.ImageSize = s
		populated = true
	}

	if v := meta[MetaAspectRatio]; v != "" {
		imgCfg.AspectRatio = v
		populated = true
	}

	if v := meta[MetaPersonGeneration]; v != "" {
		imgCfg.PersonGeneration = strings.ToUpper(v)
		populated = true
	}

	var imgCfgPtr *geminiImageConfig
	if populated {
		imgCfgPtr = imgCfg
	}

	// thinkingConfig
	var thinkPtr *geminiThinkingConfig
	if v := meta[MetaThinkingBudget]; v != "" {
		if budget, err := strconv.Atoi(v); err == nil && budget > 0 {
			thinkPtr = &geminiThinkingConfig{ThinkingBudget: budget}
		}
	}

	// candidateCount
	candidateCount := 0
	if v := meta[MetaCandidateCount]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			candidateCount = n
		}
	}

	return &geminiGenerationConfig{
		ResponseModalities: modalities,
		ImageConfig:        imgCfgPtr,
		ThinkingConfig:     thinkPtr,
		CandidateCount:     candidateCount,
	}
}

func buildGenerateContentConfigFromImageRequest(req *GenerateRequest, allowSearch bool) (*genai.GenerateContentConfig, string) {
	req.Metadata = ensureMeta(req.Metadata)
	config := &genai.GenerateContentConfig{
		ResponseModalities: []string{"IMAGE"},
		ImageConfig:        &genai.ImageConfig{},
	}

	if v := req.Metadata[MetaResponseModalities]; v != "" {
		parts := strings.Split(v, ",")
		config.ResponseModalities = make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.ToUpper(strings.TrimSpace(part))
			if part != "" {
				config.ResponseModalities = append(config.ResponseModalities, part)
			}
		}
	}

	if v := req.Metadata[MetaImageSize]; v != "" {
		config.ImageConfig.ImageSize = strings.ToUpper(strings.TrimSpace(v))
	} else if size := sizeToGemini(req.Size); size != "" {
		config.ImageConfig.ImageSize = size
	}
	if v := req.Metadata[MetaAspectRatio]; v != "" {
		config.ImageConfig.AspectRatio = v
	}
	if v := req.Metadata[MetaPersonGeneration]; v != "" {
		config.ImageConfig.PersonGeneration = strings.ToUpper(strings.TrimSpace(v))
	}
	if config.ImageConfig.ImageSize == "" && config.ImageConfig.AspectRatio == "" && config.ImageConfig.PersonGeneration == "" {
		config.ImageConfig = nil
	}
	if v := req.Metadata[MetaThinkingBudget]; v != "" {
		if budget, err := strconv.Atoi(v); err == nil && budget > 0 {
			b := int32(budget)
			config.ThinkingConfig = &genai.ThinkingConfig{
				IncludeThoughts: true,
				ThinkingBudget:  &b,
			}
		}
	}
	if v := req.Metadata[MetaCandidateCount]; v != "" {
		if count, err := strconv.Atoi(v); err == nil && count > 0 {
			config.CandidateCount = int32(count)
		}
	} else if req.N > 0 {
		config.CandidateCount = int32(req.N)
	}
	if v := req.Metadata[MetaSystemPrompt]; v != "" {
		config.SystemInstruction = genai.NewContentFromText(v, genai.RoleUser)
	}
	if v := req.Metadata[MetaSafetyThreshold]; v != "" {
		threshold := genai.HarmBlockThreshold(strings.TrimSpace(v))
		config.SafetySettings = []*genai.SafetySetting{
			{Category: genai.HarmCategoryHarassment, Threshold: threshold},
			{Category: genai.HarmCategoryHateSpeech, Threshold: threshold},
			{Category: genai.HarmCategorySexuallyExplicit, Threshold: threshold},
			{Category: genai.HarmCategoryDangerousContent, Threshold: threshold},
			{Category: genai.HarmCategoryCivicIntegrity, Threshold: threshold},
		}
	}
	if allowSearch && strings.EqualFold(strings.TrimSpace(req.Metadata[MetaEnableSearch]), "true") {
		config.Tools = []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}}
	}

	prompt := strings.TrimSpace(req.Prompt)
	if req.NegativePrompt != "" {
		prompt = strings.TrimSpace(prompt + "\nAvoid: " + req.NegativePrompt)
	}
	return config, prompt
}

func imageDataFromGenerateContentResponse(resp *genai.GenerateContentResponse) []ImageData {
	images := make([]ImageData, 0)
	if resp == nil {
		return images
	}
	for _, candidate := range resp.Candidates {
		if candidate == nil || candidate.Content == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part == nil || part.InlineData == nil || len(part.InlineData.Data) == 0 {
				continue
			}
			images = append(images, ImageData{
				B64JSON: base64.StdEncoding.EncodeToString(part.InlineData.Data),
			})
		}
	}
	return images
}

// buildTools 根据元数据构建 tools 列表.
// Google Search grounding 仅 gemini-3-pro-image-preview 官方支持；其他模型忽略该字段.
func buildTools(meta map[string]string) []map[string]interface{} {
	if meta[MetaEnableSearch] == "true" {
		return []map[string]interface{}{
			{"google_search": map[string]interface{}{}},
		}
	}
	return nil
}

// buildSystemInstruction 根据元数据构建 system_instruction.
func buildSystemInstruction(meta map[string]string) *geminiSystemInstruction {
	if text := meta[MetaSystemPrompt]; text != "" {
		return &geminiSystemInstruction{
			Parts: []geminiPart{{Text: text}},
		}
	}
	return nil
}

// buildSafetySettings 根据元数据构建 safetySettings.
// 若 MetaSafetyThreshold 存在，对所有 harm category 统一设置阈值.
func buildSafetySettings(meta map[string]string) []geminiSafetySetting {
	threshold := strings.ToUpper(meta[MetaSafetyThreshold])
	if threshold == "" {
		return nil
	}
	settings := make([]geminiSafetySetting, 0, len(harmCategories))
	for _, cat := range harmCategories {
		settings = append(settings, geminiSafetySetting{
			Category:  cat,
			Threshold: threshold,
		})
	}
	return settings
}

// ensureMeta 确保 Metadata 不为 nil.
func ensureMeta(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

// ---------- 非流式内部方法 ----------

// doRequest 发送请求并解析响应，提取图片数据.
func (p *GeminiProvider) doRequest(ctx context.Context, model string, body geminiImageRequest) ([]ImageData, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.buildURL(model, false)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini error: status=%d body=%s", resp.StatusCode, string(errBody))
	}

	var gResp geminiImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&gResp); err != nil {
		return nil, fmt.Errorf("failed to decode gemini response: %w", err)
	}

	return extractImages(gResp), nil
}

// buildURL 构造 generateContent 或 streamGenerateContent 请求 URL.
func (p *GeminiProvider) buildURL(model string, streaming bool) string {
	baseURL := p.cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}
	endpoint := "generateContent"
	if streaming {
		endpoint = "streamGenerateContent"
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:%s?key=%s", baseURL, model, endpoint, p.cfg.APIKey)
	if streaming {
		url += "&alt=sse"
	}
	return url
}

// extractImages 从 geminiImageResponse 中提取所有 inlineData 图像.
func extractImages(gResp geminiImageResponse) []ImageData {
	var images []ImageData
	for _, candidate := range gResp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				images = append(images, ImageData{
					B64JSON: part.InlineData.Data,
				})
			}
		}
	}
	return images
}

// buildRequest 构建标准图像请求体.
func buildRequest(contents []geminiContent, meta map[string]string, genReq *GenerateRequest) geminiImageRequest {
	return geminiImageRequest{
		Contents:          contents,
		SystemInstruction: buildSystemInstruction(meta),
		Tools:             buildTools(meta),
		GenerationConfig:  buildGenConfig(genReq, []string{"IMAGE"}),
		SafetySettings:    buildSafetySettings(meta),
	}
}

// ---------- Provider 接口实现 ----------

// Generate 利用 Gemini 原生能力生成图像.
//
// 支持通过 req.Metadata 传入 Gemini 专属参数：
//   - "image_size"          : "1K"/"2K"/"4K"（优先级高于 req.Size）
//   - "aspect_ratio"        : "1:1"/"16:9"/"9:16"/"2:3"/"3:2"/"3:4"/"4:3"/"4:5"/"5:4"/"21:9"
//   - "response_modalities" : "IMAGE" 或 "TEXT,IMAGE"（默认 "IMAGE"）
//   - "enable_search"       : "true" 启用 Google Search grounding（仅 Pro 模型支持）
//   - "system_prompt"       : 系统提示词文本
//   - "thinking_budget"     : thinking token 预算，整数字符串（如 "1024"）
//   - "person_generation"   : "ALLOW_ALL"/"ALLOW_ADULT"/"ALLOW_NONE"
//   - "safety_threshold"    : "BLOCK_NONE"/"BLOCK_FEW"/"BLOCK_SOME"/"BLOCK_MOST"
//   - "candidate_count"     : 候选数量，整数字符串
func (p *GeminiProvider) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	req.Metadata = ensureMeta(req.Metadata)
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create google genai client: %w", err)
	}

	config, prompt := buildGenerateContentConfigFromImageRequest(req, true)
	resp, err := client.Models.GenerateContent(ctx, model, genai.Text(prompt), config)
	if err != nil {
		return nil, fmt.Errorf("gemini image request failed: %w", err)
	}
	images := imageDataFromGenerateContentResponse(resp)

	return &GenerateResponse{
		Provider: p.Name(),
		Model:    model,
		Images:   images,
		Usage: ImageUsage{
			ImagesGenerated: len(images),
		},
		CreatedAt: time.Now(),
	}, nil
}

// Edit 利用 Gemini 多模态能力编辑修改已有图像.
//
// 支持与 Generate 相同的 req.Metadata 扩展参数（image_size/aspect_ratio/enable_search 等）.
func (p *GeminiProvider) Edit(ctx context.Context, req *EditRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	imageData, err := io.ReadAll(req.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	req.Metadata = ensureMeta(req.Metadata)
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create google genai client: %w", err)
	}

	genReq := &GenerateRequest{Prompt: req.Prompt, Size: req.Size, Metadata: req.Metadata}
	config, prompt := buildGenerateContentConfigFromImageRequest(genReq, true)
	contents := []*genai.Content{
		genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromBytes(imageData, "image/png"),
			genai.NewPartFromText(prompt),
		}, genai.RoleUser),
	}

	resp, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini image edit request failed: %w", err)
	}
	images := imageDataFromGenerateContentResponse(resp)

	return &GenerateResponse{
		Provider:  p.Name(),
		Model:     model,
		Images:    images,
		CreatedAt: time.Now(),
	}, nil
}

// CreateVariation 使用 Gemini 创建图像变体.
func (p *GeminiProvider) CreateVariation(ctx context.Context, req *VariationRequest) (*GenerateResponse, error) {
	if req.Image == nil {
		return nil, fmt.Errorf("image is required")
	}

	editReq := &EditRequest{
		Image:    req.Image,
		Prompt:   "Create a variation of this image with similar style and composition but different details",
		Model:    req.Model,
		Size:     req.Size,
		Metadata: req.Metadata,
	}

	return p.Edit(ctx, editReq)
}

// ---------- StreamingProvider 接口实现 ----------

// GenerateStream 利用 Gemini streamGenerateContent 接口流式生成图像.
//
// 流式机制：
//   - 当 responseModalities 包含 "TEXT" 时，模型会先输出描述文字（逐 token 流式到达），
//     再输出图像数据（完整 inlineData 在一个 chunk 中到达）；
//   - 当 responseModalities 为 ["IMAGE"] 时，无文字输出，直接返回图像 chunk.
//
// 默认模态为 "TEXT,IMAGE"（流式场景下以获得进度反馈），可通过
// req.Metadata["response_modalities"]="IMAGE" 覆盖为纯图像模式.
//
// emit 回调约定：
//   - chunk.Text != ""：文字 token，直接展示给用户；
//   - chunk.Image != nil：完整图像数据；
//   - chunk.Done == true：流正常结束；
//   - chunk.Err != nil：流异常终止.
func (p *GeminiProvider) GenerateStream(ctx context.Context, req *GenerateRequest, emit func(StreamChunk)) error {
	model := req.Model
	if model == "" {
		model = p.cfg.Model
	}
	req.Metadata = ensureMeta(req.Metadata)

	// 流式默认使用 TEXT+IMAGE，使调用方能获得进度反馈
	if req.Metadata[MetaResponseModalities] == "" {
		req.Metadata[MetaResponseModalities] = "TEXT,IMAGE"
	}
	client, err := p.sdkClient(ctx)
	if err != nil {
		return fmt.Errorf("failed to create google genai client: %w", err)
	}

	config, prompt := buildGenerateContentConfigFromImageRequest(req, true)
	for result, err := range client.Models.GenerateContentStream(ctx, model, genai.Text(prompt), config) {
		if err != nil {
			emit(StreamChunk{Err: err})
			return err
		}
		if result == nil {
			continue
		}
		for _, candidate := range result.Candidates {
			if candidate == nil || candidate.Content == nil {
				continue
			}
			for _, part := range candidate.Content.Parts {
				if part == nil {
					continue
				}
				if part.Text != "" && !part.Thought {
					emit(StreamChunk{Text: part.Text})
					continue
				}
				if part.InlineData != nil && len(part.InlineData.Data) > 0 {
					emit(StreamChunk{Image: &ImageData{B64JSON: base64.StdEncoding.EncodeToString(part.InlineData.Data)}})
				}
			}
		}
	}

	emit(StreamChunk{Done: true})
	return nil
}

// parseSSE 解析 Gemini SSE 响应流，通过 emit 推送 StreamChunk.
func (p *GeminiProvider) parseSSE(ctx context.Context, body io.Reader, emit func(StreamChunk)) error {
	scanner := bufio.NewScanner(body)
	// 调大 buf 以应对可能的大 base64 图像数据行
	buf := make([]byte, 0, 512*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			emit(StreamChunk{Err: err})
			return err
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk geminiImageResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 跳过非 JSON 行（注释、心跳等）
			continue
		}

		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					emit(StreamChunk{Text: part.Text})
				} else if part.InlineData != nil {
					emit(StreamChunk{Image: &ImageData{B64JSON: part.InlineData.Data}})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		emit(StreamChunk{Err: fmt.Errorf("SSE scan error: %w", err)})
		return err
	}

	emit(StreamChunk{Done: true})
	return nil
}

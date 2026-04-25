package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	googlegenai "github.com/BaSui01/agentflow/llm/internal/googlegenai"
	providerbase "github.com/BaSui01/agentflow/llm/providers/base"
	"google.golang.org/genai"

	"github.com/BaSui01/agentflow/types"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/middleware"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"go.uber.org/zap"
)

// 官方端点（可被配置 BaseURL 覆盖）：
// - Google AI（API Key）：https://generativelanguage.googleapis.com
//   - POST /v1beta/models/{model}:generateContent
//   - POST /v1beta/models/{model}:streamGenerateContent?alt=sse
//   - GET  /v1beta/models
//
// - Vertex AI（ProjectID 非空）：https://{region}-aiplatform.googleapis.com
//   - POST /v1/projects/{project}/locations/{region}/publishers/google/models/{model}:generateContent
//   - POST /v1/projects/{project}/locations/{region}/publishers/google/models/{model}:streamGenerateContent?alt=sse
//   - GET  /v1/projects/{project}/locations/{region}/publishers/google/models
const (
	defaultGoogleAIBaseURL = "https://generativelanguage.googleapis.com"
	vertexAIHostPattern    = "https://%s-aiplatform.googleapis.com"
	defaultVertexRegion    = "us-central1"
)

// GeminiProvider 实现 Google Gemini 的 LLM Provider
// Gemini API 特点：
// 1. 使用 x-goog-api-key 请求头认证
// 2. 多模态支持（文本、图片、音频、视频）
// 3. 支持长上下文（最高 1M tokens）
// 4. 原生工具调用支持
type GeminiProvider struct {
	cfg           providers.GeminiConfig
	client        *http.Client
	logger        *zap.Logger
	rewriterChain *middleware.RewriterChain
	keyIndex      uint64 // 多 Key 轮询索引
}

// NewGeminiProvider 创建 Gemini Provider
func NewGeminiProvider(cfg providers.GeminiConfig, logger *zap.Logger) *GeminiProvider {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	// Vertex AI 模式：设置默认 Region
	if cfg.ProjectID != "" && cfg.Region == "" {
		cfg.Region = defaultVertexRegion
	}

	// 设置默认 BaseURL（未配置时使用官方端点）
	if cfg.BaseURL == "" {
		if cfg.ProjectID != "" {
			cfg.BaseURL = fmt.Sprintf(vertexAIHostPattern, cfg.Region)
		} else {
			cfg.BaseURL = defaultGoogleAIBaseURL
		}
	}

	return &GeminiProvider{
		cfg:    cfg,
		client: tlsutil.SecureHTTPClient(timeout),
		logger: logger,
		rewriterChain: middleware.NewRewriterChain(
			middleware.NewXMLToolRewriter(),
			middleware.NewEmptyToolsCleaner(),
		),
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) sdkClient(ctx context.Context) (*genai.Client, error) {
	return googlegenai.NewClient(ctx, googlegenai.ClientConfig{
		APIKey:     p.resolveAPIKey(ctx),
		BaseURL:    p.cfg.BaseURL,
		ProjectID:  p.cfg.ProjectID,
		Region:     p.cfg.Region,
		AuthType:   p.cfg.AuthType,
		Timeout:    p.cfg.Timeout,
		HTTPClient: p.client,
	})
}

func (p *GeminiProvider) mapSDKError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return providerbase.MapHTTPError(apiErr.Code, strings.TrimSpace(apiErr.Message), p.Name())
	}

	return &types.Error{
		Code:       llm.ErrUpstreamError,
		Message:    err.Error(),
		Cause:      err,
		HTTPStatus: http.StatusBadGateway,
		Retryable:  true,
		Provider:   p.Name(),
	}
}

func (p *GeminiProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	start := time.Now()
	client, err := p.sdkClient(ctx)
	latency := time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, p.mapSDKError(err)
	}
	_, err = client.Models.List(ctx, &genai.ListModelsConfig{PageSize: 1})
	latency = time.Since(start)
	if err != nil {
		return &llm.HealthStatus{Healthy: false, Latency: latency}, p.mapSDKError(err)
	}

	return &llm.HealthStatus{Healthy: true, Latency: latency}, nil
}

func (p *GeminiProvider) SupportsNativeFunctionCalling() bool { return true }

// SupportsStructuredOutput returns true because Gemini supports native
// structured output via responseMimeType + responseSchema.
func (p *GeminiProvider) SupportsStructuredOutput() bool { return true }

// ListModels 获取 Gemini 支持的模型列表
func (p *GeminiProvider) ListModels(ctx context.Context) ([]llm.Model, error) {
	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	page, err := client.Models.List(ctx, &genai.ListModelsConfig{})
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	source := page.Items
	for page.NextPageToken != "" {
		page, err = page.Next(ctx)
		if err != nil {
			return nil, p.mapSDKError(err)
		}
		source = append(source, page.Items...)
	}

	// 转换为统一格式
	models := make([]llm.Model, 0, len(source))
	for _, m := range source {
		if m == nil {
			continue
		}
		// 提取模型 ID（去掉 "models/" 前缀）
		modelID := strings.TrimSpace(firstNonEmpty(
			strings.TrimPrefix(strings.TrimSpace(m.Name), "models/"),
		))
		if modelID == "" {
			continue
		}
		caps := resolveGeminiCapabilities(modelID, m.DisplayName, m.Description, m.SupportedActions)
		model := llm.Model{
			ID:              modelID,
			Object:          "model",
			OwnedBy:         "google",
			MaxInputTokens:  int(m.InputTokenLimit),
			MaxOutputTokens: int(m.OutputTokenLimit),
			Capabilities:    caps,
		}
		models = append(models, model)
	}

	return models, nil
}

func resolveGeminiCapabilities(modelID, displayName, description string, methods []string) []string {
	fingerprint := strings.ToLower(strings.TrimSpace(strings.Join([]string{modelID, displayName, description}, " ")))
	caps := make([]string, 0, 6)
	seen := make(map[string]struct{}, 6)
	addCap := func(cap string) {
		if strings.TrimSpace(cap) == "" {
			return
		}
		if _, ok := seen[cap]; ok {
			return
		}
		seen[cap] = struct{}{}
		caps = append(caps, cap)
	}

	for _, method := range methods {
		switch strings.ToLower(strings.TrimSpace(method)) {
		case "embedcontent":
			addCap("embedding")
		case "counttokens":
			addCap("token_counting")
		case "createtunedmodel":
			addCap("fine_tuning")
		case "generateanswer":
			addCap("question_answering")
		}
	}

	switch {
	case isGeminiEmbeddingModel(fingerprint):
		addCap("embedding")
	case isGeminiVideoGenerationModel(fingerprint):
		addCap("video-gen")
	case isGeminiAudioGenerationModel(fingerprint):
		addCap("audio-gen")
	case isGeminiImageGenerationModel(fingerprint):
		addCap("image-gen")
		addCap("vision")
	default:
		if hasGeminiGenerateContent(methods) || strings.Contains(fingerprint, "gemini") {
			addCap("chat")
			addCap("vision")
			addCap("tool_calls")
			addCap("function_call")
		}
	}

	if len(caps) == 0 {
		addCap("chat")
	}
	return caps
}

func hasGeminiGenerateContent(methods []string) bool {
	for _, method := range methods {
		if strings.EqualFold(strings.TrimSpace(method), "generateContent") {
			return true
		}
	}
	return false
}

func isGeminiEmbeddingModel(fingerprint string) bool {
	return strings.Contains(fingerprint, "embedding")
}

func isGeminiImageGenerationModel(fingerprint string) bool {
	return strings.Contains(fingerprint, "imagen") ||
		strings.Contains(fingerprint, "image generation") ||
		strings.Contains(fingerprint, "generate images") ||
		strings.Contains(fingerprint, "nano banana") ||
		strings.Contains(fingerprint, "nanobanana") ||
		strings.Contains(fingerprint, "-image") ||
		strings.Contains(fingerprint, "image-preview")
}

func isGeminiVideoGenerationModel(fingerprint string) bool {
	return strings.Contains(fingerprint, "veo") ||
		strings.Contains(fingerprint, "video generation") ||
		strings.Contains(fingerprint, "generate videos")
}

func isGeminiAudioGenerationModel(fingerprint string) bool {
	return strings.Contains(fingerprint, "preview-tts") ||
		strings.Contains(fingerprint, "text-to-speech") ||
		strings.Contains(fingerprint, "speech generation") ||
		strings.Contains(fingerprint, "audio generation")
}

func (p *GeminiProvider) isVertexAI() bool {
	return p.cfg.ProjectID != ""
}

func (p *GeminiProvider) completionEndpoint(model string) string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
			base, p.cfg.ProjectID, p.cfg.Region, model)
	}
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", base, model)
}

func (p *GeminiProvider) streamEndpoint(model string) string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models/%s:streamGenerateContent?alt=sse",
			base, p.cfg.ProjectID, p.cfg.Region, model)
	}
	return fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse", base, model)
}

func (p *GeminiProvider) modelsEndpoint() string {
	base := strings.TrimRight(p.cfg.BaseURL, "/")
	if p.isVertexAI() {
		return fmt.Sprintf("%s/v1/projects/%s/locations/%s/publishers/google/models",
			base, p.cfg.ProjectID, p.cfg.Region)
	}
	return fmt.Sprintf("%s/v1beta/models", base)
}

// Endpoints 返回该提供者使用的所有 API 端点完整 URL。
func (p *GeminiProvider) Endpoints() llm.ProviderEndpoints {
	// 使用默认模型来展示端点格式
	model := p.cfg.Model
	if model == "" {
		model = "gemini-2.5-flash"
	}
	return llm.ProviderEndpoints{
		Completion: p.completionEndpoint(model),
		Stream:     p.streamEndpoint(model),
		Models:     p.modelsEndpoint(),
		BaseURL:    p.cfg.BaseURL,
	}
}

func (p *GeminiProvider) buildHeaders(req *http.Request, apiKey string) {
	if p.cfg.AuthType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("x-goog-api-key", apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// resolveAPIKey 解析 API Key，支持上下文覆盖和多 Key 轮询
func (p *GeminiProvider) resolveAPIKey(ctx context.Context) string {
	if c, ok := llm.CredentialOverrideFromContext(ctx); ok {
		if strings.TrimSpace(c.APIKey) != "" {
			return strings.TrimSpace(c.APIKey)
		}
	}
	if len(p.cfg.APIKeys) > 0 {
		idx := atomic.AddUint64(&p.keyIndex, 1) - 1
		return p.cfg.APIKeys[idx%uint64(len(p.cfg.APIKeys))].Key
	}
	return p.cfg.APIKey
}

func convertToGenAIContents(msgs []types.Message) (*genai.Content, []*genai.Content) {
	var systemInstruction *genai.Content
	contents := make([]*genai.Content, 0, len(msgs))

	for _, m := range msgs {
		if m.Role == llm.RoleSystem {
			systemInstruction = genai.NewContentFromText(m.Content, genai.RoleUser)
			continue
		}

		var role genai.Role = genai.RoleUser
		if m.Role == llm.RoleAssistant {
			role = genai.RoleModel
		}

		parts := make([]*genai.Part, 0, 1+len(m.ToolCalls))
		partIndex := 0
		if m.Role == llm.RoleAssistant && m.ReasoningContent != nil && strings.TrimSpace(*m.ReasoningContent) != "" {
			parts = append(parts, &genai.Part{
				Text:             *m.ReasoningContent,
				Thought:          true,
				ThoughtSignature: decodeThoughtSignature(geminiThoughtSignatureByIndex(m, partIndex)),
			})
			partIndex++
		}

		if m.Content != "" && m.Role != llm.RoleTool {
			parts = append(parts, &genai.Part{
				Text:             m.Content,
				ThoughtSignature: decodeThoughtSignature(geminiThoughtSignatureByIndex(m, partIndex)),
			})
			partIndex++
		}

		if len(m.ToolCalls) > 0 {
			for _, tc := range m.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal(tc.Arguments, &args); err != nil {
					continue
				}
				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   tc.ID,
						Name: tc.Name,
						Args: args,
					},
					ThoughtSignature: decodeThoughtSignature(geminiThoughtSignatureByIndex(m, partIndex)),
				})
				partIndex++
			}
		}

		if m.Role == llm.RoleTool && m.ToolCallID != "" {
			writeback, ok := providerbase.ToolOutputFromMessage(m, nil)
			if !ok {
				continue
			}
			parts = append(parts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:       writeback.CallID,
					Name:     writeback.Name,
					Response: providerbase.BuildGeminiFunctionResponse(writeback),
				},
			})
		}

		if len(parts) > 0 {
			contents = append(contents, genai.NewContentFromParts(parts, role))
		}
	}

	return systemInstruction, contents
}

func convertToGenAITools(tools []types.ToolSchema, wsOpts *llm.WebSearchOptions) []*genai.Tool {
	needGoogleSearch := wsOpts != nil
	declarations := make([]*genai.FunctionDeclaration, 0, len(tools))
	for _, t := range tools {
		if providerbase.IsSearchToolPlaceholder(t.Name) {
			needGoogleSearch = true
			continue
		}
		params := providerbase.ToolParametersSchemaMap(t.Parameters)
		if len(params) == 0 {
			continue
		}
		declarations = append(declarations, &genai.FunctionDeclaration{
			Name:                 t.Name,
			Description:          t.Description,
			ParametersJsonSchema: params,
		})
	}

	result := make([]*genai.Tool, 0, 2)
	if len(declarations) > 0 {
		result = append(result, &genai.Tool{
			FunctionDeclarations: declarations,
		})
	}
	if needGoogleSearch {
		result = append(result, &genai.Tool{
			GoogleSearch: &genai.GoogleSearch{},
		})
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func convertToolChoiceToGenAI(toolChoice any, includeServerSide *bool) *genai.ToolConfig {
	var cfg *genai.ToolConfig
	buildMode := func(mode genai.FunctionCallingConfigMode, allowed []string) *genai.ToolConfig {
		out := &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{
				Mode: mode,
			},
		}
		if len(allowed) > 0 {
			out.FunctionCallingConfig.AllowedFunctionNames = allowed
		}
		return out
	}

	spec := providerbase.NormalizeToolChoice(toolChoice)
	switch spec.Mode {
	case "auto":
		cfg = buildMode(genai.FunctionCallingConfigModeAuto, nil)
	case "any":
		cfg = buildMode(genai.FunctionCallingConfigModeAny, spec.AllowedFunctionNames)
	case "none":
		cfg = buildMode(genai.FunctionCallingConfigModeNone, nil)
	case "validated":
		cfg = buildMode(genai.FunctionCallingConfigModeValidated, spec.AllowedFunctionNames)
	case "tool":
		cfg = buildMode(genai.FunctionCallingConfigModeAny, providerbase.NormalizeUniqueStrings([]string{spec.SpecificName}))
	}
	if spec.IncludeServerSideToolUse != nil {
		includeServerSide = spec.IncludeServerSideToolUse
	}

	if includeServerSide != nil {
		if cfg == nil {
			cfg = &genai.ToolConfig{}
		}
		cfg.IncludeServerSideToolInvocations = includeServerSide
	}
	return cfg
}

func buildGenAIGenerationConfig(req *llm.ChatRequest, safetySettings []providers.GeminiSafetySetting) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}

	if req.Temperature != 0 {
		cfg.Temperature = genai.Ptr(req.Temperature)
	}
	if req.TopP != 0 {
		cfg.TopP = genai.Ptr(req.TopP)
	}
	if req.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(req.MaxTokens)
	}
	if len(req.Stop) > 0 {
		cfg.StopSequences = req.Stop
	}
	if req.FrequencyPenalty != nil {
		cfg.FrequencyPenalty = req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		cfg.PresencePenalty = req.PresencePenalty
	}
	if req.N != nil && *req.N > 0 {
		cfg.CandidateCount = int32(*req.N)
	}
	if req.LogProbs != nil {
		cfg.ResponseLogprobs = *req.LogProbs
	}
	if req.TopLogProbs != nil {
		v := int32(*req.TopLogProbs)
		cfg.Logprobs = &v
	}

	if req.ResponseFormat != nil {
		switch req.ResponseFormat.Type {
		case llm.ResponseFormatJSONObject:
			cfg.ResponseMIMEType = "application/json"
		case llm.ResponseFormatJSONSchema:
			cfg.ResponseMIMEType = "application/json"
			if req.ResponseFormat.JSONSchema != nil {
				cfg.ResponseJsonSchema = req.ResponseFormat.JSONSchema.Schema
			}
		}
	}

	applyGeminiThinkingConfig(cfg, req)

	if len(safetySettings) > 0 {
		cfg.SafetySettings = make([]*genai.SafetySetting, 0, len(safetySettings))
		for _, s := range safetySettings {
			cfg.SafetySettings = append(cfg.SafetySettings, &genai.SafetySetting{
				Category:  genai.HarmCategory(strings.TrimSpace(s.Category)),
				Threshold: genai.HarmBlockThreshold(strings.TrimSpace(s.Threshold)),
			})
		}
	}

	cfg.Tools = convertToGenAITools(req.Tools, req.WebSearchOptions)
	cfg.ToolConfig = convertToolChoiceToGenAI(req.ToolChoice, req.IncludeServerSideToolInvocations)
	cfg.CachedContent = strings.TrimSpace(req.CachedContent)

	if len(req.Modalities) > 0 {
		cfg.ResponseModalities = make([]string, 0, len(req.Modalities))
		for _, modality := range req.Modalities {
			modality = strings.ToUpper(strings.TrimSpace(modality))
			if modality != "" {
				cfg.ResponseModalities = append(cfg.ResponseModalities, modality)
			}
		}
	}

	if mr := strings.TrimSpace(req.MediaResolution); mr != "" {
		cfg.MediaResolution = genai.MediaResolution(strings.ToUpper(mr))
	}

	if isEmptyGenAIConfig(cfg) {
		return nil
	}
	return cfg
}

func applyGeminiThinkingConfig(cfg *genai.GenerateContentConfig, req *llm.ChatRequest) {
	if cfg == nil || req == nil {
		return
	}

	// Formal main-face fields take priority over legacy ReasoningMode.
	hasFormalThinking := strings.TrimSpace(req.ThinkingLevel) != "" ||
		req.ThinkingBudget != nil ||
		req.IncludeThoughts != nil
	hasLegacyMode := strings.TrimSpace(req.ReasoningMode) != ""

	if !hasFormalThinking && !hasLegacyMode {
		return
	}

	cfg.ThinkingConfig = &genai.ThinkingConfig{}
	model := strings.TrimSpace(strings.ToLower(req.Model))
	if model == "" {
		model = defaultModel
	}

	// If formal fields are set, use them directly.
	if hasFormalThinking {
		applyFormalThinkingFields(cfg.ThinkingConfig, req)
		return
	}

	// Legacy fallback: derive from ReasoningMode.
	mode := strings.TrimSpace(strings.ToLower(req.ReasoningMode))

	if strings.Contains(model, "gemini-2.5") {
		applyGemini25ThinkingConfig(cfg.ThinkingConfig, mode, model)
		return
	}

	cfg.ThinkingConfig.IncludeThoughts = true
	switch mode {
	case "minimal":
		cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevelMinimal
	case "low":
		cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevelLow
	case "medium":
		cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevelMedium
	case "high":
		cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevelHigh
	default:
		cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevelMedium
	}
}

// applyFormalThinkingFields maps formal main-face thinking fields to genai.ThinkingConfig.
func applyFormalThinkingFields(cfg *genai.ThinkingConfig, req *llm.ChatRequest) {
	if cfg == nil {
		return
	}

	// IncludeThoughts: explicit bool wins; default true when thinking is requested.
	if req.IncludeThoughts != nil {
		cfg.IncludeThoughts = *req.IncludeThoughts
	} else {
		cfg.IncludeThoughts = true
	}

	// ThinkingBudget: pass through directly (Gemini 2.5 style).
	if req.ThinkingBudget != nil {
		budget := *req.ThinkingBudget
		cfg.ThinkingBudget = &budget
	}

	// ThinkingLevel: map string to genai enum (Gemini 3.x style).
	if level := strings.TrimSpace(strings.ToLower(req.ThinkingLevel)); level != "" {
		switch level {
		case "minimal":
			cfg.ThinkingLevel = genai.ThinkingLevelMinimal
		case "low":
			cfg.ThinkingLevel = genai.ThinkingLevelLow
		case "medium":
			cfg.ThinkingLevel = genai.ThinkingLevelMedium
		case "high":
			cfg.ThinkingLevel = genai.ThinkingLevelHigh
		default:
			cfg.ThinkingLevel = genai.ThinkingLevel(strings.ToUpper(level))
		}
	}
}

func applyGemini25ThinkingConfig(cfg *genai.ThinkingConfig, mode, model string) {
	if cfg == nil {
		return
	}

	cfg.IncludeThoughts = mode != "disabled"

	switch mode {
	case "disabled":
		if strings.Contains(model, "flash") || strings.Contains(model, "lite") {
			budget := int32(0)
			cfg.ThinkingBudget = &budget
		}
	case "minimal":
		if strings.Contains(model, "flash") || strings.Contains(model, "lite") {
			budget := int32(0)
			cfg.ThinkingBudget = &budget
			cfg.IncludeThoughts = false
			return
		}
		budget := int32(-1)
		cfg.ThinkingBudget = &budget
		cfg.IncludeThoughts = false
	default:
		budget := int32(-1)
		cfg.ThinkingBudget = &budget
		cfg.IncludeThoughts = true
	}
}

func isEmptyGenAIConfig(cfg *genai.GenerateContentConfig) bool {
	if cfg == nil {
		return true
	}
	return cfg.SystemInstruction == nil &&
		cfg.Temperature == nil &&
		cfg.TopP == nil &&
		cfg.CandidateCount == 0 &&
		cfg.MaxOutputTokens == 0 &&
		len(cfg.StopSequences) == 0 &&
		!cfg.ResponseLogprobs &&
		cfg.Logprobs == nil &&
		cfg.PresencePenalty == nil &&
		cfg.FrequencyPenalty == nil &&
		cfg.ResponseMIMEType == "" &&
		cfg.ResponseSchema == nil &&
		cfg.ResponseJsonSchema == nil &&
		cfg.RoutingConfig == nil &&
		cfg.ModelSelectionConfig == nil &&
		len(cfg.SafetySettings) == 0 &&
		len(cfg.Tools) == 0 &&
		cfg.ToolConfig == nil &&
		cfg.CachedContent == "" &&
		len(cfg.ResponseModalities) == 0 &&
		cfg.SpeechConfig == nil &&
		!cfg.AudioTimestamp &&
		cfg.ThinkingConfig == nil &&
		cfg.ImageConfig == nil &&
		cfg.EnableEnhancedCivicAnswers == nil &&
		cfg.ModelArmorConfig == nil &&
		cfg.ServiceTier == ""
}

func decodeThoughtSignature(value string) []byte {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded
	}
	return []byte(value)
}

func (p *GeminiProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	// 统一入口：应用改写器链
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultModel)
	systemInstruction, contents := convertToGenAIContents(req.Messages)
	config := buildGenAIGenerationConfig(req, p.cfg.SafetySettings)
	if config == nil {
		config = &genai.GenerateContentConfig{}
	}
	config.SystemInstruction = systemInstruction

	resp, err := client.Models.GenerateContent(ctx, model, contents, config)
	if err != nil {
		return nil, p.mapSDKError(err)
	}

	if err := checkPromptFeedbackFromGenAI(resp, p.Name()); err != nil {
		return nil, err
	}

	return toChatResponseFromGenAI(resp, p.Name(), model), nil
}

func (p *GeminiProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	// 对齐 Google streamGenerateContent：SSE data 行含 JSON，可选 [DONE] 或 error 负载。
	// 文档：https://ai.google.dev/gemini-api/docs/text-generation（Streaming）
	rewrittenReq, err := p.rewriterChain.Execute(ctx, req)
	if err != nil {
		return nil, &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("request rewrite failed: %v", err),
			HTTPStatus: http.StatusBadRequest,
			Provider:   p.Name(),
		}
	}
	req = rewrittenReq

	client, err := p.sdkClient(ctx)
	if err != nil {
		return nil, p.mapSDKError(err)
	}
	model := providerbase.ChooseModel(req, p.cfg.Model, defaultModel)
	systemInstruction, contents := convertToGenAIContents(req.Messages)
	config := buildGenAIGenerationConfig(req, p.cfg.SafetySettings)
	if config == nil {
		config = &genai.GenerateContentConfig{}
	}
	config.SystemInstruction = systemInstruction

	ch := make(chan llm.StreamChunk)
	go func() {
		defer close(ch)
		for result, err := range client.Models.GenerateContentStream(ctx, model, contents, config) {
			if err != nil {
				if isBenignGenAIStreamDone(err) {
					return
				}
				mapped := p.mapSDKError(err)
				if te, ok := mapped.(*types.Error); ok {
					select {
					case <-ctx.Done():
						return
					case ch <- llm.StreamChunk{Err: te}:
					}
				}
				return
			}
			if result == nil {
				continue
			}

			if err := checkPromptFeedbackFromGenAI(result, p.Name()); err != nil {
				select {
				case <-ctx.Done():
					return
				case ch <- llm.StreamChunk{Err: err.(*types.Error)}:
				}
				return
			}

			for _, chunk := range streamChunksFromGenAI(result, p.Name(), model) {
				select {
				case <-ctx.Done():
					return
				case ch <- chunk:
				}
			}
		}
	}()

	return ch, nil
}

func isBenignGenAIStreamDone(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "iterateResponseStream") && strings.Contains(msg, "[DONE]")
}

func toChatResponseFromGenAI(gr *genai.GenerateContentResponse, provider, model string) *llm.ChatResponse {
	if gr == nil {
		return &llm.ChatResponse{
			Provider: provider,
			Model:    model,
		}
	}

	choices := make([]llm.ChatChoice, 0, len(gr.Candidates))
	for _, candidate := range gr.Candidates {
		if candidate == nil {
			continue
		}
		msg := messageFromGenAICandidate(gr.ResponseID, candidate, provider)
		choices = append(choices, llm.ChatChoice{
			Index:        int(candidate.Index),
			FinishReason: normalizeFinishReason(string(candidate.FinishReason)),
			Message:      msg,
		})
	}

	resp := &llm.ChatResponse{
		ID:       gr.ResponseID,
		Provider: provider,
		Model:    model,
		Choices:  choices,
	}
	if gr.UsageMetadata != nil {
		resp.Usage = *convertUsageMetadataFromGenAI(gr.UsageMetadata)
	}
	return resp
}

func streamChunksFromGenAI(gr *genai.GenerateContentResponse, provider, model string) []llm.StreamChunk {
	if gr == nil {
		return nil
	}

	chunks := make([]llm.StreamChunk, 0, len(gr.Candidates)+1)
	for _, candidate := range gr.Candidates {
		if candidate == nil {
			continue
		}
		chunks = append(chunks, llm.StreamChunk{
			ID:           gr.ResponseID,
			Provider:     provider,
			Model:        model,
			Index:        int(candidate.Index),
			FinishReason: normalizeFinishReason(string(candidate.FinishReason)),
			Delta:        messageFromGenAICandidate(gr.ResponseID, candidate, provider),
		})
	}
	if gr.UsageMetadata != nil {
		chunks = append(chunks, llm.StreamChunk{
			Provider: provider,
			Model:    model,
			Usage:    convertUsageMetadataFromGenAI(gr.UsageMetadata),
		})
	}
	return chunks
}

func messageFromGenAICandidate(responseID string, candidate *genai.Candidate, provider string) types.Message {
	msg := types.Message{Role: llm.RoleAssistant}
	if candidate == nil || candidate.Content == nil {
		return msg
	}

	toolCallIndex := 0
	for partIndex, part := range candidate.Content.Parts {
		if part == nil {
			continue
		}
		if part.Thought {
			appendGenAIThoughtPart(&msg, part, partIndex, provider)
			continue
		}
		if len(part.ThoughtSignature) > 0 {
			msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{
				Provider:  provider,
				Kind:      "thought_signature",
				State:     encodeThoughtSignature(part.ThoughtSignature),
				PartIndex: partIndex,
			})
		}
		if part.Text != "" {
			msg.Content += part.Text
		}
		if part.FunctionCall != nil {
			argsJSON, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				continue
			}
			toolCallID := strings.TrimSpace(part.FunctionCall.ID)
			if toolCallID == "" {
				toolCallID = fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, toolCallIndex)
				if responseID != "" {
					toolCallID = fmt.Sprintf("call_%s_%s_%d", responseID, part.FunctionCall.Name, toolCallIndex)
				}
			}
			msg.ToolCalls = append(msg.ToolCalls, providerbase.NewFunctionToolCall(toolCallID, part.FunctionCall.Name, argsJSON))
			toolCallIndex++
		}
	}

	if candidate.GroundingMetadata != nil {
		msg.Annotations = append(msg.Annotations, extractGroundingAnnotationsFromGenAI(candidate.GroundingMetadata)...)
	}
	return msg
}

func appendGenAIThoughtPart(msg *types.Message, part *genai.Part, partIndex int, provider string) {
	if msg == nil || part == nil {
		return
	}
	if strings.TrimSpace(part.Text) != "" {
		if msg.ReasoningContent == nil || strings.TrimSpace(*msg.ReasoningContent) == "" {
			msg.ReasoningContent = strPtr(part.Text)
		} else {
			joined := strings.TrimSpace(*msg.ReasoningContent + "\n\n" + part.Text)
			msg.ReasoningContent = strPtr(joined)
		}
		msg.ReasoningSummaries = append(msg.ReasoningSummaries, types.ReasoningSummary{
			Provider: provider,
			Kind:     "thought_summary",
			Text:     part.Text,
			ID:       fmt.Sprintf("part_%d", partIndex),
		})
	}
	if len(part.ThoughtSignature) > 0 {
		msg.OpaqueReasoning = append(msg.OpaqueReasoning, types.OpaqueReasoning{
			Provider:  provider,
			Kind:      "thought_signature",
			State:     encodeThoughtSignature(part.ThoughtSignature),
			PartIndex: partIndex,
			ID:        fmt.Sprintf("part_%d", partIndex),
		})
	}
}

func encodeThoughtSignature(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(data)
}

func convertUsageMetadataFromGenAI(m *genai.GenerateContentResponseUsageMetadata) *llm.ChatUsage {
	if m == nil {
		return nil
	}
	usage := &llm.ChatUsage{
		PromptTokens:     int(m.PromptTokenCount),
		CompletionTokens: int(m.CandidatesTokenCount),
		TotalTokens:      int(m.TotalTokenCount),
	}
	if m.ThoughtsTokenCount > 0 {
		usage.CompletionTokensDetails = &llm.CompletionTokensDetails{
			ReasoningTokens: int(m.ThoughtsTokenCount),
		}
	}
	if m.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails = &llm.PromptTokensDetails{
			CachedTokens: int(m.CachedContentTokenCount),
		}
	}
	return usage
}

func checkPromptFeedbackFromGenAI(resp *genai.GenerateContentResponse, provider string) error {
	if resp == nil || resp.PromptFeedback == nil || resp.PromptFeedback.BlockReason == "" {
		return nil
	}
	msg := fmt.Sprintf("request blocked by safety filter: %s", resp.PromptFeedback.BlockReason)
	if resp.PromptFeedback.BlockReasonMessage != "" {
		msg = fmt.Sprintf("%s — %s", msg, resp.PromptFeedback.BlockReasonMessage)
	}
	return &types.Error{
		Code:       llm.ErrContentFiltered,
		Message:    msg,
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

func extractGroundingAnnotationsFromGenAI(gm *genai.GroundingMetadata) []types.Annotation {
	if gm == nil {
		return nil
	}

	var annotations []types.Annotation
	if len(gm.GroundingSupports) > 0 {
		for _, support := range gm.GroundingSupports {
			if support == nil {
				continue
			}
			for _, idx := range support.GroundingChunkIndices {
				if idx < 0 || int(idx) >= len(gm.GroundingChunks) {
					continue
				}
				chunk := gm.GroundingChunks[int(idx)]
				if chunk == nil || chunk.Web == nil {
					continue
				}
				ann := types.Annotation{
					Type:  "url_citation",
					URL:   chunk.Web.URI,
					Title: chunk.Web.Title,
				}
				if support.Segment != nil {
					ann.StartIndex = int(support.Segment.StartIndex)
					ann.EndIndex = int(support.Segment.EndIndex)
				}
				annotations = append(annotations, ann)
			}
		}
		return annotations
	}

	for _, chunk := range gm.GroundingChunks {
		if chunk == nil || chunk.Web == nil {
			continue
		}
		annotations = append(annotations, types.Annotation{
			Type:  "url_citation",
			URL:   chunk.Web.URI,
			Title: chunk.Web.Title,
		})
	}
	return annotations
}

// =============================================================================
// Helper functions
// =============================================================================

const defaultModel = "gemini-2.5-pro"

// normalizeFinishReason maps Gemini finish reasons to OpenAI-compatible values.
func normalizeFinishReason(reason string) string {
	switch reason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII":
		return "content_filter"
	case "LANGUAGE":
		return "content_filter"
	case "":
		return ""
	default:
		return strings.ToLower(reason)
	}
}

func geminiThoughtSignatureByIndex(msg types.Message, index int) string {
	for _, opaque := range msg.OpaqueReasoning {
		provider := strings.TrimSpace(opaque.Provider)
		if provider != "" && provider != "gemini" {
			continue
		}
		if strings.TrimSpace(opaque.Kind) != "thought_signature" {
			continue
		}
		if opaque.PartIndex == index {
			return strings.TrimSpace(opaque.State)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func strPtr(s string) *string {
	return &s
}

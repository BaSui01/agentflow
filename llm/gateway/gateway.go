package gateway

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/capabilities"
	speech "github.com/BaSui01/agentflow/llm/capabilities/audio"
	"github.com/BaSui01/agentflow/llm/capabilities/avatar"
	"github.com/BaSui01/agentflow/llm/capabilities/embedding"
	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/moderation"
	"github.com/BaSui01/agentflow/llm/capabilities/music"
	"github.com/BaSui01/agentflow/llm/capabilities/rerank"
	"github.com/BaSui01/agentflow/llm/capabilities/threed"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	llmpolicy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llmtokenizer "github.com/BaSui01/agentflow/llm/tokenizer"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

// Config 定义 gateway 运行依赖。
type Config struct {
	ChatProvider      llm.Provider
	Capabilities      *capabilities.Entry
	CostCalculator    *observability.CostCalculator
	Ledger            observability.Ledger
	PolicyManager     *llmpolicy.Manager
	TokenizerResolver func(model string) llmtokenizer.Tokenizer
	Logger            *zap.Logger
}

// ToolsInput 是 tools 能力统一 payload。
type ToolsInput struct {
	Calls []types.ToolCall
}

// ImageInput 是 image 能力统一 payload。
type ImageInput struct {
	Provider string
	Generate *image.GenerateRequest
	Edit     *image.EditRequest
}

// VideoInput 是 video 能力统一 payload。
type VideoInput struct {
	Provider string
	Generate *video.GenerateRequest
}

// AudioInput 是 audio 能力统一 payload（TTS/STT）。
type AudioInput struct {
	Provider   string
	Synthesize *speech.TTSRequest
	Transcribe *speech.STTRequest
}

// EmbeddingInput 是 embedding 能力统一 payload。
type EmbeddingInput struct {
	Provider string
	Request  *embedding.EmbeddingRequest
}

// RerankInput 是 rerank 能力统一 payload。
type RerankInput struct {
	Provider string
	Request  *rerank.RerankRequest
}

// ModerationInput 是 moderation 能力统一 payload。
type ModerationInput struct {
	Provider string
	Request  *moderation.ModerationRequest
}

// MusicInput 是 music 能力统一 payload。
type MusicInput struct {
	Provider string
	Generate *music.GenerateRequest
}

// ThreeDInput 是 threed 能力统一 payload。
type ThreeDInput struct {
	Provider string
	Generate *threed.GenerateRequest
}

// AvatarInput 是 avatar 能力统一 payload。
type AvatarInput struct {
	Provider string
	Generate *avatar.GenerateRequest
}

// Service 是统一入口 gateway 实现。
type Service struct {
	chatProvider      llm.Provider
	capabilities      *capabilities.Entry
	costCalculator    *observability.CostCalculator
	ledger            observability.Ledger
	policyManager     *llmpolicy.Manager
	tokenizerResolver func(model string) llmtokenizer.Tokenizer
	logger            *zap.Logger
}

var _ llmcore.Gateway = (*Service)(nil)

// New 创建 gateway。
func New(cfg Config) *Service {
	logger := cfg.Logger
	if logger == nil {
		panic("llm.Gateway: logger is required and cannot be nil")
	}
	calc := cfg.CostCalculator
	if calc == nil {
		calc = observability.NewCostCalculator()
	}
	ledger := cfg.Ledger
	if ledger == nil {
		ledger = observability.NewNoopLedger()
	}
	resolver := cfg.TokenizerResolver
	if resolver == nil {
		resolver = llmtokenizer.GetTokenizerOrEstimator
	}
	return &Service{
		chatProvider:      cfg.ChatProvider,
		capabilities:      cfg.Capabilities,
		costCalculator:    calc,
		ledger:            ledger,
		policyManager:     cfg.PolicyManager,
		tokenizerResolver: resolver,
		logger:            logger,
	}
}

// Invoke 执行统一同步调用。
func (s *Service) Invoke(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	normalizeRequest(req)
	if err := s.preflightPolicy(ctx, req); err != nil {
		return nil, err
	}

	var (
		resp *llmcore.UnifiedResponse
		err  error
	)
	switch req.Capability {
	case llmcore.CapabilityChat:
		resp, err = s.invokeChat(ctx, req)
	case llmcore.CapabilityTools:
		resp, err = s.invokeTools(ctx, req)
	case llmcore.CapabilityImage:
		resp, err = s.invokeImage(ctx, req)
	case llmcore.CapabilityVideo:
		resp, err = s.invokeVideo(ctx, req)
	case llmcore.CapabilityAudio:
		resp, err = s.invokeAudio(ctx, req)
	case llmcore.CapabilityEmbedding:
		resp, err = s.invokeEmbedding(ctx, req)
	case llmcore.CapabilityRerank:
		resp, err = s.invokeRerank(ctx, req)
	case llmcore.CapabilityModeration:
		resp, err = s.invokeModeration(ctx, req)
	case llmcore.CapabilityMusic:
		resp, err = s.invokeMusic(ctx, req)
	case llmcore.CapabilityThreeD:
		resp, err = s.invokeThreeD(ctx, req)
	case llmcore.CapabilityAvatar:
		resp, err = s.invokeAvatar(ctx, req)
	default:
		return nil, llmcore.InvalidCapabilityError(req.Capability)
	}
	if err != nil {
		return nil, err
	}
	resp.Usage = normalizeUsage(resp.Usage)
	resp.Cost = s.normalizeCost(resp.ProviderDecision, resp.Usage, resp.Cost)
	s.recordResponseUsage(req, resp)
	s.recordLedger(
		ctx,
		req,
		firstNonEmpty(resp.TraceID, req.TraceID),
		resp.ProviderDecision,
		resp.Usage,
		resp.Cost,
	)
	return resp, nil
}

// Stream 执行统一流式调用。
func (s *Service) Stream(ctx context.Context, req *llmcore.UnifiedRequest) (<-chan llmcore.UnifiedChunk, error) {
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	normalizeRequest(req)
	if err := s.preflightPolicy(ctx, req); err != nil {
		return nil, err
	}

	if req.Capability != llmcore.CapabilityChat {
		return nil, llmcore.InvalidCapabilityError(req.Capability)
	}
	if s.chatProvider == nil {
		return nil, llmcore.GatewayUnavailableError("chat provider is not configured")
	}

	chatReq, ok := req.Payload.(*llm.ChatRequest)
	if !ok || chatReq == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityChat, "*llm.ChatRequest")
	}
	mergeChatRoutingMetadata(req, chatReq)

	source, err := s.chatProvider.Stream(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	out := make(chan llmcore.UnifiedChunk)
	go func(ctx context.Context) {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("stream relay panic recovered", zap.Any("panic", r))
			}
			close(out)
		}()
		traceID := firstNonEmpty(req.TraceID, chatReq.TraceID)
		var (
			finalUsage    *llmcore.Usage
			finalCost     *llmcore.Cost
			finalDecision llmcore.ProviderDecision
		)

		for chunk := range source {
			decision := llmcore.ProviderDecision{
				Provider: firstNonEmpty(chunk.Provider, s.chatProvider.Name()),
				Model:    firstNonEmpty(chunk.Model, chatReq.Model, req.ModelHint),
				Strategy: string(req.RoutePolicy),
			}

			if chunk.Err != nil {
				select {
				case out <- llmcore.UnifiedChunk{
					Err:              chunk.Err,
					TraceID:          traceID,
					ProviderDecision: decision,
				}:
				case <-ctx.Done():
					return
				}
				continue
			}

			copied := chunk
			var usage *llmcore.Usage
			var cost *llmcore.Cost
			if chunk.Usage != nil {
				u := fromChatUsage(*chunk.Usage)
				u = normalizeUsage(u)
				usage = &u
				c := s.normalizeCost(decision, u, llmcore.Cost{})
				cost = &c

				uCopy := u
				cCopy := c
				finalUsage = &uCopy
				finalCost = &cCopy
				finalDecision = decision
			}

			select {
			case out <- llmcore.UnifiedChunk{
				Output:           &copied,
				Usage:            usage,
				Cost:             cost,
				TraceID:          traceID,
				ProviderDecision: decision,
			}:
			case <-ctx.Done():
				return
			}
		}

		if finalUsage != nil && finalCost != nil {
			s.policyManager.RecordUsage(llmpolicy.UsageRecord{
				Timestamp: time.Now(),
				Tokens:    finalUsage.TotalTokens,
				Cost:      costAmount(finalCost),
				Model:     finalDecision.Model,
				RequestID: traceID,
				UserID:    metadataValue(req, "user_id"),
				AgentID:   metadataValue(req, "agent_id"),
			})
			s.recordLedger(ctx, req, traceID, finalDecision, *finalUsage, *finalCost)
		}
	}(ctx)

	return out, nil
}

func (s *Service) invokeChat(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.chatProvider == nil {
		return nil, llmcore.GatewayUnavailableError("chat provider is not configured")
	}

	chatReq, ok := req.Payload.(*llm.ChatRequest)
	if !ok || chatReq == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityChat, "*llm.ChatRequest")
	}
	mergeChatRoutingMetadata(req, chatReq)

	resp, err := s.chatProvider.Completion(ctx, chatReq)
	if err != nil {
		return nil, err
	}

	usage := fromChatUsage(resp.Usage)
	provider := firstNonEmpty(resp.Provider, s.chatProvider.Name())
	model := firstNonEmpty(resp.Model, chatReq.Model, req.ModelHint)

	return &llmcore.UnifiedResponse{
		Output:  resp,
		Usage:   usage,
		Cost:    llmcore.Cost{},
		TraceID: firstNonEmpty(req.TraceID, chatReq.TraceID),
		ProviderDecision: llmcore.ProviderDecision{
			Provider: provider,
			Model:    model,
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeTools(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*ToolsInput)
	if !ok || input == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityTools, "*gateway.ToolsInput")
	}

	results, err := s.capabilities.ExecuteTools(ctx, input.Calls)
	if err != nil {
		return nil, err
	}

	outputUnits := len(results)
	return &llmcore.UnifiedResponse{
		Output: results,
		Usage: llmcore.Usage{
			InputUnits:  len(input.Calls),
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: 0,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: "tools",
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeImage(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*ImageInput)
	if !ok || input == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityImage, "*gateway.ImageInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	var (
		resp *image.GenerateResponse
		err  error
	)

	if input.Edit != nil {
		provider, providerErr := s.capabilities.Image(providerName)
		if providerErr != nil {
			return nil, types.WrapError(providerErr, types.ErrInvalidRequest, providerErr.Error())
		}
		resp, err = provider.Edit(ctx, input.Edit)
	} else if input.Generate != nil {
		resp, err = s.capabilities.GenerateImage(ctx, input.Generate, providerName)
	} else {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityImage, "ImageInput with Generate or Edit")
	}
	if err != nil {
		return nil, err
	}

	outputUnits := resp.Usage.ImagesGenerated
	if outputUnits == 0 {
		outputUnits = len(resp.Images)
	}

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  1,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: resp.Usage.Cost,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeVideo(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*VideoInput)
	if !ok || input == nil || input.Generate == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityVideo, "*gateway.VideoInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.GenerateVideo(ctx, input.Generate, providerName)
	if err != nil {
		return nil, err
	}

	outputUnits := resp.Usage.VideosGenerated
	if outputUnits == 0 {
		outputUnits = len(resp.Videos)
	}

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  1,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: resp.Usage.Cost,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeAudio(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*AudioInput)
	if !ok || input == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityAudio, "*gateway.AudioInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	if input.Synthesize != nil {
		resp, err := s.capabilities.Synthesize(ctx, input.Synthesize, providerName)
		if err != nil {
			return nil, err
		}
		return &llmcore.UnifiedResponse{
			Output: resp,
			Usage: llmcore.Usage{
				InputUnits:  1,
				OutputUnits: 1,
				TotalUnits:  1,
			},
			Cost: llmcore.Cost{
				AmountUSD: 0,
				Currency:  "USD",
			},
			TraceID: req.TraceID,
			ProviderDecision: llmcore.ProviderDecision{
				Provider: firstNonEmpty(resp.Provider, providerName),
				Model:    firstNonEmpty(resp.Model, req.ModelHint),
				Strategy: string(req.RoutePolicy),
			},
		}, nil
	}

	if input.Transcribe != nil {
		resp, err := s.capabilities.Transcribe(ctx, input.Transcribe, providerName)
		if err != nil {
			return nil, err
		}
		return &llmcore.UnifiedResponse{
			Output: resp,
			Usage: llmcore.Usage{
				InputUnits:  1,
				OutputUnits: 1,
				TotalUnits:  1,
			},
			Cost: llmcore.Cost{
				AmountUSD: 0,
				Currency:  "USD",
			},
			TraceID: req.TraceID,
			ProviderDecision: llmcore.ProviderDecision{
				Provider: firstNonEmpty(resp.Provider, providerName),
				Model:    firstNonEmpty(resp.Model, req.ModelHint),
				Strategy: string(req.RoutePolicy),
			},
		}, nil
	}

	return nil, llmcore.InvalidPayloadError(llmcore.CapabilityAudio, "AudioInput with Synthesize or Transcribe")
}

func (s *Service) invokeEmbedding(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*EmbeddingInput)
	if !ok || input == nil || input.Request == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityEmbedding, "*gateway.EmbeddingInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.Embed(ctx, input.Request, providerName)
	if err != nil {
		return nil, err
	}

	totalTokens := resp.Usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = resp.Usage.PromptTokens
	}
	outputUnits := len(resp.Embeddings)

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			PromptTokens: resp.Usage.PromptTokens,
			TotalTokens:  totalTokens,
			InputUnits:   len(input.Request.Input),
			OutputUnits:  outputUnits,
			TotalUnits:   outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: resp.Usage.Cost,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeRerank(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*RerankInput)
	if !ok || input == nil || input.Request == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityRerank, "*gateway.RerankInput")
	}

	providerName := strings.TrimSpace(input.Provider)
	if providerName == "" {
		providerName = s.capabilities.ResolveRerankProvider(req.Hints.ChatProvider)
	}
	if providerName == "" {
		providerName = s.capabilities.ResolveRerankProvider(metadataValue(req, llmcore.MetadataKeyChatProvider))
	}
	if providerName == "" {
		providerName = req.ProviderHint
	}
	resp, err := s.capabilities.RerankDocs(ctx, input.Request, providerName)
	if err != nil {
		return nil, err
	}

	outputUnits := len(resp.Results)
	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			TotalTokens: resp.Usage.TotalTokens,
			InputUnits:  len(input.Request.Documents),
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: resp.Usage.Cost,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeModeration(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*ModerationInput)
	if !ok || input == nil || input.Request == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityModeration, "*gateway.ModerationInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.Moderate(ctx, input.Request, providerName)
	if err != nil {
		return nil, err
	}

	inputUnits := len(input.Request.Input) + len(input.Request.Images)
	outputUnits := len(resp.Results)

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  inputUnits,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: 0,
			Currency:  "USD",
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeMusic(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*MusicInput)
	if !ok || input == nil || input.Generate == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityMusic, "*gateway.MusicInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.GenerateMusic(ctx, input.Generate, providerName)
	if err != nil {
		return nil, err
	}

	outputUnits := resp.Usage.TracksGenerated
	if outputUnits == 0 {
		outputUnits = len(resp.Tracks)
	}

	currency := "USD"
	amount := 0.0
	if resp.Usage.Credits > 0 {
		currency = "CREDITS"
		amount = resp.Usage.Credits
	}

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  1,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: amount,
			Currency:  currency,
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeThreeD(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*ThreeDInput)
	if !ok || input == nil || input.Generate == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityThreeD, "*gateway.ThreeDInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.Generate3D(ctx, input.Generate, providerName)
	if err != nil {
		return nil, err
	}

	outputUnits := resp.Usage.ModelsGenerated
	if outputUnits == 0 {
		outputUnits = len(resp.Models)
	}

	currency := "USD"
	amount := 0.0
	if resp.Usage.Credits > 0 {
		currency = "CREDITS"
		amount = resp.Usage.Credits
	}

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  1,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: amount,
			Currency:  currency,
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func (s *Service) invokeAvatar(ctx context.Context, req *llmcore.UnifiedRequest) (*llmcore.UnifiedResponse, error) {
	if s.capabilities == nil {
		return nil, llmcore.GatewayUnavailableError("capabilities entry is not configured")
	}

	input, ok := req.Payload.(*AvatarInput)
	if !ok || input == nil || input.Generate == nil {
		return nil, llmcore.InvalidPayloadError(llmcore.CapabilityAvatar, "*gateway.AvatarInput")
	}

	providerName := firstNonEmpty(strings.TrimSpace(input.Provider), req.ProviderHint)
	resp, err := s.capabilities.GenerateAvatar(ctx, input.Generate, providerName)
	if err != nil {
		return nil, err
	}

	outputUnits := resp.Usage.AvatarsGenerated
	if outputUnits == 0 {
		outputUnits = len(resp.Assets)
	}

	currency := "USD"
	amount := 0.0
	if resp.Usage.Credits > 0 {
		currency = "CREDITS"
		amount = resp.Usage.Credits
	}

	return &llmcore.UnifiedResponse{
		Output: resp,
		Usage: llmcore.Usage{
			InputUnits:  1,
			OutputUnits: outputUnits,
			TotalUnits:  outputUnits,
		},
		Cost: llmcore.Cost{
			AmountUSD: amount,
			Currency:  currency,
		},
		TraceID: req.TraceID,
		ProviderDecision: llmcore.ProviderDecision{
			Provider: firstNonEmpty(resp.Provider, providerName),
			Model:    firstNonEmpty(resp.Model, req.ModelHint),
			Strategy: string(req.RoutePolicy),
		},
	}, nil
}

func fromChatUsage(u llm.ChatUsage) llmcore.Usage {
	return llmcore.Usage{
		PromptTokens:     u.PromptTokens,
		CompletionTokens: u.CompletionTokens,
		TotalTokens:      u.TotalTokens,
	}
}

func normalizeUsage(usage llmcore.Usage) llmcore.Usage {
	if usage.PromptTokens < 0 {
		usage.PromptTokens = 0
	}
	if usage.CompletionTokens < 0 {
		usage.CompletionTokens = 0
	}
	if usage.TotalTokens < 0 {
		usage.TotalTokens = 0
	}
	if usage.InputUnits < 0 {
		usage.InputUnits = 0
	}
	if usage.OutputUnits < 0 {
		usage.OutputUnits = 0
	}
	if usage.TotalUnits < 0 {
		usage.TotalUnits = 0
	}

	if usage.TotalTokens == 0 && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if usage.TotalUnits == 0 {
		switch {
		case usage.OutputUnits > 0:
			usage.TotalUnits = usage.OutputUnits
		case usage.InputUnits > 0:
			usage.TotalUnits = usage.InputUnits
		}
	}
	if usage.OutputUnits == 0 && usage.TotalUnits > 0 && usage.InputUnits == 0 {
		usage.OutputUnits = usage.TotalUnits
	}
	return usage
}

func (s *Service) normalizeCost(decision llmcore.ProviderDecision, usage llmcore.Usage, cost llmcore.Cost) llmcore.Cost {
	if cost.AmountUSD < 0 {
		cost.AmountUSD = 0
	}

	currency := strings.ToUpper(strings.TrimSpace(cost.Currency))
	if currency == "" {
		currency = "USD"
	}

	if currency == "USD" && cost.AmountUSD == 0 && (usage.PromptTokens > 0 || usage.CompletionTokens > 0) {
		cost.AmountUSD = s.costCalculator.Calculate(
			decision.Provider,
			decision.Model,
			usage.PromptTokens,
			usage.CompletionTokens,
		)
		if cost.AmountUSD < 0 {
			cost.AmountUSD = 0
		}
	}

	cost.Currency = currency
	return cost
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *Service) preflightPolicy(ctx context.Context, req *llmcore.UnifiedRequest) error {
	if s == nil || s.policyManager == nil {
		return nil
	}

	estimatedTokens := parseInt(metadataValue(req, "estimated_tokens"))
	if estimatedTokens == 0 {
		estimatedTokens = s.estimateRequestTokens(req)
		if estimatedTokens > 0 {
			ensureMetadata(req)["estimated_tokens"] = strconv.Itoa(estimatedTokens)
		}
	}
	estimatedCost := parseFloat(metadataValue(req, "estimated_cost_usd"))
	if estimatedCost == 0 {
		estimatedCost = parseFloat(metadataValue(req, "estimated_cost"))
	}
	return s.policyManager.PreCheck(ctx, estimatedTokens, estimatedCost)
}

func (s *Service) estimateRequestTokens(req *llmcore.UnifiedRequest) int {
	if s == nil || req == nil || req.Payload == nil {
		return 0
	}

	switch req.Capability {
	case llmcore.CapabilityChat:
		chatReq, ok := req.Payload.(*llm.ChatRequest)
		if !ok || chatReq == nil {
			return 0
		}
		return s.estimateChatTokens(req, chatReq)
	case llmcore.CapabilityEmbedding:
		input, ok := req.Payload.(*EmbeddingInput)
		if !ok || input == nil || input.Request == nil {
			return 0
		}
		model := firstNonEmpty(input.Request.Model, req.ModelHint)
		return s.countTextsTokens(model, input.Request.Input)
	case llmcore.CapabilityRerank:
		input, ok := req.Payload.(*RerankInput)
		if !ok || input == nil || input.Request == nil {
			return 0
		}
		model := firstNonEmpty(input.Request.Model, req.ModelHint)
		texts := make([]string, 0, len(input.Request.Documents)+1)
		texts = append(texts, input.Request.Query)
		for _, doc := range input.Request.Documents {
			texts = append(texts, doc.Title, doc.Text)
		}
		return s.countTextsTokens(model, texts)
	case llmcore.CapabilityModeration:
		input, ok := req.Payload.(*ModerationInput)
		if !ok || input == nil || input.Request == nil {
			return 0
		}
		model := firstNonEmpty(input.Request.Model, req.ModelHint)
		return s.countTextsTokens(model, input.Request.Input)
	case llmcore.CapabilityTools:
		input, ok := req.Payload.(*ToolsInput)
		if !ok || input == nil {
			return 0
		}
		texts := make([]string, 0, len(input.Calls)*2)
		for _, call := range input.Calls {
			texts = append(texts, call.Name, string(call.Arguments))
		}
		return s.countTextsTokens(req.ModelHint, texts)
	default:
		return 0
	}
}

func (s *Service) estimateChatTokens(req *llmcore.UnifiedRequest, chatReq *llm.ChatRequest) int {
	model := firstNonEmpty(chatReq.Model, req.ModelHint)
	tokenizer := s.resolveTokenizer(model)
	if tokenizer == nil {
		return 0
	}

	messages := toTokenizerMessages(chatReq.Messages)
	promptTokens, err := tokenizer.CountMessages(messages)
	if err != nil {
		promptTokens = s.countTextsTokens(model, messageContents(chatReq.Messages))
	}

	toolsTokens := 0
	for _, schema := range chatReq.Tools {
		raw, marshalErr := json.Marshal(schema)
		if marshalErr != nil {
			continue
		}
		tokens, countErr := tokenizer.CountTokens(string(raw))
		if countErr != nil {
			continue
		}
		toolsTokens += tokens
	}

	completionBudget := 0
	if chatReq.MaxCompletionTokens != nil && *chatReq.MaxCompletionTokens > 0 {
		completionBudget = *chatReq.MaxCompletionTokens
	} else if chatReq.MaxTokens > 0 {
		completionBudget = chatReq.MaxTokens
	}

	total := promptTokens + toolsTokens + completionBudget
	if total < 0 {
		return 0
	}
	return total
}

func (s *Service) countTextsTokens(model string, texts []string) int {
	if len(texts) == 0 {
		return 0
	}
	tokenizer := s.resolveTokenizer(model)
	if tokenizer == nil {
		return 0
	}
	total := 0
	for _, text := range texts {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		count, err := tokenizer.CountTokens(trimmed)
		if err != nil || count < 0 {
			continue
		}
		total += count
	}
	return total
}

func (s *Service) resolveTokenizer(model string) llmtokenizer.Tokenizer {
	resolver := s.tokenizerResolver
	if resolver == nil {
		resolver = llmtokenizer.GetTokenizerOrEstimator
	}
	return resolver(firstNonEmpty(model, "gpt-4o-mini"))
}

func toTokenizerMessages(messages []types.Message) []llmtokenizer.Message {
	out := make([]llmtokenizer.Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, llmtokenizer.Message{
			Role:    string(msg.Role),
			Content: msg.Content,
		})
	}
	return out
}

func messageContents(messages []types.Message) []string {
	out := make([]string, 0, len(messages))
	for _, msg := range messages {
		out = append(out, msg.Content)
	}
	return out
}

func (s *Service) recordResponseUsage(req *llmcore.UnifiedRequest, resp *llmcore.UnifiedResponse) {
	if s == nil || s.policyManager == nil || resp == nil {
		return
	}

	s.policyManager.RecordUsage(llmpolicy.UsageRecord{
		Timestamp: time.Now(),
		Tokens:    resp.Usage.TotalTokens,
		Cost:      resp.Cost.AmountUSD,
		Model:     resp.ProviderDecision.Model,
		RequestID: firstNonEmpty(resp.TraceID, req.TraceID),
		UserID:    metadataValue(req, "user_id"),
		AgentID:   metadataValue(req, "agent_id"),
	})
}

func (s *Service) recordLedger(
	ctx context.Context,
	req *llmcore.UnifiedRequest,
	traceID string,
	decision llmcore.ProviderDecision,
	usage llmcore.Usage,
	cost llmcore.Cost,
) {
	if s == nil || s.ledger == nil || req == nil {
		return
	}

	err := s.ledger.Record(ctx, observability.LedgerEntry{
		Timestamp:  time.Now(),
		TraceID:    traceID,
		Capability: string(req.Capability),
		Provider:   decision.Provider,
		Model:      decision.Model,
		BaseURL:    decision.BaseURL,
		Strategy:   string(req.RoutePolicy),
		Usage:      usage,
		Cost:       cost,
		Metadata:   cloneMetadata(req.Metadata),
	})
	if err != nil {
		s.logger.Warn("gateway ledger record failed", zap.Error(err), zap.String("trace_id", traceID))
	}
}

func metadataValue(req *llmcore.UnifiedRequest, key string) string {
	if req == nil || req.Metadata == nil {
		return ""
	}
	return strings.TrimSpace(req.Metadata[key])
}

func ensureMetadata(req *llmcore.UnifiedRequest) map[string]string {
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	return req.Metadata
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneTags(src []string) []string {
	if len(src) == 0 {
		return nil
	}
	out := make([]string, len(src))
	copy(out, src)
	return out
}

func mergeChatRoutingMetadata(req *llmcore.UnifiedRequest, chatReq *llm.ChatRequest) {
	if req == nil || chatReq == nil {
		return
	}

	if len(req.Metadata) > 0 {
		if chatReq.Metadata == nil {
			chatReq.Metadata = make(map[string]string, len(req.Metadata))
		}
		for k, v := range req.Metadata {
			if strings.TrimSpace(chatReq.Metadata[k]) == "" {
				chatReq.Metadata[k] = v
			}
		}
	}

	providerHint := firstNonEmpty(
		strings.TrimSpace(chatReq.Metadata[llmcore.MetadataKeyChatProvider]),
		strings.TrimSpace(chatReq.Metadata["provider"]),
		strings.TrimSpace(chatReq.Metadata["provider_hint"]),
		strings.TrimSpace(req.ProviderHint),
		strings.TrimSpace(req.Hints.ChatProvider),
	)
	if providerHint != "" {
		if chatReq.Metadata == nil {
			chatReq.Metadata = make(map[string]string, 1)
		}
		chatReq.Metadata[llmcore.MetadataKeyChatProvider] = providerHint
		req.ProviderHint = providerHint
		if req.Hints.ChatProvider == "" {
			req.Hints.ChatProvider = providerHint
		}
	}

	routePolicy := normalizeRoutePolicy(firstNonEmpty(
		strings.TrimSpace(chatReq.Metadata["route_policy"]),
		string(req.RoutePolicy),
	))
	if routePolicy != "" {
		if chatReq.Metadata == nil {
			chatReq.Metadata = make(map[string]string, 1)
		}
		chatReq.Metadata["route_policy"] = string(routePolicy)
		req.RoutePolicy = routePolicy
	}

	if len(chatReq.Tags) == 0 && len(req.Tags) > 0 {
		chatReq.Tags = cloneTags(req.Tags)
	}
}

func normalizeRoutePolicy(raw string) llmcore.RoutePolicy {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "cost", "cost_first":
		return llmcore.RoutePolicyCostFirst
	case "health", "health_first":
		return llmcore.RoutePolicyHealthFirst
	case "latency", "latency_first":
		return llmcore.RoutePolicyLatencyFirst
	case "balanced":
		return llmcore.RoutePolicyBalanced
	default:
		return ""
	}
}

func providerHintFromMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	return firstNonEmpty(
		strings.TrimSpace(metadata[llmcore.MetadataKeyChatProvider]),
		strings.TrimSpace(metadata["provider"]),
		strings.TrimSpace(metadata["provider_hint"]),
	)
}

func buildUnifiedChatRequest(req *llm.ChatRequest) *llmcore.UnifiedRequest {
	if req == nil {
		return &llmcore.UnifiedRequest{Capability: llmcore.CapabilityChat}
	}
	metadata := cloneMetadata(req.Metadata)
	providerHint := providerHintFromMetadata(metadata)
	return &llmcore.UnifiedRequest{
		Capability:   llmcore.CapabilityChat,
		ProviderHint: providerHint,
		ModelHint:    req.Model,
		RoutePolicy:  normalizeRoutePolicy(firstNonEmpty(strings.TrimSpace(metadata["route_policy"]), "balanced")),
		TraceID:      req.TraceID,
		Hints: llmcore.CapabilityHints{
			ChatProvider: providerHint,
		},
		Payload:  req,
		Metadata: metadata,
		Tags:     cloneTags(req.Tags),
	}
}

func parseInt(raw string) int {
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func parseFloat(raw string) float64 {
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func costAmount(cost *llmcore.Cost) float64 {
	if cost == nil {
		return 0
	}
	if currency := strings.TrimSpace(cost.Currency); currency != "" && !strings.EqualFold(currency, "USD") {
		return 0
	}
	if cost.AmountUSD < 0 {
		return 0
	}
	return cost.AmountUSD
}

// ChatProviderAdapter 将 gateway 暴露为 llm.Provider，便于复用既有上层组件。
type ChatProviderAdapter struct {
	gateway  llmcore.Gateway
	fallback llm.Provider
}

// NewChatProviderAdapter 创建适配器。
func NewChatProviderAdapter(gw llmcore.Gateway, fallback llm.Provider) *ChatProviderAdapter {
	return &ChatProviderAdapter{
		gateway:  gw,
		fallback: fallback,
	}
}

func (a *ChatProviderAdapter) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	if a.gateway == nil {
		return nil, llmcore.GatewayUnavailableError("llm gateway is not configured")
	}
	resp, err := a.gateway.Invoke(ctx, buildUnifiedChatRequest(req))
	if err != nil {
		return nil, err
	}
	chatResp, ok := resp.Output.(*llm.ChatResponse)
	if !ok || chatResp == nil {
		return nil, types.NewInternalError("invalid gateway chat response")
	}
	return chatResp, nil
}

func (a *ChatProviderAdapter) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	if a.gateway == nil {
		return nil, llmcore.GatewayUnavailableError("llm gateway is not configured")
	}
	stream, err := a.gateway.Stream(ctx, buildUnifiedChatRequest(req))
	if err != nil {
		return nil, err
	}

	out := make(chan llm.StreamChunk)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zap.L().Error("ChatProviderAdapter stream relay panic recovered", zap.Any("panic", r))
			}
			close(out)
		}()
		for chunk := range stream {
			var sc llm.StreamChunk
			if chunk.Err != nil {
				sc = llm.StreamChunk{Err: chunk.Err}
			} else {
				streamChunk, ok := chunk.Output.(*llm.StreamChunk)
				if !ok || streamChunk == nil {
					sc = llm.StreamChunk{
						Err: types.NewInternalError("invalid gateway stream chunk"),
					}
				} else {
					sc = *streamChunk
				}
			}
			select {
			case out <- sc:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (a *ChatProviderAdapter) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	if a.fallback != nil {
		return a.fallback.HealthCheck(ctx)
	}
	return &llm.HealthStatus{Healthy: true}, nil
}

func (a *ChatProviderAdapter) Name() string {
	if a.fallback != nil {
		return a.fallback.Name()
	}
	return "gateway"
}

func (a *ChatProviderAdapter) SupportsNativeFunctionCalling() bool {
	if a.fallback != nil {
		return a.fallback.SupportsNativeFunctionCalling()
	}
	return false
}

// SupportsStructuredOutput 让 structured 包可保留对原生结构化输出探测。
func (a *ChatProviderAdapter) SupportsStructuredOutput() bool {
	if a.fallback == nil {
		return false
	}
	p, ok := a.fallback.(interface{ SupportsStructuredOutput() bool })
	return ok && p.SupportsStructuredOutput()
}

func (a *ChatProviderAdapter) ListModels(ctx context.Context) ([]llm.Model, error) {
	if a.fallback != nil {
		return a.fallback.ListModels(ctx)
	}
	return nil, nil
}

func (a *ChatProviderAdapter) Endpoints() llm.ProviderEndpoints {
	if a.fallback != nil {
		return a.fallback.Endpoints()
	}
	return llm.ProviderEndpoints{}
}

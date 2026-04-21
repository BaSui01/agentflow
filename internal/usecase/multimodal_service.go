package usecase

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/pkg/storage"
	"github.com/BaSui01/agentflow/types"
)

const defaultMultimodalNegativeText = "blurry, low quality, watermark, text, logo, signature, bad anatomy, deformed, mutated"

type MultimodalProviderResolver func(provider string) (string, error)

// MultimodalRuntime captures the hot-swappable runtime dependencies used by MultimodalService.
type MultimodalRuntime struct {
	Gateway              llmcore.Gateway
	Pipeline             multimodal.PromptPipeline
	ResolveImageProvider MultimodalProviderResolver
	ResolveVideoProvider MultimodalProviderResolver
	ReferenceStore       storage.ReferenceStore
	ReferenceTTL         time.Duration
	ReferenceMaxSize     int64
	ChatEnabled          bool
	DefaultChatModel     string
}

// MultimodalService encapsulates multimodal image/video/plan/chat execution.
type MultimodalService interface {
	GenerateImage(ctx context.Context, req MultimodalImageRequest) (*MultimodalImageResult, error)
	GenerateVideo(ctx context.Context, req MultimodalVideoRequest) (*MultimodalVideoResult, error)
	GeneratePlan(ctx context.Context, req MultimodalPlanRequest) (*MultimodalPlanResult, error)
	Chat(ctx context.Context, req MultimodalChatRequest) (*MultimodalChatResult, error)
}

// DefaultMultimodalService is the default MultimodalService implementation.
type DefaultMultimodalService struct {
	runtimeRef RuntimeRef[MultimodalRuntime]
}

// NewDefaultMultimodalService constructs the default multimodal usecase service.
func NewDefaultMultimodalService(runtime MultimodalRuntime) MultimodalService {
	return &DefaultMultimodalService{
		runtimeRef: NewAtomicRuntimeRef(runtime),
	}
}

// UpdateRuntime swaps the service runtime in place.
func (s *DefaultMultimodalService) UpdateRuntime(runtime MultimodalRuntime) {
	if s == nil {
		return
	}
	if s.runtimeRef == nil {
		s.runtimeRef = NewAtomicRuntimeRef(runtime)
		return
	}
	s.runtimeRef.Store(runtime)
}

func (s *DefaultMultimodalService) runtime() MultimodalRuntime {
	if s == nil || s.runtimeRef == nil {
		return MultimodalRuntime{}
	}
	return s.runtimeRef.Load()
}

func (s *DefaultMultimodalService) GenerateImage(ctx context.Context, req MultimodalImageRequest) (*MultimodalImageResult, error) {
	runtime := s.runtime()
	if runtime.Gateway == nil || runtime.ResolveImageProvider == nil || runtime.Pipeline == nil {
		return nil, types.NewServiceUnavailableError("multimodal runtime is not configured")
	}
	providerName, err := runtime.ResolveImageProvider(req.Provider)
	if err != nil {
		return nil, types.NewError(types.ErrInvalidRequest, err.Error()).WithCause(err)
	}

	negative := strings.TrimSpace(req.NegativePrompt)
	if negative == "" {
		negative = defaultMultimodalNegativeText
	}

	promptResult, err := runtime.Pipeline.Build(ctx, multimodal.PromptContext{
		Modality:       "image",
		BasePrompt:     req.Prompt,
		Advanced:       req.Advanced,
		StyleTokens:    req.StyleTokens,
		QualityTokens:  req.QualityTokens,
		NegativePrompt: negative,
	})
	if err != nil {
		return nil, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	result := &MultimodalImageResult{
		Provider:        providerName,
		EffectivePrompt: promptResult.Prompt,
		NegativePrompt:  promptResult.NegativePrompt,
	}

	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		result.Mode = "image-to-image"
		data, dataErr := s.resolveReferenceImage(runtime, timeoutCtx, req.ReferenceID, req.ReferenceImageURL)
		if dataErr != nil {
			return nil, dataErr
		}
		gatewayResp, invokeErr := runtime.Gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
			Capability:   llmcore.CapabilityImage,
			ProviderHint: providerName,
			ModelHint:    req.Model,
			Payload: &llmgateway.ImageInput{
				Provider: providerName,
				Edit: &image.EditRequest{
					Image:          bytes.NewReader(data),
					Prompt:         promptResult.Prompt,
					Model:          req.Model,
					N:              req.N,
					Size:           req.Size,
					ResponseFormat: req.ResponseFormat,
				},
			},
		})
		if invokeErr != nil {
			return nil, invokeErr
		}
		resp, ok := gatewayResp.Output.(*image.GenerateResponse)
		if !ok || resp == nil {
			return nil, types.NewInternalError("invalid image gateway response")
		}
		result.Response = resp
		return result, nil
	}

	result.Mode = "text-to-image"
	gatewayResp, invokeErr := runtime.Gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
		Capability:   llmcore.CapabilityImage,
		ProviderHint: providerName,
		ModelHint:    req.Model,
		Payload: &llmgateway.ImageInput{
			Provider: providerName,
			Generate: &image.GenerateRequest{
				Prompt:         promptResult.Prompt,
				NegativePrompt: promptResult.NegativePrompt,
				Model:          req.Model,
				N:              req.N,
				Size:           req.Size,
				Quality:        req.Quality,
				Style:          req.Style,
				ResponseFormat: req.ResponseFormat,
			},
		},
	})
	if invokeErr != nil {
		return nil, invokeErr
	}
	resp, ok := gatewayResp.Output.(*image.GenerateResponse)
	if !ok || resp == nil {
		return nil, types.NewInternalError("invalid image gateway response")
	}
	result.Response = resp
	return result, nil
}

func (s *DefaultMultimodalService) GenerateVideo(ctx context.Context, req MultimodalVideoRequest) (*MultimodalVideoResult, error) {
	runtime := s.runtime()
	if runtime.Gateway == nil || runtime.ResolveVideoProvider == nil || runtime.Pipeline == nil {
		return nil, types.NewServiceUnavailableError("multimodal runtime is not configured")
	}
	providerName, err := runtime.ResolveVideoProvider(req.Provider)
	if err != nil {
		return nil, types.NewError(types.ErrInvalidRequest, err.Error()).WithCause(err)
	}

	promptResult, err := runtime.Pipeline.Build(ctx, multimodal.PromptContext{
		Modality:       "video",
		BasePrompt:     req.Prompt,
		Advanced:       req.Advanced,
		StyleTokens:    req.StyleTokens,
		NegativePrompt: req.NegativePrompt,
		Camera:         req.Camera,
		Mood:           req.Mood,
	})
	if err != nil {
		return nil, err
	}

	genReq := &video.GenerateRequest{
		Prompt:         promptResult.Prompt,
		NegativePrompt: promptResult.NegativePrompt,
		Model:          req.Model,
		Duration:       req.Duration,
		AspectRatio:    req.AspectRatio,
		Resolution:     req.Resolution,
		FPS:            req.FPS,
		Seed:           req.Seed,
		ResponseFormat: req.ResponseFormat,
	}
	if strings.TrimSpace(req.CallbackURL) != "" {
		if genReq.Metadata == nil {
			genReq.Metadata = make(map[string]string)
		}
		genReq.Metadata["callback_url"] = strings.TrimSpace(req.CallbackURL)
	}

	mode := "text-to-video"
	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		mode = "image-to-video"
		if req.ReferenceID != "" {
			data, mimeType, ok := s.getReference(runtime, req.ReferenceID)
			if !ok {
				return nil, types.NewError(types.ErrInvalidRequest, "reference_id not found or expired")
			}
			attachReferenceImage(providerName, genReq, data, mimeType)
		} else {
			validatedURL, urlErr := multimodal.ValidatePublicReferenceImageURL(ctx, req.ReferenceImageURL)
			if urlErr != nil {
				return nil, types.NewError(types.ErrInvalidRequest, urlErr.Error()).WithCause(urlErr)
			}
			genReq.ImageURL = validatedURL
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	gatewayResp, invokeErr := runtime.Gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
		Capability:   llmcore.CapabilityVideo,
		ProviderHint: providerName,
		ModelHint:    req.Model,
		Payload: &llmgateway.VideoInput{
			Provider: providerName,
			Generate: genReq,
		},
	})
	if invokeErr != nil {
		return nil, invokeErr
	}
	resp, ok := gatewayResp.Output.(*video.GenerateResponse)
	if !ok || resp == nil {
		return nil, types.NewInternalError("invalid video gateway response")
	}

	return &MultimodalVideoResult{
		Mode:            mode,
		Provider:        providerName,
		EffectivePrompt: promptResult.Prompt,
		Response:        resp,
	}, nil
}

func (s *DefaultMultimodalService) resolveReferenceImage(runtime MultimodalRuntime, ctx context.Context, referenceID, referenceURL string) ([]byte, error) {
	if referenceID != "" {
		data, _, ok := s.getReference(runtime, referenceID)
		if !ok {
			return nil, types.NewError(types.ErrInvalidRequest, "reference_id not found or expired")
		}
		return data, nil
	}

	validatedURL, urlErr := multimodal.ValidatePublicReferenceImageURL(ctx, referenceURL)
	if urlErr != nil {
		return nil, types.NewError(types.ErrInvalidRequest, urlErr.Error()).WithCause(urlErr)
	}
	data, _, dlErr := multimodal.DownloadReferenceImage(ctx, validatedURL, runtime.ReferenceMaxSize)
	if dlErr != nil {
		return nil, types.NewError(types.ErrInvalidRequest, dlErr.Error()).WithCause(dlErr)
	}
	return data, nil
}

func (s *DefaultMultimodalService) getReference(runtime MultimodalRuntime, id string) ([]byte, string, bool) {
	if runtime.ReferenceStore == nil {
		return nil, "", false
	}
	ref, ok := runtime.ReferenceStore.Get(id)
	if !ok || ref == nil {
		return nil, "", false
	}
	if runtime.ReferenceTTL > 0 && time.Since(ref.CreatedAt) > runtime.ReferenceTTL {
		runtime.ReferenceStore.Delete(id)
		return nil, "", false
	}
	return append([]byte(nil), ref.Data...), ref.MimeType, true
}

func attachReferenceImage(providerName string, req *video.GenerateRequest, data []byte, mimeType string) {
	b64 := base64.StdEncoding.EncodeToString(data)
	if providerName == "veo" {
		req.Image = b64
		return
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	req.ImageURL = fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
}

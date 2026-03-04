package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm/capabilities/image"
	"github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	"github.com/BaSui01/agentflow/llm/capabilities/video"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	"github.com/BaSui01/agentflow/types"
)

type multimodalService interface {
	GenerateImage(ctx context.Context, req multimodalImageRequest) (*multimodalImageResult, error)
	GenerateVideo(ctx context.Context, req multimodalVideoRequest) (*multimodalVideoResult, error)
}

type referenceLoader func(id string) ([]byte, string, bool)

type defaultMultimodalService struct {
	gateway              llmcore.Gateway
	pipeline             multimodal.PromptPipeline
	resolveImageProvider func(string) (string, error)
	resolveVideoProvider func(string) (string, error)
	loadReference        referenceLoader
	referenceMaxSize     int64
}

type multimodalImageResult struct {
	Mode            string
	Provider        string
	EffectivePrompt string
	NegativePrompt  string
	Response        *image.GenerateResponse
}

type multimodalVideoResult struct {
	Mode            string
	Provider        string
	EffectivePrompt string
	Response        *video.GenerateResponse
}

func newDefaultMultimodalService(
	gateway llmcore.Gateway,
	pipeline multimodal.PromptPipeline,
	resolveImageProvider func(string) (string, error),
	resolveVideoProvider func(string) (string, error),
	loadReference referenceLoader,
	referenceMaxSize int64,
) multimodalService {
	return &defaultMultimodalService{
		gateway:              gateway,
		pipeline:             pipeline,
		resolveImageProvider: resolveImageProvider,
		resolveVideoProvider: resolveVideoProvider,
		loadReference:        loadReference,
		referenceMaxSize:     referenceMaxSize,
	}
}

func (s *defaultMultimodalService) GenerateImage(ctx context.Context, req multimodalImageRequest) (*multimodalImageResult, error) {
	providerName, err := s.resolveImageProvider(req.Provider)
	if err != nil {
		return nil, types.NewError(types.ErrInvalidRequest, err.Error()).
			WithCause(err)
	}

	negative := strings.TrimSpace(req.NegativePrompt)
	if negative == "" {
		negative = defaultNegativeText
	}

	promptResult, err := s.pipeline.Build(ctx, multimodal.PromptContext{
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

	result := &multimodalImageResult{
		Provider:        providerName,
		EffectivePrompt: promptResult.Prompt,
		NegativePrompt:  promptResult.NegativePrompt,
	}

	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		result.Mode = "image-to-image"
		data, dataErr := s.resolveReferenceImage(timeoutCtx, req.ReferenceID, req.ReferenceImageURL)
		if dataErr != nil {
			return nil, dataErr
		}
		gatewayResp, invokeErr := s.gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
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
	gatewayResp, invokeErr := s.gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
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

func (s *defaultMultimodalService) GenerateVideo(ctx context.Context, req multimodalVideoRequest) (*multimodalVideoResult, error) {
	providerName, err := s.resolveVideoProvider(req.Provider)
	if err != nil {
		return nil, types.NewError(types.ErrInvalidRequest, err.Error()).
			WithCause(err)
	}

	promptResult, err := s.pipeline.Build(ctx, multimodal.PromptContext{
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

	mode := "text-to-video"
	if req.ReferenceID != "" || strings.TrimSpace(req.ReferenceImageURL) != "" {
		mode = "image-to-video"
		if req.ReferenceID != "" {
			data, mimeType, ok := s.loadReference(req.ReferenceID)
			if !ok {
				return nil, types.NewError(types.ErrInvalidRequest, "reference_id not found or expired")
			}
			attachReferenceImage(providerName, genReq, data, mimeType)
		} else {
			validatedURL, urlErr := multimodal.ValidatePublicReferenceImageURL(ctx, req.ReferenceImageURL)
			if urlErr != nil {
				return nil, types.NewError(types.ErrInvalidRequest, urlErr.Error()).
					WithCause(urlErr)
			}
			genReq.ImageURL = validatedURL
		}
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 6*time.Minute)
	defer cancel()
	gatewayResp, invokeErr := s.gateway.Invoke(timeoutCtx, &llmcore.UnifiedRequest{
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

	return &multimodalVideoResult{
		Mode:            mode,
		Provider:        providerName,
		EffectivePrompt: promptResult.Prompt,
		Response:        resp,
	}, nil
}

func (s *defaultMultimodalService) resolveReferenceImage(ctx context.Context, referenceID, referenceURL string) ([]byte, error) {
	if referenceID != "" {
		data, _, ok := s.loadReference(referenceID)
		if !ok {
			return nil, types.NewError(types.ErrInvalidRequest, "reference_id not found or expired")
		}
		return data, nil
	}

	validatedURL, urlErr := multimodal.ValidatePublicReferenceImageURL(ctx, referenceURL)
	if urlErr != nil {
		return nil, types.NewError(types.ErrInvalidRequest, urlErr.Error()).
			WithCause(urlErr)
	}
	data, _, dlErr := multimodal.DownloadReferenceImage(ctx, validatedURL, s.referenceMaxSize)
	if dlErr != nil {
		return nil, types.NewError(types.ErrInvalidRequest, dlErr.Error()).
			WithCause(dlErr)
	}
	return data, nil
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

func toHTTPStatus(err error) int {
	var typedErr *types.Error
	if errors.As(err, &typedErr) {
		if typedErr != nil && typedErr.Code == types.ErrInvalidRequest {
			return http.StatusBadRequest
		}
	}
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "required") ||
		strings.Contains(lower, "unsupported") ||
		strings.Contains(lower, "not support") {
		return http.StatusBadRequest
	}
	return http.StatusBadGateway
}

func errorCodeFrom(err error, fallback types.ErrorCode) types.ErrorCode {
	var typedErr *types.Error
	if errors.As(err, &typedErr) && typedErr != nil {
		return typedErr.Code
	}
	if toHTTPStatus(err) == http.StatusBadRequest {
		return types.ErrInvalidRequest
	}
	return fallback
}

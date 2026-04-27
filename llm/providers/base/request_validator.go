package providerbase

import (
	"fmt"
	"net/http"
	"strings"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/types"
)

func ValidateTemperatureTopPMutualExclusion(temperature, topP float32, provider string) error {
	if temperature != 0 && topP != 0 {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("%s requests should set either temperature or top_p, but not both", strings.Title(provider)),
			HTTPStatus: http.StatusBadRequest,
			Provider:   provider,
		}
	}
	return nil
}

func ValidateMaxTokensRange(maxTokens, minVal, maxVal int, provider string) error {
	if maxTokens <= 0 {
		return nil
	}
	if minVal > 0 && maxTokens < minVal {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("%s max_tokens must be >= %d, got %d", strings.Title(provider), minVal, maxTokens),
			HTTPStatus: http.StatusBadRequest,
			Provider:   provider,
		}
	}
	if maxVal > 0 && maxTokens > maxVal {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("%s max_tokens must be <= %d, got %d", strings.Title(provider), maxVal, maxTokens),
			HTTPStatus: http.StatusBadRequest,
			Provider:   provider,
		}
	}
	return nil
}

func ValidateTemperatureRange(temperature float32, minVal, maxVal float32, provider string) error {
	if temperature == 0 {
		return nil
	}
	if temperature < minVal {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("%s temperature must be >= %g, got %g", strings.Title(provider), minVal, temperature),
			HTTPStatus: http.StatusBadRequest,
			Provider:   provider,
		}
	}
	if temperature > maxVal {
		return &types.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    fmt.Sprintf("%s temperature must be <= %g, got %g", strings.Title(provider), maxVal, temperature),
			HTTPStatus: http.StatusBadRequest,
			Provider:   provider,
		}
	}
	return nil
}

func ValidateModelName(model string, allowedPrefixes []string, provider string) error {
	if model == "" {
		return nil
	}
	if len(allowedPrefixes) == 0 {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(model))
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(normalized, strings.ToLower(strings.TrimSpace(prefix))) {
			return nil
		}
	}
	return &types.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("%s model %q does not match any allowed prefix: %v", strings.Title(provider), model, allowedPrefixes),
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

func RewriteChainError(err error, provider string) *types.Error {
	return &types.Error{
		Code:       llm.ErrInvalidRequest,
		Message:    fmt.Sprintf("request rewrite failed: %v", err),
		HTTPStatus: http.StatusBadRequest,
		Provider:   provider,
	}
}

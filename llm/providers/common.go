package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/BaSui01/agentflow/llm"
)

// MapHTTPError maps HTTP status codes to llm.Error with appropriate retry flags
// This is a common error mapping function used by all providers
func MapHTTPError(status int, msg string, provider string) *llm.Error {
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{
			Code:       llm.ErrUnauthorized,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusForbidden:
		return &llm.Error{
			Code:       llm.ErrForbidden,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusTooManyRequests:
		return &llm.Error{
			Code:       llm.ErrRateLimited,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	case http.StatusBadRequest:
		// Check for quota/credit keywords
		msgLower := strings.ToLower(msg)
		if strings.Contains(msgLower, "quota") ||
			strings.Contains(msgLower, "credit") ||
			strings.Contains(msgLower, "limit") {
			return &llm.Error{
				Code:       llm.ErrQuotaExceeded,
				Message:    msg,
				HTTPStatus: status,
				Provider:   provider,
			}
		}
		return &llm.Error{
			Code:       llm.ErrInvalidRequest,
			Message:    msg,
			HTTPStatus: status,
			Provider:   provider,
		}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	case 529: // Model overloaded (used by some providers)
		return &llm.Error{
			Code:       llm.ErrModelOverloaded,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  true,
			Provider:   provider,
		}
	default:
		return &llm.Error{
			Code:       llm.ErrUpstreamError,
			Message:    msg,
			HTTPStatus: status,
			Retryable:  status >= 500,
			Provider:   provider,
		}
	}
}

// ReadErrorMessage reads error message from response body
// Attempts to parse JSON error response, falls back to raw text
func ReadErrorMessage(body io.Reader) string {
	data, err := io.ReadAll(body)
	if err != nil {
		return "failed to read error response"
	}

	// Try to parse as generic error response
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		if errResp.Error.Type != "" {
			return fmt.Sprintf("%s (type: %s)", errResp.Error.Message, errResp.Error.Type)
		}
		return errResp.Error.Message
	}

	// Fallback to raw text
	return string(data)
}

// ChooseModel selects the model to use based on request and default
func ChooseModel(req *llm.ChatRequest, defaultModel, fallbackModel string) string {
	if req != nil && req.Model != "" {
		return req.Model
	}
	if defaultModel != "" {
		return defaultModel
	}
	return fallbackModel
}

// SafeCloseBody safely closes HTTP response body and logs errors
func SafeCloseBody(body io.ReadCloser) {
	if body != nil {
		_ = body.Close()
	}
}

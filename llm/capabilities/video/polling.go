package video

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	defaultVideoTimeout           = 300 * time.Second
	defaultVideoPollInterval      = 5 * time.Second
	maxVideoGenerateConcurrency   = 16
	maxVideoPollAttempts          = 120
	maxVideoPollConsecutiveErrors = 3
	maxVideoPollInterval          = 60 * time.Second
	maxVideoErrorLogBodyBytes     = 2048
	pollProgressLogEvery          = 10
	pollSlowWarnThreshold         = 10
)

var videoGenerateSemaphore = make(chan struct{}, maxVideoGenerateConcurrency)

func startProviderSpan(ctx context.Context, provider string, operation string) (context.Context, trace.Span) {
	ctx, span := otel.Tracer("agentflow/llm/video").Start(ctx, "video."+provider+"."+operation)
	span.SetAttributes(
		attribute.String("video.provider", provider),
		attribute.String("video.operation", operation),
	)
	return ctx, span
}

func acquireVideoGenerateSlot(ctx context.Context) (func(), error) {
	select {
	case videoGenerateSemaphore <- struct{}{}:
		return func() {
			<-videoGenerateSemaphore
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func marshalJSONRequest(provider string, body any) ([]byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s request: %w", provider, err)
	}
	return payload, nil
}

func httpStatusError(logger *zap.Logger, provider string, phase string, statusCode int, bodyReader io.Reader) error {
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		if logger != nil {
			logger.Error("video provider request failed and response body read failed",
				zap.String("provider", provider),
				zap.String("phase", phase),
				zap.Int("status_code", statusCode),
				zap.Error(err))
		}
		return fmt.Errorf("%s error: status=%d", provider, statusCode)
	}
	bodyText := truncateLogText(string(body), maxVideoErrorLogBodyBytes)
	if logger != nil {
		logger.Error("video provider request failed",
			zap.String("provider", provider),
			zap.String("phase", phase),
			zap.Int("status_code", statusCode),
			zap.String("response_body", bodyText))
	}
	return fmt.Errorf("%s error: status=%d", provider, statusCode)
}

func withResponseBodyClose(resp *http.Response, fn func() error) error {
	defer resp.Body.Close()
	return fn()
}

func decodeJSONAndClose(resp *http.Response, out any) error {
	return withResponseBodyClose(resp, func() error {
		return json.NewDecoder(resp.Body).Decode(out)
	})
}

func statusErrorAndClose(logger *zap.Logger, provider string, phase string, resp *http.Response) error {
	return withResponseBodyClose(resp, func() error {
		return httpStatusError(logger, provider, phase, resp.StatusCode, resp.Body)
	})
}

func nextPollInterval(current time.Duration) time.Duration {
	if current <= 0 {
		return defaultVideoPollInterval
	}
	next := current * 2
	if next > maxVideoPollInterval {
		return maxVideoPollInterval
	}
	return next
}

func truncateLogText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	text := strings.TrimSpace(s)
	if len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}

func shortPromptForLog(prompt string) string {
	return truncateLogText(prompt, 120)
}

var pollTaskIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
var pollOperationNamePattern = regexp.MustCompile(`^[A-Za-z0-9._/:-]+$`)

func validatePollTaskID(taskID string) error {
	trimmed := strings.TrimSpace(taskID)
	if trimmed == "" {
		return fmt.Errorf("task id must not be empty")
	}
	if !pollTaskIDPattern.MatchString(trimmed) {
		return fmt.Errorf("task id contains invalid characters")
	}
	return nil
}

func validatePollOperationName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("operation name must not be empty")
	}
	if !pollOperationNamePattern.MatchString(trimmed) {
		return fmt.Errorf("operation name contains invalid characters")
	}
	return nil
}

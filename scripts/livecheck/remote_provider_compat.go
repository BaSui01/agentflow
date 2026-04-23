package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/providers"
	claudeprov "github.com/BaSui01/agentflow/llm/providers/anthropic"
	geminiprov "github.com/BaSui01/agentflow/llm/providers/gemini"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
)

func runAnthropicGLM5RemoteCompat(ctx context.Context, logger *zap.Logger) error {
	if !remoteCompatEnabled() {
		logger.Info("test J skipped (set LIVECHECK_ENABLE_REMOTE_COMPAT=1 to enable)")
		return nil
	}
	logger.Info("test J: anthropic glm-5 remote compatibility start")

	baseURL, err := getenvRequired("AGENT_BASE_URL")
	if err != nil {
		return err
	}
	apiKey, err := getenvRequired("AGENT_API_KEY")
	if err != nil {
		return err
	}

	p := claudeprov.NewClaudeProvider(providers.ClaudeConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   "glm-5",
			Timeout: 60 * time.Second,
		},
		AnthropicVersion: "2023-06-01",
	}, logger)

	modelCtx, cancelModels := context.WithTimeout(ctx, 30*time.Second)
	defer cancelModels()
	models, err := p.ListModels(modelCtx)
	if err != nil {
		return fmt.Errorf("anthropic list models failed: %w", err)
	}
	if !hasModel(models, "glm-5") {
		return fmt.Errorf("anthropic endpoint does not expose model glm-5")
	}

	reqCtx, cancelReq := context.WithTimeout(ctx, 60*time.Second)
	defer cancelReq()
	resp, err := p.Completion(reqCtx, &llm.ChatRequest{
		Model: "glm-5",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "Reply with exactly: OK"},
		},
		MaxTokens:   128,
		Temperature: 0,
	})
	if err != nil {
		return fmt.Errorf("anthropic completion failed: %w", err)
	}
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return fmt.Errorf("anthropic no choice: %w", err)
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return fmt.Errorf("anthropic completion returned empty text content")
	}

	logger.Info("test J: anthropic glm-5 remote compatibility done",
		zap.String("model", resp.Model),
		zap.String("answer", truncateText(choice.Message.Content, 120)),
	)
	return nil
}

func runGeminiGLM5RemoteCompat(ctx context.Context, logger *zap.Logger) error {
	if !remoteCompatEnabled() {
		logger.Info("test K skipped (set LIVECHECK_ENABLE_REMOTE_COMPAT=1 to enable)")
		return nil
	}
	logger.Info("test K: gemini glm-5 remote compatibility start")

	baseURL, err := getenvRequired("AGENT_BASE_URL")
	if err != nil {
		return err
	}
	apiKey, err := getenvRequired("AGENT_API_KEY")
	if err != nil {
		return err
	}

	p := geminiprov.NewGeminiProvider(providers.GeminiConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   "glm-5",
			Timeout: 60 * time.Second,
		},
	}, logger)

	modelCtx, cancelModels := context.WithTimeout(ctx, 30*time.Second)
	defer cancelModels()
	models, err := p.ListModels(modelCtx)
	if err != nil {
		return fmt.Errorf("gemini list models failed: %w", err)
	}
	if !hasModel(models, "glm-5") {
		return fmt.Errorf("gemini endpoint does not expose model glm-5")
	}

	reqCtx, cancelReq := context.WithTimeout(ctx, 60*time.Second)
	defer cancelReq()
	resp, err := p.Completion(reqCtx, &llm.ChatRequest{
		Model: "glm-5",
		Messages: []types.Message{
			{Role: llm.RoleUser, Content: "Reply with exactly: OK"},
		},
		MaxTokens:   64,
		Temperature: 0,
	})
	if err != nil {
		return fmt.Errorf("gemini completion failed: %w", err)
	}
	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return fmt.Errorf("gemini no choice: %w", err)
	}
	if strings.TrimSpace(choice.Message.Content) == "" {
		return fmt.Errorf("gemini completion returned empty text content")
	}

	logger.Info("test K: gemini glm-5 remote compatibility done",
		zap.String("model", resp.Model),
		zap.String("answer", truncateText(choice.Message.Content, 120)),
	)
	return nil
}

func remoteCompatEnabled() bool {
	v := strings.TrimSpace(os.Getenv("LIVECHECK_ENABLE_REMOTE_COMPAT"))
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func hasModel(models []llm.Model, id string) bool {
	for _, m := range models {
		if strings.EqualFold(strings.TrimSpace(m.ID), id) {
			return true
		}
	}
	return false
}

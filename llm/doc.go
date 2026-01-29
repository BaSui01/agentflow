// Copyright 2024 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by a MIT license that can be
// found in the LICENSE file.

/*
Package llm provides unified LLM provider abstraction and routing.

# Overview

The llm package provides a unified interface for interacting with multiple
Large Language Model providers. It abstracts away provider-specific details
and provides features like routing, caching, retry logic, and observability.

# Architecture

	┌─────────────────────────────────────────────────────────────┐
	│                    Application Layer                        │
	├─────────────────────────────────────────────────────────────┤
	│                    Router / Load Balancer                   │
	│  (Model-based routing, failover, load balancing)           │
	├─────────────────────────────────────────────────────────────┤
	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
	│  │   Cache     │  │   Retry     │  │   Observability     │ │
	│  │  (L1/L2)    │  │  (Backoff)  │  │  (Metrics/Tracing)  │ │
	│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
	├─────────────────────────────────────────────────────────────┤
	│                    Provider Interface                       │
	├──────────┬──────────┬──────────┬──────────┬────────────────┤
	│  OpenAI  │ Anthropic│  Gemini  │ DeepSeek │    Others...   │
	└──────────┴──────────┴──────────┴──────────┴────────────────┘

# Provider Interface

The core Provider interface defines the contract for all LLM providers:

	type Provider interface {
	    Completion(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	    Stream(ctx context.Context, req *ChatRequest) (<-chan StreamChunk, error)
	    HealthCheck(ctx context.Context) (*HealthStatus, error)
	    Name() string
	    SupportsNativeFunctionCalling() bool
	}

# Supported Providers

The package supports 13+ LLM providers out of the box:

  - OpenAI (GPT-4, GPT-4o, GPT-3.5-turbo)
  - Anthropic (Claude 3 Opus, Sonnet, Haiku)
  - Google (Gemini Pro, Gemini Ultra)
  - DeepSeek (DeepSeek-Chat, DeepSeek-Coder)
  - Alibaba (Qwen-Turbo, Qwen-Plus, Qwen-Max)
  - Tencent (Hunyuan)
  - Moonshot (Kimi)
  - Zhipu (GLM-4)
  - ByteDance (Doubao)
  - Baidu (ERNIE)
  - MiniMax
  - Mistral
  - Meta (Llama)
  - xAI (Grok)

# Usage

Basic usage with a single provider:

	provider, err := openai.NewProvider(&openai.Config{
	    APIKey: "your-api-key",
	    Model:  "gpt-4o",
	})
	if err != nil {
	    log.Fatal(err)
	}

	resp, err := provider.Completion(ctx, &llm.ChatRequest{
	    Model: "gpt-4o",
	    Messages: []llm.Message{
	        {Role: llm.RoleUser, Content: "Hello!"},
	    },
	})

Using the router for multi-provider setup:

	router := llm.NewRouter(
	    llm.WithProvider("openai", openaiProvider),
	    llm.WithProvider("anthropic", anthropicProvider),
	    llm.WithFallback("anthropic"),
	)

	// Router automatically selects provider based on model
	resp, err := router.Completion(ctx, &llm.ChatRequest{
	    Model: "claude-3-opus", // Routes to Anthropic
	})

# Streaming

All providers support streaming responses:

	stream, err := provider.Stream(ctx, &llm.ChatRequest{
	    Model: "gpt-4o",
	    Messages: messages,
	})
	if err != nil {
	    log.Fatal(err)
	}

	for chunk := range stream {
	    if chunk.Error != nil {
	        log.Printf("Error: %v", chunk.Error)
	        break
	    }
	    fmt.Print(chunk.Content)
	}

# Caching

The package provides multi-level caching:

	cache := cache.NewMultiLevelCache(redisClient, &cache.CacheConfig{
	    LocalMaxSize: 1000,
	    LocalTTL:     5 * time.Minute,
	    RedisTTL:     1 * time.Hour,
	    EnableLocal:  true,
	    EnableRedis:  true,
	})

# Retry and Resilience

Built-in retry with exponential backoff:

	resilientProvider := llm.NewResilientProvider(provider, &llm.ResilienceConfig{
	    MaxRetries:     3,
	    InitialBackoff: 100 * time.Millisecond,
	    MaxBackoff:     10 * time.Second,
	    CircuitBreaker: true,
	})

# Observability

The package integrates with OpenTelemetry for metrics and tracing:

	provider := observability.WrapProvider(baseProvider, &observability.Config{
	    EnableMetrics: true,
	    EnableTracing: true,
	    ServiceName:   "my-service",
	})

# Tool Calling

Support for native function calling:

	resp, err := provider.Completion(ctx, &llm.ChatRequest{
	    Model: "gpt-4o",
	    Messages: messages,
	    Tools: []llm.ToolSchema{
	        {
	            Name:        "get_weather",
	            Description: "Get current weather for a location",
	            Parameters:  weatherParamsSchema,
	        },
	    },
	})

# Error Handling

The package defines structured error codes:

	const (
	    ErrInvalidRequest      ErrorCode = "invalid_request"
	    ErrAuthentication      ErrorCode = "authentication_error"
	    ErrRateLimit           ErrorCode = "rate_limit"
	    ErrContextTooLong      ErrorCode = "context_too_long"
	    ErrServiceUnavailable  ErrorCode = "service_unavailable"
	)

Use IsRetryable to check if an error can be retried:

	if llm.IsRetryable(err) {
	    // Implement retry logic
	}

# API Key Management

Support for API key pools with rotation:

	pool := llm.NewAPIKeyPool(db, providerID, llm.StrategyRoundRobin, logger)
	pool.LoadKeys(ctx)
	key, err := pool.SelectKey(ctx)

See the subpackages for additional functionality:
  - llm/cache: Prompt caching with multiple strategies
  - llm/middleware: Request/response middleware
  - llm/observability: Metrics, tracing, and cost tracking
  - llm/retry: Retry strategies and backoff
  - llm/router: Multi-provider routing
  - llm/tools: ReAct loop and tool execution
  - llm/providers/*: Provider-specific implementations
*/
package llm

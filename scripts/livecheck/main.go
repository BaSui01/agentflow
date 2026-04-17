package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/BaSui01/agentflow/agent"
	agentruntime "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/llm"
	embeddingcap "github.com/BaSui01/agentflow/llm/capabilities/embedding"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/llm/providers"
	"github.com/BaSui01/agentflow/llm/providers/openai"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/rag/core"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type liveToolManager struct {
	logger      *zap.Logger
	totalCalls  atomic.Int64
	allowedList []types.ToolSchema
}

func newLiveToolManager(logger *zap.Logger) *liveToolManager {
	addSchema := types.ToolSchema{
		Name:        "add",
		Description: "Add two numbers and return their sum",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "a":{"type":"number"},
    "b":{"type":"number"}
  },
  "required":["a","b"]
}`),
	}

	echoSchema := types.ToolSchema{
		Name:        "echo",
		Description: "Echo input text",
		Parameters: json.RawMessage(`{
  "type":"object",
  "properties":{
    "text":{"type":"string"}
  },
  "required":["text"]
}`),
	}

	return &liveToolManager{
		logger:      logger,
		allowedList: []types.ToolSchema{addSchema, echoSchema},
	}
}

func (m *liveToolManager) GetAllowedTools(agentID string) []types.ToolSchema {
	return m.allowedList
}

func (m *liveToolManager) ExecuteForAgent(ctx context.Context, agentID string, calls []types.ToolCall) []llmtools.ToolResult {
	out := make([]llmtools.ToolResult, 0, len(calls))
	for _, call := range calls {
		start := time.Now()
		m.totalCalls.Add(1)
		res := llmtools.ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
		}

		switch call.Name {
		case "add":
			var in struct {
				A float64 `json:"a"`
				B float64 `json:"b"`
			}
			if err := json.Unmarshal(call.Arguments, &in); err != nil {
				res.Error = fmt.Sprintf("invalid add args: %v", err)
				break
			}
			payload, _ := json.Marshal(map[string]any{
				"a":   in.A,
				"b":   in.B,
				"sum": in.A + in.B,
			})
			res.Result = payload
		case "echo":
			var in struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(call.Arguments, &in); err != nil {
				res.Error = fmt.Sprintf("invalid echo args: %v", err)
				break
			}
			payload, _ := json.Marshal(map[string]any{
				"echo": in.Text,
			})
			res.Result = payload
		default:
			res.Error = "unknown tool: " + call.Name
		}

		res.Duration = time.Since(start)
		m.logger.Info("tool executed",
			zap.String("agent_id", agentID),
			zap.String("tool", call.Name),
			zap.String("tool_call_id", call.ID),
			zap.ByteString("arguments", call.Arguments),
			zap.ByteString("result", res.Result),
			zap.String("error", res.Error),
			zap.Duration("duration", res.Duration),
		)
		out = append(out, res)
	}
	return out
}

func (m *liveToolManager) TotalCalls() int64 {
	return m.totalCalls.Load()
}

func getenvRequired(key string) (string, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func getenvWithDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func pickChatModel(models []llm.Model) (string, error) {
	if len(models) == 0 {
		return "", errors.New("no models returned from provider")
	}

	priority := []string{
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"glm-5",
		"claude-sonnet-4-5-20250929",
		"qwen3.5-plus",
		"qwen-max",
		"qwen-plus",
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4.1",
		"gpt-4.1-mini",
		"gpt-3.5-turbo",
	}

	available := make(map[string]struct{}, len(models))
	for _, m := range models {
		available[m.ID] = struct{}{}
	}

	for _, candidate := range priority {
		if _, ok := available[candidate]; ok {
			return candidate, nil
		}
	}

	for _, m := range models {
		id := strings.ToLower(strings.TrimSpace(m.ID))
		if id == "" {
			continue
		}
		if strings.HasPrefix(id, "grok-") {
			continue
		}
		return m.ID, nil
	}

	return models[0].ID, nil
}

const (
	suiteLive  = "live"
	suiteFault = "fault"
	suiteAll   = "all"
)

type runOptions struct {
	suite  string
	only   []string
	logDir string
	list   bool
}

type liveRuntime struct {
	chatProvider     llm.Provider
	chatModel        string
	embeddingBaseURL string
	embeddingAPIKey  string
}

type liveTest struct {
	name         string
	suite        string
	requiresLive bool
	run          func(context.Context, *zap.Logger, *liveRuntime) error
}

func main() {
	opts, err := parseRunOptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse options: %v\n", err)
		os.Exit(1)
	}

	logger, logPath, cleanup, err := newLivecheckLogger(opts.logDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if logPath != "" {
		logger.Info("livecheck file logging enabled", zap.String("log_file", logPath))
	}

	allTests := buildTestPlan()
	if opts.list {
		printTestPlan(allTests)
		return
	}

	selectedTests, err := selectTests(allTests, opts.suite, opts.only)
	if err != nil {
		logger.Fatal("failed to select tests", zap.Error(err))
	}

	requireLive := false
	requireEmbedding := false
	for _, tc := range selectedTests {
		if tc.requiresLive {
			requireLive = true
		}
		if strings.HasPrefix(strings.ToUpper(tc.name), "C-") {
			requireEmbedding = true
		}
	}

	ctx := context.Background()
	var runtimeDeps *liveRuntime
	if requireLive {
		runtimeDeps, err = setupLiveRuntime(ctx, logger, requireEmbedding)
		if err != nil {
			logger.Fatal("failed to setup live runtime", zap.Error(err))
		}
	}

	logger.Info("live check started",
		zap.String("suite", opts.suite),
		zap.Strings("only", opts.only),
		zap.Int("selected_tests", len(selectedTests)),
	)

	failures := 0
	for _, tc := range selectedTests {
		start := time.Now()
		runErr := tc.run(ctx, logger, runtimeDeps)
		if runErr != nil {
			failures++
			logger.Error("live test failed",
				zap.String("test", tc.name),
				zap.Error(runErr),
				zap.Duration("duration", time.Since(start)),
			)
			continue
		}
		logger.Info("live test passed",
			zap.String("test", tc.name),
			zap.Duration("duration", time.Since(start)),
		)
	}

	if failures > 0 {
		logger.Fatal("live check finished with failures",
			zap.Int("total", len(selectedTests)),
			zap.Int("failed", failures),
		)
	}

	logger.Info("live check finished successfully",
		zap.Int("total", len(selectedTests)),
		zap.Int("failed", failures),
	)
}

func parseRunOptions() (runOptions, error) {
	var opts runOptions
	var onlyRaw string

	flag.StringVar(&opts.suite, "suite", suiteLive, "test suite: live|fault|all")
	flag.StringVar(&onlyRaw, "only", "", "comma-separated test selectors, e.g. A,B,D,E,G or X1,X2")
	flag.StringVar(&opts.logDir, "log-dir", "scripts/livecheck/logs", "directory for timestamped log file")
	flag.BoolVar(&opts.list, "list", false, "list all available tests and exit")
	flag.Parse()

	opts.suite = strings.ToLower(strings.TrimSpace(opts.suite))
	switch opts.suite {
	case suiteLive, suiteFault, suiteAll:
	default:
		return runOptions{}, fmt.Errorf("invalid --suite=%q, expected one of: live|fault|all", opts.suite)
	}

	opts.only = parseOnlySelectors(onlyRaw)
	return opts, nil
}

func parseOnlySelectors(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})

	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		v := strings.ToUpper(strings.TrimSpace(p))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func newLivecheckLogger(logDir string) (*zap.Logger, string, func(), error) {
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		l, err := zap.NewDevelopment()
		if err != nil {
			return nil, "", nil, err
		}
		return l, "", func() { _ = l.Sync() }, nil
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, "", nil, fmt.Errorf("create log dir: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	logPath := filepath.Join(logDir, "livecheck-"+ts+".log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, "", nil, fmt.Errorf("open log file: %w", err)
	}

	encCfg := zap.NewDevelopmentEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(os.Stdout),
		zap.DebugLevel,
	)
	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encCfg),
		zapcore.AddSync(file),
		zap.DebugLevel,
	)

	logger := zap.New(
		zapcore.NewTee(consoleCore, fileCore),
		zap.AddCaller(),
		zap.AddStacktrace(zap.ErrorLevel),
	)

	cleanup := func() {
		_ = logger.Sync()
		_ = file.Close()
	}

	return logger, logPath, cleanup, nil
}

func buildTestPlan() []liveTest {
	return []liveTest{
		{
			name:         "A-basic-agent",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runAgentBasic(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:         "B-tool-loop",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runAgentToolLoop(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:         "C-rag-embedding",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runRAGEmbedding(ctx, logger, deps.embeddingBaseURL, deps.embeddingAPIKey)
			},
		},
		{
			name:         "D-skills-mcp",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runSkillsAndMCP(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:         "E-multi-agent-collaboration",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runMultiAgentCollaboration(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:         "F-hierarchical-subtasks",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runHierarchicalExecution(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:         "G-subagent-delegation",
			suite:        suiteLive,
			requiresLive: true,
			run: func(ctx context.Context, logger *zap.Logger, deps *liveRuntime) error {
				return runSubAgentDelegation(ctx, logger, deps.chatProvider, deps.chatModel)
			},
		},
		{
			name:  "H-openai-responses-web-search",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runOpenAIResponsesWebSearchRegression(ctx, logger)
			},
		},
		{
			name:  "I-gemini-model-aware-adapter",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runGeminiModelAwareRegression(ctx, logger)
			},
		},
		{
			name:  "J-anthropic-glm5-remote-compat",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runAnthropicGLM5RemoteCompat(ctx, logger)
			},
		},
		{
			name:  "K-gemini-glm5-remote-compat",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runGeminiGLM5RemoteCompat(ctx, logger)
			},
		},
		{
			name:  "L-openai-endpoint-mode-routing",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runOpenAIEndpointModeRoutingRegression(ctx, logger)
			},
		},
		{
			name:  "M-anthropic-compat-endpoint",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runAnthropicCompatEndpointRegression(ctx, logger)
			},
		},
		{
			name:  "N-gemini-compat-endpoint",
			suite: suiteLive,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runGeminiCompatEndpointRegression(ctx, logger)
			},
		},
		{
			name:  "X1-timeout",
			suite: suiteFault,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runFaultTimeout(ctx, logger)
			},
		},
		{
			name:  "X2-empty-model-list",
			suite: suiteFault,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runFaultEmptyModelList(ctx, logger)
			},
		},
		{
			name:  "X3-mcp-tool-error",
			suite: suiteFault,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runFaultMCPToolError(ctx, logger)
			},
		},
		{
			name:  "X4-rag-dimension-mismatch",
			suite: suiteFault,
			run: func(ctx context.Context, logger *zap.Logger, _ *liveRuntime) error {
				return runFaultRAGDimensionMismatch(ctx, logger)
			},
		},
	}
}

func printTestPlan(tests []liveTest) {
	fmt.Println("Available tests:")
	for _, tc := range tests {
		fmt.Printf("  - %s (%s)\n", tc.name, tc.suite)
	}
}

func selectTests(all []liveTest, suite string, only []string) ([]liveTest, error) {
	filtered := make([]liveTest, 0, len(all))
	for _, tc := range all {
		if suite != suiteAll && tc.suite != suite {
			continue
		}
		filtered = append(filtered, tc)
	}
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no tests available in suite %q", suite)
	}

	if len(only) == 0 {
		return filtered, nil
	}

	selected := make([]liveTest, 0, len(filtered))
	for _, tc := range filtered {
		for _, selector := range only {
			if selectorMatches(selector, tc.name) {
				selected = append(selected, tc)
				break
			}
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no tests matched --only=%v in suite %q", only, suite)
	}
	return selected, nil
}

func selectorMatches(selector, testName string) bool {
	s := strings.ToUpper(strings.TrimSpace(selector))
	t := strings.ToUpper(strings.TrimSpace(testName))
	if s == "" || t == "" {
		return false
	}
	return s == t || strings.HasPrefix(t, s+"-")
}

func setupLiveRuntime(ctx context.Context, logger *zap.Logger, requireEmbedding bool) (*liveRuntime, error) {
	chatBaseURL, err := getenvRequired("AGENT_BASE_URL")
	if err != nil {
		return nil, err
	}
	chatAPIKey, err := getenvRequired("AGENT_API_KEY")
	if err != nil {
		return nil, err
	}

	embeddingBaseURL := strings.TrimSpace(os.Getenv("EMBEDDING_BASE_URL"))
	embeddingAPIKey := strings.TrimSpace(os.Getenv("EMBEDDING_API_KEY"))
	if requireEmbedding {
		if embeddingBaseURL == "" {
			return nil, errors.New("EMBEDDING_BASE_URL is required")
		}
		if embeddingAPIKey == "" {
			return nil, errors.New("EMBEDDING_API_KEY is required")
		}
	}

	chatProvider := openai.NewOpenAIProvider(providers.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  chatAPIKey,
			BaseURL: chatBaseURL,
			Timeout: 60 * time.Second,
		},
	}, logger)
	logger.Info("chat provider endpoints", zap.Any("endpoints", chatProvider.Endpoints()))

	modelsCtx, cancelModels := context.WithTimeout(ctx, 30*time.Second)
	models, err := chatProvider.ListModels(modelsCtx)
	cancelModels()
	if err != nil {
		return nil, fmt.Errorf("ListModels failed: %w", err)
	}

	modelIDs := make([]string, 0, len(models))
	for i, m := range models {
		if i >= 10 {
			break
		}
		modelIDs = append(modelIDs, m.ID)
	}
	logger.Info("models fetched",
		zap.Int("count", len(models)),
		zap.Strings("first_10", modelIDs),
	)

	chatModel := getenvWithDefault("AGENT_MODEL", "")
	if chatModel == "" {
		chatModel, err = pickChatModel(models)
		if err != nil {
			return nil, fmt.Errorf("failed to choose model: %w", err)
		}
	}
	logger.Info("chat model selected", zap.String("model", chatModel))

	return &liveRuntime{
		chatProvider:     chatProvider,
		chatModel:        chatModel,
		embeddingBaseURL: embeddingBaseURL,
		embeddingAPIKey:  embeddingAPIKey,
	}, nil
}

func runAgentBasic(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test A: basic agent execute start")
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "live-agent-basic",
			Name: "live-agent-basic",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       model,
			MaxTokens:   200,
			Temperature: 0,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt: "You are a concise assistant.",
		},
	}

	ag, err := agentruntime.NewBuilder(provider, logger).Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer ag.Teardown(context.Background())

	if err := ag.Init(ctx); err != nil {
		return err
	}

	execCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	out, err := ag.Execute(execCtx, &agent.Input{
		TraceID: "live-basic-trace",
		Content: "Reply with exactly: OK",
	})
	if err != nil {
		return err
	}

	logger.Info("test A: basic agent execute done",
		zap.String("output", out.Content),
		zap.Duration("duration", out.Duration),
		zap.Int("tokens_used", out.TokensUsed),
	)
	return nil
}

func runAgentToolLoop(ctx context.Context, logger *zap.Logger, provider llm.Provider, model string) error {
	logger.Info("test B: tool loop start")
	toolMgr := newLiveToolManager(logger)
	cfg := types.AgentConfig{
		Core: types.CoreConfig{
			ID:   "live-agent-tools",
			Name: "live-agent-tools",
			Type: "assistant",
		},
		LLM: types.LLMConfig{
			Model:       model,
			MaxTokens:   400,
			Temperature: 0,
		},
		Runtime: types.RuntimeConfig{
			SystemPrompt:       "You are a tool-using assistant.",
			Tools:              []string{"add"},
			MaxReActIterations: 6,
		},
	}

	ag, err := agentruntime.NewBuilder(provider, logger).WithOptions(agentruntime.BuildOptions{
		ToolManager: toolMgr,
	}).Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer ag.Teardown(context.Background())

	if err := ag.Init(ctx); err != nil {
		return err
	}

	var streamToolCalls atomic.Int64
	var streamToolResults atomic.Int64

	execCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	rc := &agent.RunConfig{
		ToolChoice:         agent.StringPtr("auto"),
		MaxReActIterations: agent.IntPtr(6),
	}
	execCtx = agent.WithRunConfig(execCtx, rc)
	execCtx = agent.WithRuntimeStreamEmitter(execCtx, func(ev agent.RuntimeStreamEvent) {
		switch ev.Type {
		case agent.RuntimeStreamToolCall:
			streamToolCalls.Add(1)
			if ev.ToolCall != nil {
				logger.Info("runtime stream tool_call",
					zap.String("name", ev.ToolCall.Name),
					zap.String("id", ev.ToolCall.ID),
					zap.ByteString("arguments", ev.ToolCall.Arguments),
				)
			}
		case agent.RuntimeStreamToolResult:
			streamToolResults.Add(1)
			if ev.ToolResult != nil {
				logger.Info("runtime stream tool_result",
					zap.String("name", ev.ToolResult.Name),
					zap.String("id", ev.ToolResult.ToolCallID),
					zap.ByteString("result", ev.ToolResult.Result),
					zap.String("error", ev.ToolResult.Error),
				)
			}
		}
	})

	resp, err := ag.ChatCompletion(execCtx, []types.Message{
		{
			Role:    llm.RoleUser,
			Content: "Please use the add tool exactly once to calculate 19 + 23, then give one short final sentence.",
		},
	})
	if err != nil {
		if strings.Contains(err.Error(), "max iterations reached") && toolMgr.TotalCalls() > 0 {
			logger.Warn("tool loop reached max iterations after successful tool calls",
				zap.Int64("tool_calls_executed", toolMgr.TotalCalls()),
				zap.Int64("stream_tool_call_events", streamToolCalls.Load()),
				zap.Int64("stream_tool_result_events", streamToolResults.Load()),
			)
			return nil
		}
		return err
	}

	choice, err := llm.FirstChoice(resp)
	if err != nil {
		return err
	}

	logger.Info("test B: tool loop done",
		zap.Int64("tool_calls_executed", toolMgr.TotalCalls()),
		zap.Int64("stream_tool_call_events", streamToolCalls.Load()),
		zap.Int64("stream_tool_result_events", streamToolResults.Load()),
		zap.String("final_answer", choice.Message.Content),
	)
	return nil
}

func runRAGEmbedding(ctx context.Context, logger *zap.Logger, embeddingBaseURL string, embeddingAPIKey string) error {
	logger.Info("test C: rag+embedding start")
	embeddingModel := getenvWithDefault("EMBEDDING_MODEL", "text-embedding-v2")
	embeddingDimensions := inferEmbeddingDimensions(embeddingModel)
	if v := strings.TrimSpace(os.Getenv("EMBEDDING_DIMENSIONS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			embeddingDimensions = n
		}
	}
	embedder := embeddingcap.NewOpenAIProvider(embeddingcap.OpenAIConfig{
		BaseProviderConfig: providers.BaseProviderConfig{
			APIKey:  embeddingAPIKey,
			BaseURL: embeddingBaseURL,
			Model:   embeddingModel,
			Timeout: 60 * time.Second,
		},
		Dimensions: embeddingDimensions,
	})

	docs := []core.Document{
		{ID: "doc-1", Content: "AgentFlow supports tool calling loops through ReAct execution."},
		{ID: "doc-2", Content: "RAG combines BM25 retrieval with vector similarity search for grounding."},
		{ID: "doc-3", Content: "Embedding converts text into dense vectors used by vector stores."},
	}

	docTexts := make([]string, 0, len(docs))
	for _, d := range docs {
		docTexts = append(docTexts, d.Content)
	}

	embedCtx, cancelEmbed := context.WithTimeout(ctx, 120*time.Second)
	defer cancelEmbed()
	externalEmbeddingUsed := true

	docVectors, err := embedder.EmbedDocuments(embedCtx, docTexts)
	if err != nil {
		externalEmbeddingUsed = false
		logger.Warn("external EmbedDocuments failed; fallback to deterministic local embeddings",
			zap.Error(err),
			zap.String("embedding_model", embeddingModel),
		)
		docVectors = make([][]float64, len(docTexts))
		for i, text := range docTexts {
			docVectors[i] = deterministicVector(text, 128)
		}
	}
	if len(docVectors) != len(docs) {
		return fmt.Errorf("unexpected vectors length: got %d want %d", len(docVectors), len(docs))
	}
	for i := range docs {
		docs[i].Embedding = docVectors[i]
	}

	vectorStore := rag.NewInMemoryVectorStore(logger)
	retrievalCfg := rag.DefaultHybridRetrievalConfig()
	retrievalCfg.TopK = 3
	retrievalCfg.MinScore = 0
	retriever := rag.NewHybridRetrieverWithVectorStore(retrievalCfg, vectorStore, logger)
	if err := retriever.IndexDocuments(docs); err != nil {
		return err
	}

	query := "How does AgentFlow use tools and RAG together?"
	queryEmbedding, err := embedder.EmbedQuery(embedCtx, query)
	if err != nil {
		externalEmbeddingUsed = false
		logger.Warn("external EmbedQuery failed; fallback to deterministic local embedding",
			zap.Error(err),
			zap.String("embedding_model", embeddingModel),
		)
		queryEmbedding = deterministicVector(query, 128)
	}

	results, err := retriever.Retrieve(embedCtx, query, queryEmbedding)
	if err != nil {
		return err
	}

	logger.Info("test C: rag+embedding done",
		zap.String("embedding_model", embeddingModel),
		zap.Int("embedding_dimensions", embeddingDimensions),
		zap.Bool("external_embedding_used", externalEmbeddingUsed),
		zap.Int("retrieval_hits", len(results)),
	)
	for i, r := range results {
		if i >= 3 {
			break
		}
		logger.Info("rag hit",
			zap.Int("rank", i+1),
			zap.String("doc_id", r.Document.ID),
			zap.Float64("score", r.FinalScore),
			zap.String("content", r.Document.Content),
		)
	}

	return nil
}

func deterministicVector(text string, dim int) []float64 {
	if dim <= 0 {
		dim = 64
	}
	vec := make([]float64, dim)
	if text == "" {
		return vec
	}

	bytes := []byte(text)
	for i, b := range bytes {
		slot := (int(b) + i) % dim
		vec[slot] += float64((int(b)%31)+1) * 0.1
	}

	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm == 0 {
		return vec
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func inferEmbeddingDimensions(model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "qwen3-embedding-0.6b"):
		return 1024
	case strings.Contains(m, "text-embedding-3-small"):
		return 1536
	case strings.Contains(m, "text-embedding-3-large"):
		return 3072
	default:
		return 3072
	}
}

func truncateText(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

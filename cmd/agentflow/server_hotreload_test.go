package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/BaSui01/agentflow/agent/observability/hitl"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/api"
	"github.com/BaSui01/agentflow/api/handlers"
	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/internal/usecase"
	llmcore "github.com/BaSui01/agentflow/llm/core"
	llmgateway "github.com/BaSui01/agentflow/llm/gateway"
	pkgserver "github.com/BaSui01/agentflow/pkg/server"
	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type hotReloadProvider struct {
	content string
	name    string
}

func (p *hotReloadProvider) Completion(_ context.Context, req *llmcore.ChatRequest) (*llmcore.ChatResponse, error) {
	return &llmcore.ChatResponse{
		Provider: p.name,
		Model:    req.Model,
		Choices: []llmcore.ChatChoice{{
			Index: 0,
			Message: types.Message{
				Role:    types.RoleAssistant,
				Content: p.content,
			},
		}},
	}, nil
}

func (p *hotReloadProvider) Stream(_ context.Context, req *llmcore.ChatRequest) (<-chan llmcore.StreamChunk, error) {
	out := make(chan llmcore.StreamChunk, 1)
	out <- llmcore.StreamChunk{
		Provider: p.name,
		Model:    req.Model,
		Delta: types.Message{
			Role:    types.RoleAssistant,
			Content: p.content,
		},
	}
	close(out)
	return out, nil
}

func (*hotReloadProvider) HealthCheck(context.Context) (*llmcore.HealthStatus, error) {
	return &llmcore.HealthStatus{Healthy: true}, nil
}

func (p *hotReloadProvider) Name() string { return p.name }

func (*hotReloadProvider) SupportsNativeFunctionCalling() bool { return true }

func (*hotReloadProvider) ListModels(context.Context) ([]llmcore.Model, error) { return nil, nil }

func (*hotReloadProvider) Endpoints() llmcore.ProviderEndpoints { return llmcore.ProviderEndpoints{} }

func (p *hotReloadProvider) CountTokens(context.Context, *llmcore.ChatRequest) (*llmcore.TokenCountResponse, error) {
	return &llmcore.TokenCountResponse{
		Model:       p.name,
		InputTokens: 4,
		TotalTokens: 4,
	}, nil
}

type hotReloadTestAgent struct {
	id        string
	teardowns *atomic.Int32
}

func (a *hotReloadTestAgent) ID() string            { return a.id }
func (a *hotReloadTestAgent) Name() string          { return a.id }
func (a *hotReloadTestAgent) Type() agent.AgentType { return agent.TypeGeneric }
func (a *hotReloadTestAgent) State() agent.State    { return agent.StateReady }
func (a *hotReloadTestAgent) Init(context.Context) error {
	return nil
}
func (a *hotReloadTestAgent) Teardown(context.Context) error {
	if a.teardowns != nil {
		a.teardowns.Add(1)
	}
	return nil
}
func (a *hotReloadTestAgent) Plan(context.Context, *agent.Input) (*agent.PlanResult, error) {
	return &agent.PlanResult{}, nil
}
func (a *hotReloadTestAgent) Execute(context.Context, *agent.Input) (*agent.Output, error) {
	return &agent.Output{Content: a.id}, nil
}
func (a *hotReloadTestAgent) Observe(context.Context, *agent.Feedback) error { return nil }

func TestServerHotReload_UpdatesChatHandlerInPlace(t *testing.T) {
	t.Parallel()

	modeA := "reload-a"
	modeB := "reload-b"
	bootstrap.UnregisterMainProviderBuilder(modeA)
	bootstrap.UnregisterMainProviderBuilder(modeB)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeA,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-a", content: "from-a"}, nil
		}))
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeB,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-b", content: "from-b"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(modeA)
	defer bootstrap.UnregisterMainProviderBuilder(modeB)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = modeA
	cfg.Budget.Enabled = false

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	require.NoError(t, s.reloadLLMRuntime(cfg))
	require.NotNil(t, s.handlers.chatHandler)
	handler := s.handlers.chatHandler
	require.NoError(t, s.initHotReloadManager())

	assertHotReloadChatContent(t, handler, "from-a")

	newCfg := config.DefaultConfig()
	newCfg.LLM.MainProviderMode = modeB
	newCfg.Budget.Enabled = false
	require.NoError(t, s.ops.hotReloadManager.ApplyConfig(newCfg, "test"))

	require.Same(t, handler, s.handlers.chatHandler)
	require.Equal(t, modeB, s.cfg.LLM.MainProviderMode)
	assertHotReloadChatContent(t, handler, "from-b")
}

func TestServerHotReload_RollsBackOnRuntimeRebuildFailure(t *testing.T) {
	t.Parallel()

	modeA := "reload-good"
	modeBroken := "reload-broken"
	bootstrap.UnregisterMainProviderBuilder(modeA)
	bootstrap.UnregisterMainProviderBuilder(modeBroken)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeA,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-good", content: "stable"}, nil
		}))
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeBroken,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return nil, fmt.Errorf("broken builder")
		}))
	defer bootstrap.UnregisterMainProviderBuilder(modeA)
	defer bootstrap.UnregisterMainProviderBuilder(modeBroken)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = modeA
	cfg.Budget.Enabled = false

	s := &Server{cfg: cfg, logger: zap.NewNop()}
	require.NoError(t, s.reloadLLMRuntime(cfg))
	require.NotNil(t, s.handlers.chatHandler)
	handler := s.handlers.chatHandler
	require.NoError(t, s.initHotReloadManager())

	badCfg := config.DefaultConfig()
	badCfg.LLM.MainProviderMode = modeBroken
	badCfg.Budget.Enabled = false
	err := s.ops.hotReloadManager.ApplyConfig(badCfg, "test")
	require.Error(t, err)

	require.Same(t, handler, s.handlers.chatHandler)
	require.Equal(t, modeA, s.cfg.LLM.MainProviderMode)
	require.Equal(t, modeA, s.ops.hotReloadManager.GetConfig().LLM.MainProviderMode)
	assertHotReloadChatContent(t, handler, "stable")
}

func TestServerHotReload_RequiresRestartToActivateMissingChatAndCostRoutes(t *testing.T) {
	t.Parallel()

	modeGood := "reload-startup-good"
	bootstrap.UnregisterMainProviderBuilder(modeGood)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeGood,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-good", content: "live"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(modeGood)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = modeGood
	cfg.Budget.Enabled = false

	s := &Server{
		cfg:    cfg,
		logger: zap.NewNop(),
		ops:    serverOpsBundle{httpManager: &pkgserver.Manager{}},
	}
	require.Nil(t, s.handlers.chatHandler)
	require.Nil(t, s.handlers.costHandler)
	require.NoError(t, s.reloadLLMRuntime(cfg))

	require.NotNil(t, s.text.provider)
	require.Nil(t, s.handlers.chatHandler)
	require.Nil(t, s.handlers.costHandler)
}

func TestServerHotReload_TeardownsPreviousResolverCache(t *testing.T) {
	t.Parallel()

	modeA := "reload-resolver-a"
	modeB := "reload-resolver-b"
	bootstrap.UnregisterMainProviderBuilder(modeA)
	bootstrap.UnregisterMainProviderBuilder(modeB)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeA,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-resolver-a", content: "resolver-a"}, nil
		}))
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeB,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-resolver-b", content: "resolver-b"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(modeA)
	defer bootstrap.UnregisterMainProviderBuilder(modeB)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = modeA
	cfg.Budget.Enabled = false

	s := &Server{
		cfg:    cfg,
		logger: zap.NewNop(),
	}
	require.NoError(t, s.reloadLLMRuntime(cfg))
	require.NotNil(t, s.text.provider)

	var teardowns atomic.Int32
	oldRegistry := agent.NewAgentRegistry(zap.NewNop())
	oldRegistry.Register(agent.TypeGeneric, func(
		config types.AgentConfig,
		_ llmcore.Gateway,
		_ agent.MemoryManager,
		_ agent.ToolManager,
		_ agent.EventBus,
		_ *zap.Logger,
	) (agent.Agent, error) {
		return &hotReloadTestAgent{id: config.Core.ID, teardowns: &teardowns}, nil
	})

	oldResolver := agent.NewCachingResolver(oldRegistry, llmgateway.New(llmgateway.Config{
		ChatProvider: s.text.provider,
		Logger:       zap.NewNop(),
	}), zap.NewNop())
	_, err := oldResolver.Resolve(context.Background(), "agent-before-reload")
	require.NoError(t, err)

	s.workflow.resolver = oldResolver
	s.tooling.agentRegistry = agent.NewAgentRegistry(zap.NewNop())
	require.NoError(t, s.initHotReloadManager())

	newCfg := config.DefaultConfig()
	newCfg.LLM.MainProviderMode = modeB
	newCfg.Budget.Enabled = false
	require.NoError(t, s.ops.hotReloadManager.ApplyConfig(newCfg, "test"))

	require.NotNil(t, s.workflow.resolver)
	require.NotSame(t, oldResolver, s.workflow.resolver)
	require.EqualValues(t, 1, teardowns.Load())
}

func TestServerHotReload_ReusesWorkflowHITLManager(t *testing.T) {
	t.Parallel()

	modeA := "reload-workflow-a"
	modeB := "reload-workflow-b"
	bootstrap.UnregisterMainProviderBuilder(modeA)
	bootstrap.UnregisterMainProviderBuilder(modeB)
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeA,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-workflow-a", content: "a"}, nil
		}))
	require.NoError(t, bootstrap.RegisterMainProviderBuilder(modeB,
		func(context.Context, *config.Config, *gorm.DB, *zap.Logger) (llmcore.Provider, error) {
			return &hotReloadProvider{name: "provider-workflow-b", content: "b"}, nil
		}))
	defer bootstrap.UnregisterMainProviderBuilder(modeA)
	defer bootstrap.UnregisterMainProviderBuilder(modeB)

	cfg := config.DefaultConfig()
	cfg.LLM.MainProviderMode = modeA
	cfg.Budget.Enabled = false

	s := &Server{
		cfg:    cfg,
		logger: zap.NewNop(),
	}
	require.NoError(t, s.reloadLLMRuntime(cfg))
	workflowRuntime := bootstrap.BuildReloadedWorkflowRuntime(bootstrap.ReloadedWorkflowRuntimeBuildInput{
		DefaultModel:            cfg.Agent.Model,
		RetrievalStore:          s.workflow.ragStore,
		EmbeddingProvider:       s.workflow.ragEmbedding,
		CheckpointStore:         s.workflow.checkpointStore,
		WorkflowCheckpointStore: s.workflow.workflowCheckpointStore,
		HITLManager:             s.currentWorkflowHITLManager(),
		Logger:                  s.logger,
	})
	s.handlers.workflowHandler = handlers.NewWorkflowHandler(usecase.NewDefaultWorkflowService(workflowRuntime.Facade, workflowRuntime.Parser), s.logger)
	require.NotNil(t, s.handlers.workflowHandler)
	require.NotNil(t, s.currentWorkflowHITLManager())

	workflowHandler := s.handlers.workflowHandler
	manager := extractWorkflowHITLManager(t, workflowHandler)
	require.Same(t, manager, s.currentWorkflowHITLManager())
	require.NoError(t, s.initHotReloadManager())

	newCfg := config.DefaultConfig()
	newCfg.LLM.MainProviderMode = modeB
	newCfg.Budget.Enabled = false
	require.NoError(t, s.ops.hotReloadManager.ApplyConfig(newCfg, "test"))

	require.Same(t, workflowHandler, s.handlers.workflowHandler)
	require.Same(t, manager, s.currentWorkflowHITLManager())
	require.Same(t, manager, extractWorkflowHITLManager(t, s.handlers.workflowHandler))
}

func assertHotReloadChatContent(t *testing.T, handler *handlers.ChatHandler, expected string) {
	t.Helper()

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	handler.HandleCompletion(w, r)
	require.Equal(t, http.StatusOK, w.Code)

	var envelope api.Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&envelope))
	require.True(t, envelope.Success)

	bytes, err := json.Marshal(envelope.Data)
	require.NoError(t, err)

	var resp api.ChatResponse
	require.NoError(t, json.Unmarshal(bytes, &resp))
	require.Equal(t, expected, resp.Choices[0].Message.Content)
}

func extractWorkflowHITLManager(t *testing.T, handler *handlers.WorkflowHandler) *hitl.InterruptManager {
	t.Helper()

	require.NotNil(t, handler)

	handlerValue := reflect.ValueOf(handler).Elem()
	service := readUnexportedField(handlerValue.FieldByName("service"))
	require.NotNil(t, service)

	parserField := reflect.ValueOf(service).Elem().FieldByName("parser")
	parser := readUnexportedField(parserField)
	require.NotNil(t, parser)

	stepDepsField := reflect.ValueOf(parser).Elem().FieldByName("stepDeps")
	humanHandler := readUnexportedField(stepDepsField.FieldByName("HumanHandler"))
	require.NotNil(t, humanHandler)

	requesterField := addressableValue(reflect.ValueOf(humanHandler)).FieldByName("requester")
	requester := readUnexportedField(requesterField)
	require.NotNil(t, requester)

	managerField := reflect.ValueOf(requester).Elem().FieldByName("manager")
	manager, ok := readUnexportedField(managerField).(*hitl.InterruptManager)
	require.True(t, ok)
	return manager
}

func addressableValue(v reflect.Value) reflect.Value {
	if v.CanAddr() {
		return v
	}
	copy := reflect.New(v.Type()).Elem()
	copy.Set(v)
	return copy
}

func readUnexportedField(v reflect.Value) any {
	v = addressableValue(v)
	return reflect.NewAt(v.Type(), unsafe.Pointer(v.UnsafeAddr())).Elem().Interface()
}

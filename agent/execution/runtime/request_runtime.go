package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	agenthandoff "github.com/BaSui01/agentflow/agent/adapters/handoff"
	"github.com/BaSui01/agentflow/agent/capabilities/guardrails"
	planningcap "github.com/BaSui01/agentflow/agent/capabilities/planning"
	toolcap "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcore "github.com/BaSui01/agentflow/agent/core"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm "github.com/BaSui01/agentflow/llm/core"
	"github.com/BaSui01/agentflow/llm/observability"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// ExecutionOptionsResolver resolves a provider-agnostic runtime view from the
// current agent configuration and request-scoped overrides.
type ExecutionOptionsResolver interface {
	Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions
}

// DefaultExecutionOptionsResolver is the runtime's canonical resolver.
type DefaultExecutionOptionsResolver struct{}

func NewDefaultExecutionOptionsResolver() ExecutionOptionsResolver {
	return DefaultExecutionOptionsResolver{}
}

// Resolve builds execution options from AgentConfig plus context-scoped hints
// and RunConfig overrides. The returned value is detached from cfg.
func (DefaultExecutionOptionsResolver) Resolve(ctx context.Context, cfg types.AgentConfig, input *Input) types.ExecutionOptions {
	options := cfg.ExecutionOptions()
	if model, ok := types.LLMModel(ctx); ok && strings.TrimSpace(model) != "" {
		options.Model.Model = strings.TrimSpace(model)
	}
	if provider, ok := types.LLMProvider(ctx); ok && strings.TrimSpace(provider) != "" {
		options.Model.Provider = strings.TrimSpace(provider)
	}
	if routePolicy, ok := types.LLMRoutePolicy(ctx); ok && strings.TrimSpace(routePolicy) != "" {
		options.Model.RoutePolicy = strings.TrimSpace(routePolicy)
	}
	if rc := ResolveRunConfig(ctx, input); rc != nil {
		rc.ApplyToExecutionOptions(&options)
	}
	if flag, ok := boolOverrideFromContext(inputContext(input), "disable_planner"); ok {
		options.Control.DisablePlanner = flag
	}
	if value, ok := intOverrideFromContext(inputContext(input), "top_level_loop_budget"); ok && value > 0 {
		options.Control.MaxLoopIterations = value
	}
	return options
}

func contextBool(input *Input, key string) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	flag, ok := value.(bool)
	return ok && flag
}

func contextString(input *Input, key string) string {
	if input == nil || len(input.Context) == 0 {
		return ""
	}
	value, ok := input.Context[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func intContextAtLeast(input *Input, key string, min int) bool {
	if input == nil || len(input.Context) == 0 {
		return false
	}
	value, ok := input.Context[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case int:
		return typed >= min
	case int32:
		return int(typed) >= min
	case int64:
		return int(typed) >= min
	case float64:
		return int(typed) >= min
	default:
		return false
	}
}

func contentContainsAny(input *Input, terms ...string) bool {
	if input == nil {
		return false
	}
	content := strings.ToLower(input.Content)
	for _, term := range terms {
		if strings.Contains(content, strings.ToLower(term)) {
			return true
		}
	}
	return false
}

// runConfigKey is the unexported context key for RunConfig.
type runConfigKey struct{}

// RunConfig provides runtime overrides for Agent execution.
// All pointer fields use nil to indicate "no override" — only non-nil values
// are applied, leaving the base Config defaults intact.
type RunConfig struct {
	Model              *string           `json:"model,omitempty"`
	Provider           *string           `json:"provider,omitempty"`
	RoutePolicy        *string           `json:"route_policy,omitempty"`
	Temperature        *float32          `json:"temperature,omitempty"`
	MaxTokens          *int              `json:"max_tokens,omitempty"`
	TopP               *float32          `json:"top_p,omitempty"`
	Stop               []string          `json:"stop,omitempty"`
	ToolChoice         *string           `json:"tool_choice,omitempty"`
	ToolWhitelist      []string          `json:"tool_whitelist,omitempty"`
	DisableTools       bool              `json:"disable_tools,omitempty"`
	Timeout            *time.Duration    `json:"timeout,omitempty"`
	MaxReActIterations *int              `json:"max_react_iterations,omitempty"`
	MaxLoopIterations  *int              `json:"max_loop_iterations,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	Tags               []string          `json:"tags,omitempty"`
}

// WithRunConfig stores a RunConfig in the context.
func WithRunConfig(ctx context.Context, rc *RunConfig) context.Context {
	return context.WithValue(ctx, runConfigKey{}, rc)
}

// GetRunConfig retrieves the RunConfig from the context.
// Returns nil if no RunConfig is present.
func GetRunConfig(ctx context.Context) *RunConfig {
	rc, _ := ctx.Value(runConfigKey{}).(*RunConfig)
	return rc
}

// ApplyToExecutionOptions applies RunConfig overrides to the provider-agnostic
// execution options consumed by the agent runtime.
func (rc *RunConfig) ApplyToExecutionOptions(opts *types.ExecutionOptions) {
	if rc == nil || opts == nil {
		return
	}

	if rc.Model != nil {
		opts.Model.Model = *rc.Model
	}
	if rc.Provider != nil {
		opts.Model.Provider = strings.TrimSpace(*rc.Provider)
	}
	if rc.RoutePolicy != nil {
		opts.Model.RoutePolicy = strings.TrimSpace(*rc.RoutePolicy)
	}
	if rc.Temperature != nil {
		opts.Model.Temperature = *rc.Temperature
	}
	if rc.MaxTokens != nil {
		opts.Model.MaxTokens = *rc.MaxTokens
	}
	if rc.TopP != nil {
		opts.Model.TopP = *rc.TopP
	}
	if len(rc.Stop) > 0 {
		opts.Model.Stop = append([]string(nil), rc.Stop...)
	}
	if rc.ToolChoice != nil {
		opts.Tools.ToolChoice = types.ParseToolChoiceString(strings.TrimSpace(*rc.ToolChoice))
	}
	if rc.DisableTools {
		opts.Tools.DisableTools = true
		opts.Tools.ToolWhitelist = nil
	}
	if len(rc.ToolWhitelist) > 0 {
		opts.Tools.ToolWhitelist = append([]string(nil), rc.ToolWhitelist...)
		opts.Tools.DisableTools = false
	}
	if rc.Timeout != nil {
		opts.Control.Timeout = *rc.Timeout
	}
	if rc.MaxReActIterations != nil {
		opts.Control.MaxReActIterations = *rc.MaxReActIterations
	}
	if rc.MaxLoopIterations != nil {
		opts.Control.MaxLoopIterations = *rc.MaxLoopIterations
	}
	if len(rc.Metadata) > 0 {
		if opts.Metadata == nil {
			opts.Metadata = make(map[string]string, len(rc.Metadata))
		}
		for key, value := range rc.Metadata {
			opts.Metadata[key] = value
		}
	}
	if len(rc.Tags) > 0 {
		opts.Tags = append([]string(nil), rc.Tags...)
	}
}

// EffectiveMaxReActIterations returns the RunConfig override if set,
// otherwise falls back to defaultVal.
func (rc *RunConfig) EffectiveMaxReActIterations(defaultVal int) int {
	if rc != nil && rc.MaxReActIterations != nil {
		return *rc.MaxReActIterations
	}
	return defaultVal
}

// EffectiveMaxLoopIterations returns the RunConfig override if set,
// otherwise falls back to defaultVal.
func (rc *RunConfig) EffectiveMaxLoopIterations(defaultVal int) int {
	if rc != nil && rc.MaxLoopIterations != nil {
		return *rc.MaxLoopIterations
	}
	return defaultVal
}

// ResolveRunConfig merges context-level config, Input.Context-derived config,
// and explicit input overrides into a single effective RunConfig.
func ResolveRunConfig(ctx context.Context, input *Input) *RunConfig {
	rc := GetRunConfig(ctx)
	if input == nil {
		return rc
	}
	rc = MergeRunConfig(rc, RunConfigFromInputContext(input.Context))
	rc = MergeRunConfig(rc, input.Overrides)
	return rc
}

// MergeRunConfig merges two RunConfigs, preserving base values unless override
// explicitly provides a replacement. The returned config is always a deep copy.
func MergeRunConfig(base *RunConfig, override *RunConfig) *RunConfig {
	switch {
	case base == nil && override == nil:
		return nil
	case base == nil:
		return cloneRunConfig(override)
	case override == nil:
		return cloneRunConfig(base)
	}

	merged := cloneRunConfig(base)
	if override.Model != nil {
		merged.Model = cloneStringPtr(override.Model)
	}
	if override.Provider != nil {
		merged.Provider = cloneStringPtr(override.Provider)
	}
	if override.RoutePolicy != nil {
		merged.RoutePolicy = cloneStringPtr(override.RoutePolicy)
	}
	if override.Temperature != nil {
		merged.Temperature = cloneFloat32Ptr(override.Temperature)
	}
	if override.MaxTokens != nil {
		merged.MaxTokens = cloneIntPtr(override.MaxTokens)
	}
	if override.TopP != nil {
		merged.TopP = cloneFloat32Ptr(override.TopP)
	}
	if len(override.Stop) > 0 {
		merged.Stop = append([]string(nil), override.Stop...)
	}
	if override.ToolChoice != nil {
		merged.ToolChoice = cloneStringPtr(override.ToolChoice)
	}
	if override.DisableTools {
		merged.DisableTools = true
		merged.ToolWhitelist = nil
	}
	if len(override.ToolWhitelist) > 0 {
		merged.ToolWhitelist = append([]string(nil), override.ToolWhitelist...)
		merged.DisableTools = false
	}
	if override.Timeout != nil {
		merged.Timeout = cloneDurationPtr(override.Timeout)
	}
	if override.MaxReActIterations != nil {
		merged.MaxReActIterations = cloneIntPtr(override.MaxReActIterations)
	}
	if override.MaxLoopIterations != nil {
		merged.MaxLoopIterations = cloneIntPtr(override.MaxLoopIterations)
	}
	if len(override.Metadata) > 0 {
		if merged.Metadata == nil {
			merged.Metadata = make(map[string]string, len(override.Metadata))
		}
		for k, v := range override.Metadata {
			merged.Metadata[k] = v
		}
	}
	if len(override.Tags) > 0 {
		merged.Tags = append([]string(nil), override.Tags...)
	}
	return merged
}

func cloneRunConfig(rc *RunConfig) *RunConfig {
	if rc == nil {
		return nil
	}
	out := *rc
	out.Model = cloneStringPtr(rc.Model)
	out.Provider = cloneStringPtr(rc.Provider)
	out.RoutePolicy = cloneStringPtr(rc.RoutePolicy)
	out.Temperature = cloneFloat32Ptr(rc.Temperature)
	out.MaxTokens = cloneIntPtr(rc.MaxTokens)
	out.TopP = cloneFloat32Ptr(rc.TopP)
	out.Stop = append([]string(nil), rc.Stop...)
	out.ToolChoice = cloneStringPtr(rc.ToolChoice)
	out.ToolWhitelist = append([]string(nil), rc.ToolWhitelist...)
	out.Timeout = cloneDurationPtr(rc.Timeout)
	out.MaxReActIterations = cloneIntPtr(rc.MaxReActIterations)
	out.MaxLoopIterations = cloneIntPtr(rc.MaxLoopIterations)
	out.Metadata = cloneStringMap(rc.Metadata)
	out.Tags = append([]string(nil), rc.Tags...)
	return &out
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat32Ptr(v *float32) *float32 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneIntPtr(v *int) *int {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneDurationPtr(v *time.Duration) *time.Duration {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// StringPtr returns a pointer to the given string.
func StringPtr(s string) *string { return &s }

// Float32Ptr returns a pointer to the given float32.
func Float32Ptr(f float32) *float32 { return &f }

// IntPtr returns a pointer to the given int.
func IntPtr(i int) *int { return &i }

// DurationPtr returns a pointer to the given time.Duration.
func DurationPtr(d time.Duration) *time.Duration { return &d }

// preparedRequest holds the fully-built ChatRequest together with provider
// references needed by the execution paths (streaming, ReAct, plain completion).
type preparedRequest struct {
	req          *types.ChatRequest
	chatProvider llm.Provider
	toolProvider llm.Provider // for ReAct loop (may equal chatProvider)
	hasTools     bool
	handoffTools map[string]RuntimeHandoffTarget
	toolRisks    map[string]string
	maxReActIter int
	maxLoopIter  int
	options      types.ExecutionOptions
}

// prepareChatRequest builds a ChatRequest from messages, applying context
// engineering, model selection, RunConfig overrides, route hints, and tool
// filtering. Both ChatCompletion and StreamCompletion delegate here so that
// the logic is maintained in a single place.
func (b *BaseAgent) prepareChatRequest(ctx context.Context, messages []types.Message) (*preparedRequest, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}
	if messages == nil || len(messages) == 0 {
		return nil, NewError(types.ErrInputValidation, "messages cannot be nil or empty")
	}

	chatProv := b.gatewayProvider()
	options := b.executionOptionsResolver().Resolve(ctx, b.config, nil)
	req, err := b.chatRequestAdapter().Build(options, messages)
	if err != nil {
		return nil, err
	}

	// 1. Tool whitelist filtering
	if b.toolManager != nil {
		allowedTools := b.toolManager.GetAllowedTools(b.config.Core.ID)
		switch {
		case options.Tools.DisableTools:
			req.Tools = nil
		case len(options.Tools.ToolWhitelist) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.ToolWhitelist)
		case len(options.Tools.AllowedTools) > 0:
			req.Tools = filterToolSchemasByWhitelist(allowedTools, options.Tools.AllowedTools)
		}
	}
	handoffMap := map[string]RuntimeHandoffTarget(nil)
	handoffTargets := runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID)
	if len(handoffTargets) > 0 {
		if len(req.Tools) == 0 {
			req.Tools = make([]types.ToolSchema, 0, len(handoffTargets))
		}
		handoffMap = make(map[string]RuntimeHandoffTarget, len(handoffTargets))
		seen := make(map[string]struct{}, len(req.Tools))
		for _, schema := range req.Tools {
			seen[schema.Name] = struct{}{}
		}
		for _, target := range handoffTargets {
			schema := runtimeHandoffToolSchema(target)
			handoffMap[schema.Name] = target
			if _, exists := seen[schema.Name]; exists {
				continue
			}
			seen[schema.Name] = struct{}{}
			req.Tools = append(req.Tools, schema)
		}
		if len(handoffMap) > 0 {
			if req.Metadata == nil {
				req.Metadata = make(map[string]string, 1)
			}
			req.Metadata["handoff_enabled"] = "true"
		}
	}

	// 2. 选择执行 provider。工具协议差异（如 XML fallback）统一在 llm/gateway 内处理。
	toolProv := chatProv
	if b.hasDedicatedToolExecutionSurface() {
		toolProv = b.gatewayToolProvider()
	}

	// 3. Effective loop budgets
	effectiveIter := options.Control.MaxReActIterations
	if effectiveIter <= 0 {
		effectiveIter = b.maxReActIterations()
	}
	toolRisks := make(map[string]string, len(req.Tools))
	for _, tool := range req.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		toolRisks[name] = classifyToolRiskByName(name)
	}

	return &preparedRequest{
		req:          req,
		chatProvider: chatProv,
		toolProvider: toolProv,
		hasTools:     len(req.Tools) > 0 && (b.toolManager != nil || len(handoffTargets) > 0),
		handoffTools: handoffMap,
		toolRisks:    toolRisks,
		maxReActIter: effectiveIter,
		maxLoopIter:  options.Control.MaxLoopIterations,
		options:      options,
	}, nil
}

// RunConfigFromInputContext extracts supported runtime overrides from Input.Context-style maps.
// Unknown keys are ignored.
func RunConfigFromInputContext(inputCtx map[string]any) *RunConfig {
	if len(inputCtx) == 0 {
		return nil
	}
	var rc RunConfig
	var hasOverride bool

	if value, ok := intOverrideFromContext(inputCtx, "max_react_iterations"); ok {
		rc.MaxReActIterations = IntPtr(value)
		hasOverride = true
	}
	if value, ok := intOverrideFromContext(inputCtx, "max_loop_iterations"); ok {
		rc.MaxLoopIterations = IntPtr(value)
		hasOverride = true
	}

	if !hasOverride {
		return nil
	}
	return &rc
}

func intOverrideFromContext(values map[string]any, key string) (int, bool) {
	if len(values) == 0 {
		return 0, false
	}
	raw, ok := values[key]
	if !ok {
		return 0, false
	}
	switch typed := raw.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case float32:
		return int(typed), true
	case float64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case json.Number:
		if value, err := typed.Int64(); err == nil {
			return int(value), true
		}
		if value, err := typed.Float64(); err == nil {
			return int(value), true
		}
		return 0, false
	case string:
		value, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return value, true
	default:
		return 0, false
	}
}

func boolOverrideFromContext(values map[string]any, key string) (bool, bool) {
	if len(values) == 0 {
		return false, false
	}
	raw, ok := values[key]
	if !ok {
		return false, false
	}
	switch typed := raw.(type) {
	case bool:
		return typed, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return false, false
		}
		parsed, err := strconv.ParseBool(text)
		if err != nil {
			return false, false
		}
		return parsed, true
	default:
		return false, false
	}
}

func parseBoolString(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func inputContext(input *Input) map[string]any {
	if input == nil {
		return nil
	}
	return input.Context
}

// lastUserQuery extracts the content of the last user message.
func lastUserQuery(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == types.RoleUser {
			return messages[i].Content
		}
	}
	return ""
}

// effectiveToolModel returns the tool-specific model if configured, otherwise
// falls back to the main model.
func effectiveToolModel(mainModel string, configuredToolModel string) string {
	if v := strings.TrimSpace(configuredToolModel); v != "" {
		return v
	}
	return mainModel
}

const (
	toolRiskSafeRead         = toolcap.ToolRiskSafeRead
	toolRiskRequiresApproval = toolcap.ToolRiskRequiresApproval
	toolRiskUnknown          = toolcap.ToolRiskUnknown
)

func classifyToolRiskByName(name string) string {
	return toolcap.ClassifyToolRiskByName(name)
}

func groupToolRisks(names []string) map[string][]string {
	return toolcap.GroupToolRisks(normalizeStringSlice(names))
}

// PreparedToolProtocol is the runtime-resolved tool execution bundle consumed
// by completion flows.
type PreparedToolProtocol struct {
	Executor     llmtools.ToolExecutor
	HandoffTools map[string]RuntimeHandoffTarget
	ToolRisks    map[string]string
	AllowedTools []string
}

// ToolProtocolRuntime resolves the tool execution contract for a prepared request.
type ToolProtocolRuntime interface {
	Prepare(owner *BaseAgent, pr *preparedRequest) *PreparedToolProtocol
	Execute(ctx context.Context, prepared *PreparedToolProtocol, calls []types.ToolCall) []types.ToolResult
	ToMessages(results []types.ToolResult) []types.Message
}

// DefaultToolProtocolRuntime preserves the current runtime behavior while
// centralizing handoff + tool manager orchestration behind a single interface.
type DefaultToolProtocolRuntime struct{}

func NewDefaultToolProtocolRuntime() ToolProtocolRuntime {
	return DefaultToolProtocolRuntime{}
}

func (DefaultToolProtocolRuntime) Prepare(owner *BaseAgent, pr *preparedRequest) *PreparedToolProtocol {
	if pr == nil || owner == nil {
		return &PreparedToolProtocol{
			Executor: toolManagerExecutor{},
		}
	}
	allowed := append([]string(nil), pr.options.Tools.AllowedTools...)
	base := newToolManagerExecutor(owner.toolManager, owner.config.Core.ID, allowed, owner.bus)
	executor := llmtools.ToolExecutor(base)
	if len(pr.handoffTools) > 0 {
		targets := make([]RuntimeHandoffTarget, 0, len(pr.handoffTools))
		for _, target := range pr.handoffTools {
			targets = append(targets, target)
		}
		executor = newRuntimeHandoffExecutor(owner, base, targets)
	}
	return &PreparedToolProtocol{
		Executor:     executor,
		HandoffTools: cloneRuntimeHandoffMap(pr.handoffTools),
		ToolRisks:    cloneStringMap(pr.toolRisks),
		AllowedTools: allowed,
	}
}

func (DefaultToolProtocolRuntime) Execute(ctx context.Context, prepared *PreparedToolProtocol, calls []types.ToolCall) []types.ToolResult {
	if prepared == nil || prepared.Executor == nil {
		return nil
	}
	return prepared.Executor.Execute(ctx, calls)
}

func (DefaultToolProtocolRuntime) ToMessages(results []types.ToolResult) []types.Message {
	if len(results) == 0 {
		return nil
	}
	out := make([]types.Message, 0, len(results))
	for _, result := range results {
		out = append(out, result.ToMessage())
	}
	return out
}

type runtimeHandoffTargetsKey struct{}
type runtimeConversationMessagesKey struct{}

const (
	internalContextHandoffMessages = "_agentflow_handoff_messages"
	internalContextParentHandoff   = "_agentflow_parent_handoff"
	internalContextFromAgentID     = "_agentflow_from_agent_id"
	internalContextHandoffTool     = "_agentflow_handoff_tool"
)

type RuntimeHandoffTarget struct {
	Agent       Agent
	ToolName    string
	Description string
}

type runtimeHandoffCallArgs struct {
	Input   string `json:"input,omitempty"`
	Task    string `json:"task,omitempty"`
	Message string `json:"message,omitempty"`
}

func WithRuntimeHandoffTargets(ctx context.Context, targets []RuntimeHandoffTarget) context.Context {
	filtered := cloneRuntimeHandoffTargets(targets)
	if len(filtered) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, runtimeHandoffTargetsKey{}, filtered)
}

func runtimeHandoffTargetsFromContext(ctx context.Context, currentAgentID string) []RuntimeHandoffTarget {
	if ctx == nil {
		return nil
	}
	raw, _ := ctx.Value(runtimeHandoffTargetsKey{}).([]RuntimeHandoffTarget)
	if len(raw) == 0 {
		return nil
	}
	currentAgentID = strings.TrimSpace(currentAgentID)
	out := make([]RuntimeHandoffTarget, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, target := range raw {
		if target.Agent == nil {
			continue
		}
		if currentAgentID != "" && strings.TrimSpace(target.Agent.ID()) == currentAgentID {
			continue
		}
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, RuntimeHandoffTarget{
			Agent:       target.Agent,
			ToolName:    name,
			Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		})
	}
	return out
}

func cloneRuntimeHandoffTargets(targets []RuntimeHandoffTarget) []RuntimeHandoffTarget {
	if len(targets) == 0 {
		return nil
	}
	out := make([]RuntimeHandoffTarget, 0, len(targets))
	seen := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target.Agent == nil {
			continue
		}
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, RuntimeHandoffTarget{
			Agent:       target.Agent,
			ToolName:    name,
			Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		})
	}
	return out
}

func cloneRuntimeHandoffMap(in map[string]RuntimeHandoffTarget) map[string]RuntimeHandoffTarget {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]RuntimeHandoffTarget, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func WithRuntimeConversationMessages(ctx context.Context, messages []types.Message) context.Context {
	if len(messages) == 0 {
		return ctx
	}
	if ctx == nil {
		ctx = context.Background()
	}
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	return context.WithValue(ctx, runtimeConversationMessagesKey{}, cloned)
}

func runtimeConversationMessagesFromContext(ctx context.Context) []types.Message {
	if ctx == nil {
		return nil
	}
	raw, _ := ctx.Value(runtimeConversationMessagesKey{}).([]types.Message)
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]types.Message, len(raw))
	copy(cloned, raw)
	return cloned
}

func runtimeHandoffToolName(override string, agentID string) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	s := strings.ToLower(strings.TrimSpace(agentID))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	if s == "" {
		s = "agent"
	}
	return "transfer_to_" + s
}

func runtimeHandoffToolDescription(override string, target Agent) string {
	if trimmed := strings.TrimSpace(override); trimmed != "" {
		return trimmed
	}
	if target == nil {
		return "Handoff to another agent to handle the request."
	}
	name := strings.TrimSpace(target.Name())
	if name == "" {
		name = strings.TrimSpace(target.ID())
	}
	if name == "" {
		return "Handoff to another agent to handle the request."
	}
	return fmt.Sprintf("Handoff to the %s agent to continue handling the request.", name)
}

func runtimeHandoffToolSchema(target RuntimeHandoffTarget) types.ToolSchema {
	return types.ToolSchema{
		Type:        types.ToolTypeFunction,
		Name:        runtimeHandoffToolName(target.ToolName, target.Agent.ID()),
		Description: runtimeHandoffToolDescription(target.Description, target.Agent),
		Parameters: json.RawMessage(`{
			"type":"object",
			"properties":{
				"input":{"type":"string","description":"Optional transfer prompt for the next agent."},
				"task":{"type":"string","description":"Optional concise task description for the next agent."},
				"message":{"type":"string","description":"Optional handoff note for the next agent."}
			},
			"additionalProperties":false
		}`),
	}
}

func handoffMessagesFromInputContext(values map[string]any) []types.Message {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values[internalContextHandoffMessages]
	if !ok {
		return nil
	}
	messages, ok := raw.([]types.Message)
	if !ok || len(messages) == 0 {
		return nil
	}
	cloned := make([]types.Message, len(messages))
	copy(cloned, messages)
	return cloned
}

func publicInputContext(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		switch key {
		case internalContextHandoffMessages, internalContextParentHandoff, internalContextFromAgentID, internalContextHandoffTool:
			continue
		case "memory_context", "retrieval_context", "tool_state", "skill_context", "checkpoint_id":
			continue
		default:
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type runtimeHandoffExecutor struct {
	base    toolManagerExecutor
	owner   *BaseAgent
	targets map[string]RuntimeHandoffTarget
}

func newRuntimeHandoffExecutor(owner *BaseAgent, base toolManagerExecutor, targets []RuntimeHandoffTarget) *runtimeHandoffExecutor {
	targetMap := make(map[string]RuntimeHandoffTarget, len(targets))
	for _, target := range targets {
		name := runtimeHandoffToolName(target.ToolName, target.Agent.ID())
		target.ToolName = name
		target.Description = runtimeHandoffToolDescription(target.Description, target.Agent)
		targetMap[name] = target
	}
	return &runtimeHandoffExecutor{
		base:    base,
		owner:   owner,
		targets: targetMap,
	}
}

func (e *runtimeHandoffExecutor) Execute(ctx context.Context, calls []types.ToolCall) []types.ToolResult {
	if len(calls) == 0 {
		return nil
	}
	results := make([]types.ToolResult, 0, len(calls))
	for _, call := range calls {
		results = append(results, e.ExecuteOne(ctx, call))
	}
	return results
}

func (e *runtimeHandoffExecutor) ExecuteOne(ctx context.Context, call types.ToolCall) types.ToolResult {
	if target, ok := e.targets[call.Name]; ok {
		return e.executeRuntimeHandoff(ctx, target, call)
	}
	return e.base.ExecuteOne(ctx, call)
}

func (e *runtimeHandoffExecutor) ExecuteOneStream(ctx context.Context, call types.ToolCall) <-chan llmtools.ToolStreamEvent {
	ch := make(chan llmtools.ToolStreamEvent, 1)
	go func() {
		defer close(ch)
		result := e.ExecuteOne(ctx, call)
		if result.Error != "" {
			ch <- llmtools.ToolStreamEvent{
				Type:     llmtools.ToolStreamError,
				ToolName: call.Name,
				Error:    fmt.Errorf("%s", result.Error),
			}
			return
		}
		ch <- llmtools.ToolStreamEvent{
			Type:     llmtools.ToolStreamComplete,
			ToolName: call.Name,
			Data:     result,
		}
	}()
	return ch
}

func (e *runtimeHandoffExecutor) executeRuntimeHandoff(ctx context.Context, target RuntimeHandoffTarget, call types.ToolCall) types.ToolResult {
	if e.owner == nil || target.Agent == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "handoff target is not configured"}
	}

	manager := agenthandoff.NewHandoffManager(e.owner.logger)
	manager.RegisterAgent(runtimeHandoffAgentAdapter{agent: target.Agent})

	taskInput, taskDescription := parseRuntimeHandoffCall(call)
	conversationMessages := runtimeConversationMessagesFromContext(ctx)
	if len(conversationMessages) > 0 {
		conversationMessages = append(conversationMessages, types.Message{
			Role:      types.RoleAssistant,
			ToolCalls: []types.ToolCall{call},
		})
	}

	ho, err := manager.Handoff(ctx, agenthandoff.HandoffOptions{
		FromAgentID: e.owner.ID(),
		ToAgentID:   target.Agent.ID(),
		Task: agenthandoff.Task{
			Type:        "agent_handoff",
			Description: taskDescription,
			Input:       taskInput,
			Metadata: map[string]any{
				"tool_call_id": call.ID,
				"tool_name":    call.Name,
			},
		},
		Context: agenthandoff.HandoffContext{
			ConversationID: e.owner.ID(),
			Messages:       conversationMessages,
		},
		Wait:                    true,
		ToolNameOverride:        target.ToolName,
		ToolDescriptionOverride: target.Description,
		OnHandoff: func(ctx context.Context, handoff *agenthandoff.Handoff) error {
			emitRuntimeHandoffAgentUpdated(ctx, target.Agent, handoff)
			return nil
		},
	})
	if err != nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: err.Error()}
	}
	if ho == nil || ho.Result == nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: "handoff completed without a result"}
	}
	if ho.Result.Error != "" {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: ho.Result.Error}
	}

	payload := types.ToolResultControl{
		Type: types.ToolResultControlTypeHandoff,
		Handoff: &types.ToolResultHandoff{
			HandoffID:       ho.ID,
			FromAgentID:     ho.FromAgentID,
			ToAgentID:       ho.ToAgentID,
			ToAgentName:     target.Agent.Name(),
			TransferMessage: ho.TransferMessage,
			Output:          fmt.Sprintf("%v", ho.Result.Output),
		},
	}
	if runtimePayload, ok := ho.Result.Output.(runtimeHandoffOutput); ok {
		payload.Handoff.Output = runtimePayload.Content
		payload.Handoff.Metadata = runtimePayload.Metadata
		payload.Handoff.Provider = runtimePayload.Provider
		payload.Handoff.Model = runtimePayload.Model
		payload.Handoff.TokensUsed = runtimePayload.TokensUsed
		payload.Handoff.FinishReason = runtimePayload.FinishReason
		payload.Handoff.ReasoningContent = runtimePayload.ReasoningContent
	}
	raw, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return types.ToolResult{ToolCallID: call.ID, Name: call.Name, Error: marshalErr.Error()}
	}

	return types.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Result:     raw,
	}
}

func parseRuntimeHandoffCall(call types.ToolCall) (string, string) {
	var args runtimeHandoffCallArgs
	if len(call.Arguments) > 0 {
		_ = json.Unmarshal(call.Arguments, &args)
	}
	input := strings.TrimSpace(firstNonEmpty(args.Input, args.Task, args.Message, call.Input))
	if input == "" {
		input = fmt.Sprintf("Continue handling the request via %s.", call.Name)
	}
	description := strings.TrimSpace(firstNonEmpty(args.Task, args.Message, input))
	if description == "" {
		description = input
	}
	return input, description
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type runtimeHandoffAgentAdapter struct {
	agent Agent
}

func (a runtimeHandoffAgentAdapter) ID() string { return a.agent.ID() }

func (a runtimeHandoffAgentAdapter) Capabilities() []agenthandoff.AgentCapability {
	return []agenthandoff.AgentCapability{{
		Name:      a.agent.Name(),
		TaskTypes: []string{"agent_handoff"},
		Priority:  1,
	}}
}

func (a runtimeHandoffAgentAdapter) CanHandle(agenthandoff.Task) bool { return true }
func (a runtimeHandoffAgentAdapter) AcceptHandoff(context.Context, *agenthandoff.Handoff) error {
	return nil
}

func (a runtimeHandoffAgentAdapter) ExecuteHandoff(ctx context.Context, ho *agenthandoff.Handoff) (*agenthandoff.HandoffResult, error) {
	traceID, _ := types.TraceID(ctx)
	input := &Input{
		TraceID:   traceID,
		ChannelID: ho.Context.ConversationID,
		Content:   strings.TrimSpace(fmt.Sprintf("%v", ho.Task.Input)),
		Context: map[string]any{
			internalContextHandoffMessages: ho.Context.Messages,
			internalContextParentHandoff:   ho.ID,
			internalContextFromAgentID:     ho.FromAgentID,
			internalContextHandoffTool:     ho.ToolName,
		},
	}
	if input.Content == "" {
		input.Content = ho.Task.Description
	}
	output, err := a.agent.Execute(ctx, input)
	if err != nil {
		return nil, err
	}
	if output == nil {
		return &agenthandoff.HandoffResult{Output: ""}, nil
	}
	return &agenthandoff.HandoffResult{
		Output: runtimeHandoffOutput{
			Content:          output.Content,
			Metadata:         cloneMetadata(output.Metadata),
			TokensUsed:       output.TokensUsed,
			FinishReason:     output.FinishReason,
			ReasoningContent: output.ReasoningContent,
		},
	}, nil
}

type runtimeHandoffOutput struct {
	Content          string         `json:"content"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	TokensUsed       int            `json:"tokens_used,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	ReasoningContent *string        `json:"reasoning_content,omitempty"`
}

func emitRuntimeHandoffAgentUpdated(ctx context.Context, target Agent, handoff *agenthandoff.Handoff) {
	emit, ok := runtimeStreamEmitterFromContext(ctx)
	if !ok || target == nil {
		return
	}
	emit(RuntimeStreamEvent{
		Type:         RuntimeStreamStatus,
		SDKEventType: SDKAgentUpdatedEvent,
		Timestamp:    time.Now(),
		CurrentStage: "handoff",
		Data: map[string]any{
			"status": "agent_updated",
			"new_agent": map[string]any{
				"id":   target.ID(),
				"name": target.Name(),
				"type": target.Type(),
			},
			"handoff_id":       handoff.ID,
			"from_agent_id":    handoff.FromAgentID,
			"to_agent_id":      handoff.ToAgentID,
			"transfer_message": handoff.TransferMessage,
		},
	})
}

// Merged from ephemeral_prompt.go.
type EphemeralPromptLayerBuilder = agentcontext.EphemeralPromptLayerBuilder
type EphemeralPromptLayerInput = agentcontext.EphemeralPromptLayerInput

func NewEphemeralPromptLayerBuilder() *EphemeralPromptLayerBuilder {
	return agentcontext.NewEphemeralPromptLayerBuilder()
}

type TraceFeedbackAction = agentcontext.TraceFeedbackAction
type TraceFeedbackSignals = agentcontext.TraceFeedbackSignals
type TraceFeedbackPlan = agentcontext.TraceFeedbackPlan
type TraceFeedbackConfig = agentcontext.TraceFeedbackConfig

const (
	TraceFeedbackSkip               = agentcontext.TraceFeedbackSkip
	TraceFeedbackSynopsisOnly       = agentcontext.TraceFeedbackSynopsisOnly
	TraceFeedbackHistoryOnly        = agentcontext.TraceFeedbackHistoryOnly
	TraceFeedbackMemoryRecallOnly   = agentcontext.TraceFeedbackMemoryRecallOnly
	TraceFeedbackSynopsisAndHistory = agentcontext.TraceFeedbackSynopsisAndHistory
)

type TraceFeedbackPlanner = agentcontext.TraceFeedbackPlanner
type TraceFeedbackPlanAdapter = agentcontext.TraceFeedbackPlanAdapter

func NewRuleBasedTraceFeedbackPlanner() TraceFeedbackPlanner {
	return agentcontext.NewRuleBasedTraceFeedbackPlanner()
}

func NewComposedTraceFeedbackPlanner(base TraceFeedbackPlanner, adapters ...TraceFeedbackPlanAdapter) TraceFeedbackPlanner {
	return agentcontext.NewComposedTraceFeedbackPlanner(base, adapters...)
}

func NewHintTraceFeedbackAdapter() TraceFeedbackPlanAdapter {
	return agentcontext.NewHintTraceFeedbackAdapter()
}

func DefaultTraceFeedbackConfig() TraceFeedbackConfig {
	return agentcontext.DefaultTraceFeedbackConfig()
}

func TraceFeedbackConfigFromAgentConfig(cfg types.AgentConfig) TraceFeedbackConfig {
	return agentcontext.TraceFeedbackConfigFromAgentConfig(cfg)
}

func collectTraceFeedbackSignals(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, hasMemoryRuntime bool) TraceFeedbackSignals {
	var acceptanceCriteriaCount int
	var handoff bool
	if input != nil {
		acceptanceCriteriaCount = len(normalizeStringSlice(acceptanceCriteriaForValidation(input, nil)))
		handoff = len(handoffMessagesFromInputContext(input.Context)) > 0
	}
	return agentcontext.CollectTraceFeedbackSignals(agentcontext.CollectTraceFeedbackSignalsInput{
		UserInputContext:        inputContext(input),
		Snapshot:                agentcontext.ExplainabilitySynopsisSnapshot(snapshot),
		HasMemoryRuntime:        hasMemoryRuntime,
		ContextStatus:           status,
		AcceptanceCriteriaCount: acceptanceCriteriaCount,
		Handoff:                 handoff,
	})
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// Merged from remote_tool_transport.go.
type RemoteToolTargetKind = toolcap.RemoteToolTargetKind

const (
	RemoteToolTargetHTTP  = toolcap.RemoteToolTargetHTTP
	RemoteToolTargetMCP   = toolcap.RemoteToolTargetMCP
	RemoteToolTargetA2A   = toolcap.RemoteToolTargetA2A
	RemoteToolTargetStdio = toolcap.RemoteToolTargetStdio
)

type RemoteToolTarget = toolcap.RemoteToolTarget
type ToolInvocationRequest = toolcap.ToolInvocationRequest
type ToolInvocationResult = toolcap.ToolInvocationResult
type RemoteToolTransport = toolcap.RemoteToolTransport

func NewDefaultRemoteToolTransport(logger *zap.Logger) RemoteToolTransport {
	return toolcap.NewDefaultRemoteToolTransport(logger)
}

// Merged from completion.go.

// DefaultStreamInactivityTimeout 是流式响应的默认空闲超时时间。
// 只要还在收到数据，就不会超时；只有在超过此时间没有新数据时才触发超时。
const DefaultStreamInactivityTimeout = 5 * time.Minute

// ChatCompletion 调用 LLM 完成对话。
func (b *BaseAgent) ChatCompletion(ctx context.Context, messages []types.Message) (*types.ChatResponse, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	emit, streaming := runtimeStreamEmitterFromContext(ctx)
	if streaming {
		return b.chatCompletionStreaming(ctx, pr, emit)
	}

	if pr.hasTools {
		return b.chatCompletionWithTools(ctx, pr)
	}

	return pr.chatProvider.Completion(ctx, pr.req)
}

// chatCompletionStreaming handles the streaming execution path of ChatCompletion.
// 支持 Steering：通过 context 中的 SteeringChannel 接收实时引导/停止后发送指令。
func (b *BaseAgent) chatCompletionStreaming(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter) (*types.ChatResponse, error) {
	steerCh, _ := SteeringChannelFromContext(ctx)
	reactIterationBudget := reactToolLoopBudget(pr)
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)

	if pr.hasTools {
		return b.chatCompletionStreamingWithTools(ctx, pr, emit, steerCh, reactIterationBudget)
	}
	return b.chatCompletionStreamingDirect(ctx, pr, emit, steerCh)
}

type reactStreamingState struct {
	final            *types.ChatResponse
	currentIteration int
	selectedMode     string
}

func (b *BaseAgent) chatCompletionStreamingWithTools(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel, reactIterationBudget int) (*types.ChatResponse, error) {
	state, eventCh, err := b.startReactStreaming(ctx, pr, steerCh, reactIterationBudget, emit)
	if err != nil {
		return nil, err
	}
	for ev := range eventCh {
		if err := b.handleReactStreamEvent(emit, pr, state, ev); err != nil {
			return nil, err
		}
	}
	if state.final == nil {
		return nil, ErrNoResponse
	}
	return state.final, nil
}

func (b *BaseAgent) startReactStreaming(ctx context.Context, pr *preparedRequest, steerCh *SteeringChannel, reactIterationBudget int, emit RuntimeStreamEmitter) (*reactStreamingState, <-chan llmtools.ReActStreamEvent, error) {
	const selectedMode = ReasoningModeReact
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, pr.options.Tools.ToolModel)
	ctx = withRuntimeApprovalEmitter(ctx, emit, pr)
	toolProtocol := b.toolProtocolRuntime().Prepare(b, pr)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolProtocol.Executor,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	if steerCh != nil {
		executor.SetSteeringChannel(steerCh.Receive())
	}
	eventCh, err := executor.ExecuteStream(ctx, &reactReq)
	if err != nil {
		return nil, nil, err
	}
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "reasoning",
		IterationCount: 0,
		SelectedMode:   selectedMode,
		Data: map[string]any{
			"mode":                   selectedMode,
			"react_iteration_budget": reactIterationBudget,
		},
	})
	return &reactStreamingState{selectedMode: selectedMode}, eventCh, nil
}

func (b *BaseAgent) handleReactStreamEvent(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, ev llmtools.ReActStreamEvent) error {
	switch ev.Type {
	case llmtools.ReActEventIterationStart:
		state.currentIteration = ev.Iteration
	case llmtools.ReActEventLLMChunk:
		emitReactLLMChunk(emit, state, ev)
	case llmtools.ReActEventToolsStart:
		emitReactToolCalls(emit, pr, state, ev.ToolCalls)
	case llmtools.ReActEventToolsEnd:
		emitReactToolResults(emit, pr, state, ev.ToolResults)
	case llmtools.ReActEventToolProgress:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolProgress,
			Timestamp:      time.Now(),
			ToolCallID:     ev.ToolCallID,
			ToolName:       ev.ToolName,
			Data:           ev.ProgressData,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventSteering:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: ev.SteeringContent,
			CurrentStage:    "reasoning",
			IterationCount:  state.currentIteration,
			SelectedMode:    state.selectedMode,
		})
	case llmtools.ReActEventStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	case llmtools.ReActEventCompleted:
		state.final = ev.FinalResponse
		emitReactCompletion(emit, state)
	case llmtools.ReActEventError:
		stopReason := string(classifyStopReason(ev.Error))
		emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
		return NewErrorWithCause(types.ErrAgentExecution, "streaming execution error", errors.New(ev.Error))
	}
	return nil
}

func emitReactLLMChunk(emit RuntimeStreamEmitter, state *reactStreamingState, ev llmtools.ReActStreamEvent) {
	if ev.Chunk == nil {
		return
	}
	if ev.Chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          ev.Chunk.Delta.Content,
			Delta:          ev.Chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
	if ev.Chunk.Delta.ReasoningContent != nil && *ev.Chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *ev.Chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "reasoning",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
		})
	}
}

func emitReactToolCalls(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, calls []types.ToolCall) {
	for _, call := range calls {
		sdkEventName := SDKToolCalled
		if runtimeHandoffToolRequested(pr, call.Name) {
			sdkEventName = SDKHandoffRequested
		}
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolCall,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolCall: &RuntimeToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: append(json.RawMessage(nil), call.Arguments...),
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func emitReactToolResults(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, results []types.ToolResult) {
	for _, tr := range results {
		sdkEventName, resultPayload := reactToolResultPayload(pr, tr)
		emitApprovalRuntimeEventFromToolResult(emit, pr, state, tr)
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToolResult,
			Timestamp:      time.Now(),
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			ToolResult: &RuntimeToolResult{
				ToolCallID: tr.ToolCallID,
				Name:       tr.Name,
				Result:     resultPayload,
				Error:      tr.Error,
				Duration:   tr.Duration,
			},
			SDKEventType: SDKRunItemEvent,
			SDKEventName: sdkEventName,
		})
	}
}

func withRuntimeApprovalEmitter(ctx context.Context, emit RuntimeStreamEmitter, pr *preparedRequest) context.Context {
	if emit == nil {
		return ctx
	}
	return llmtools.WithPermissionEventEmitter(ctx, func(event llmtools.PermissionEvent) {
		emitRuntimeApprovalEvent(emit, pr, event)
	})
}

func withApprovalExplainabilityEmitter(ctx context.Context, recorder ExplainabilityRecorder, traceID string) context.Context {
	if recorder == nil || strings.TrimSpace(traceID) == "" {
		return ctx
	}
	return llmtools.WithPermissionEventEmitter(ctx, func(event llmtools.PermissionEvent) {
		content := strings.TrimSpace(event.Reason)
		if content == "" {
			content = string(event.Type)
		}
		metadata := map[string]any{
			"approval_type": event.Type,
			"approval_id":   event.ApprovalID,
			"decision":      string(event.Decision),
			"tool_name":     event.ToolName,
			"rule_id":       event.RuleID,
		}
		if len(event.Metadata) > 0 {
			for key, value := range event.Metadata {
				metadata[key] = value
			}
		}
		recorder.AddExplainabilityStep(traceID, "approval", content, metadata)
		if timelineRecorder, ok := recorder.(ExplainabilityTimelineRecorder); ok {
			timelineRecorder.AddExplainabilityTimeline(traceID, "approval", content, metadata)
		}
	})
}

func emitRuntimeApprovalEvent(emit RuntimeStreamEmitter, pr *preparedRequest, event llmtools.PermissionEvent) {
	if emit == nil {
		return
	}
	sdkEventName := SDKApprovalResponse
	if event.Type == llmtools.PermissionEventRequested {
		sdkEventName = SDKApprovalRequested
	}
	data := map[string]any{
		"approval_type": event.Type,
		"decision":      string(event.Decision),
		"reason":        event.Reason,
		"approval_id":   event.ApprovalID,
	}
	if len(event.Metadata) > 0 {
		for key, value := range event.Metadata {
			data[key] = value
		}
	}
	if risk := toolRiskForPreparedRequest(pr, event.ToolName, event.Metadata); risk != "" {
		data["hosted_tool_risk"] = risk
	}
	emit(RuntimeStreamEvent{
		Type:         RuntimeStreamApproval,
		SDKEventType: SDKRunItemEvent,
		SDKEventName: sdkEventName,
		Timestamp:    time.Now(),
		ToolName:     event.ToolName,
		Data:         data,
	})
}

func emitApprovalRuntimeEventFromToolResult(emit RuntimeStreamEmitter, pr *preparedRequest, state *reactStreamingState, tr types.ToolResult) {
	if emit == nil {
		return
	}
	risk := toolRiskForPreparedRequest(pr, tr.Name, nil)
	if risk != toolRiskRequiresApproval {
		return
	}
	approvalID, required := parseApprovalRequiredError(tr.Error)
	if required {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamApproval,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKApprovalRequested,
			Timestamp:      time.Now(),
			ToolName:       tr.Name,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"approval_type":    "approval_requested",
				"approval_id":      approvalID,
				"hosted_tool_risk": risk,
				"reason":           tr.Error,
			},
		})
		return
	}
	if tr.Error == "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamApproval,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKApprovalResponse,
			Timestamp:      time.Now(),
			ToolName:       tr.Name,
			CurrentStage:   "acting",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"approval_type":    "approval_granted",
				"approved":         true,
				"hosted_tool_risk": risk,
			},
		})
	}
}

func parseApprovalRequiredError(errText string) (string, bool) {
	trimmed := strings.TrimSpace(errText)
	if !strings.HasPrefix(trimmed, "approval required") {
		return "", false
	}
	const prefix = "approval required (ID: "
	if strings.HasPrefix(trimmed, prefix) {
		rest := strings.TrimPrefix(trimmed, prefix)
		if idx := strings.Index(rest, ")"); idx >= 0 {
			return strings.TrimSpace(rest[:idx]), true
		}
	}
	return "", true
}

func toolRiskForPreparedRequest(pr *preparedRequest, toolName string, metadata map[string]string) string {
	if metadata != nil {
		if risk := strings.TrimSpace(metadata["hosted_tool_risk"]); risk != "" {
			return risk
		}
	}
	if pr != nil && len(pr.toolRisks) > 0 {
		if risk, ok := pr.toolRisks[strings.TrimSpace(toolName)]; ok {
			return risk
		}
	}
	return classifyToolRiskByName(toolName)
}

func reactToolResultPayload(pr *preparedRequest, tr types.ToolResult) (SDKRunItemEventName, json.RawMessage) {
	sdkEventName := SDKToolOutput
	resultPayload := append(json.RawMessage(nil), tr.Result...)
	if runtimeHandoffToolRequested(pr, tr.Name) {
		sdkEventName = SDKHandoffOccured
		if control := tr.Control(); control != nil && control.Handoff != nil {
			if raw, err := json.Marshal(control.Handoff); err == nil {
				resultPayload = raw
			}
		}
	}
	return sdkEventName, resultPayload
}

func emitReactCompletion(emit RuntimeStreamEmitter, state *reactStreamingState) {
	final := state.final
	if emit != nil && final != nil && len(final.Choices) > 0 && final.Choices[0].Message.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: state.currentIteration,
			SelectedMode:   state.selectedMode,
			Data: map[string]any{
				"content": final.Choices[0].Message.Content,
			},
		})
	}
	stopReason := normalizeRuntimeStopReasonFromResponse(final)
	emitCompletionLoopStatus(emit, state.currentIteration, state.selectedMode, stopReason)
}

type directStreamingAttemptResult struct {
	assembled        types.Message
	lastID           string
	lastProvider     string
	lastModel        string
	lastUsage        *types.ChatUsage
	lastFinishReason string
	reasoning        string
	steering         *SteeringMessage
}

func (b *BaseAgent) chatCompletionStreamingDirect(ctx context.Context, pr *preparedRequest, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*types.ChatResponse, error) {
	messages := append([]types.Message(nil), pr.req.Messages...)
	var cumulativeUsage types.ChatUsage
	emitRuntimeStatus(emit, "reasoning_mode_selected", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "responding",
		IterationCount: 1,
	})

	for {
		attempt, err := b.runDirectStreamingAttempt(ctx, pr, messages, emit, steerCh)
		if err != nil {
			return nil, err
		}
		accumulateChatUsage(&cumulativeUsage, attempt.lastUsage)
		if attempt.steering == nil || attempt.steering.IsZero() {
			return finalizeDirectStreamingResponse(emit, attempt, cumulativeUsage), nil
		}
		emitDirectSteeringEvent(emit, attempt.steering)
		messages = types.ApplySteeringToMessages(*attempt.steering, messages, attempt.assembled.Content, attempt.reasoning, types.RoleAssistant)
	}
}

func (b *BaseAgent) runDirectStreamingAttempt(ctx context.Context, pr *preparedRequest, messages []types.Message, emit RuntimeStreamEmitter, steerCh *SteeringChannel) (*directStreamingAttemptResult, error) {
	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	pr.req.Messages = messages
	streamCh, err := pr.chatProvider.Stream(streamCtx, pr.req)
	if err != nil {
		return nil, err
	}
	result := &directStreamingAttemptResult{}
	var reasoningBuf strings.Builder

	inactivityTimer := time.NewTimer(DefaultStreamInactivityTimeout)
	defer inactivityTimer.Stop()

chunkLoop:
	for {
		select {
		case chunk, ok := <-streamCh:
			if !ok {
				break chunkLoop
			}
			if !inactivityTimer.Stop() {
				select {
				case <-inactivityTimer.C:
				default:
				}
			}
			inactivityTimer.Reset(DefaultStreamInactivityTimeout)

			if chunk.Err != nil {
				return nil, chunk.Err
			}
			consumeDirectStreamChunk(emit, result, &reasoningBuf, chunk)
		case msg := <-steerChOrNil(steerCh):
			result.steering = &msg
			cancelStream()
			for range streamCh {
			}
			break chunkLoop
		case <-inactivityTimer.C:
			cancelStream()
			b.logger.Warn("stream inactivity timeout",
				zap.Duration("timeout", DefaultStreamInactivityTimeout),
			)
			return nil, NewError(types.ErrAgentExecution, "stream inactivity timeout after "+DefaultStreamInactivityTimeout.String()+" (no data received)")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	result.reasoning = reasoningBuf.String()
	return result, nil
}

func consumeDirectStreamChunk(emit RuntimeStreamEmitter, result *directStreamingAttemptResult, reasoningBuf *strings.Builder, chunk types.StreamChunk) {
	if chunk.ID != "" {
		result.lastID = chunk.ID
	}
	if chunk.Provider != "" {
		result.lastProvider = chunk.Provider
	}
	if chunk.Model != "" {
		result.lastModel = chunk.Model
	}
	if chunk.Usage != nil {
		result.lastUsage = chunk.Usage
	}
	if chunk.FinishReason != "" {
		result.lastFinishReason = chunk.FinishReason
	}
	if chunk.Delta.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamToken,
			Timestamp:      time.Now(),
			Token:          chunk.Delta.Content,
			Delta:          chunk.Delta.Content,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		result.assembled.Content += chunk.Delta.Content
	}
	if chunk.Delta.ReasoningContent != nil && *chunk.Delta.ReasoningContent != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamReasoning,
			Timestamp:      time.Now(),
			Reasoning:      *chunk.Delta.ReasoningContent,
			SDKEventType:   SDKRawResponseEvent,
			CurrentStage:   "responding",
			IterationCount: 1,
		})
		reasoningBuf.WriteString(*chunk.Delta.ReasoningContent)
	}
	if len(chunk.Delta.ReasoningSummaries) > 0 {
		result.assembled.ReasoningSummaries = append(result.assembled.ReasoningSummaries, chunk.Delta.ReasoningSummaries...)
	}
	if len(chunk.Delta.OpaqueReasoning) > 0 {
		result.assembled.OpaqueReasoning = append(result.assembled.OpaqueReasoning, chunk.Delta.OpaqueReasoning...)
	}
	if len(chunk.Delta.ThinkingBlocks) > 0 {
		result.assembled.ThinkingBlocks = append(result.assembled.ThinkingBlocks, chunk.Delta.ThinkingBlocks...)
	}
}

func accumulateChatUsage(total, usage *types.ChatUsage) {
	if usage == nil || total == nil {
		return
	}
	total.PromptTokens += usage.PromptTokens
	total.CompletionTokens += usage.CompletionTokens
	total.TotalTokens += usage.TotalTokens
}

func finalizeDirectStreamingResponse(emit RuntimeStreamEmitter, attempt *directStreamingAttemptResult, cumulativeUsage types.ChatUsage) *types.ChatResponse {
	if attempt.reasoning != "" {
		rc := attempt.reasoning
		attempt.assembled.ReasoningContent = &rc
	}
	attempt.assembled.Role = types.RoleAssistant
	resp := &types.ChatResponse{
		ID:       attempt.lastID,
		Provider: attempt.lastProvider,
		Model:    attempt.lastModel,
		Choices: []types.ChatChoice{{
			Index:        0,
			FinishReason: attempt.lastFinishReason,
			Message:      attempt.assembled,
		}},
		Usage: cumulativeUsage,
	}
	if emit != nil && attempt.assembled.Content != "" {
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStatus,
			SDKEventType:   SDKRunItemEvent,
			SDKEventName:   SDKMessageOutputCreated,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
			Data: map[string]any{
				"content": attempt.assembled.Content,
			},
		})
	}
	emitCompletionLoopStatus(emit, 1, "", normalizeRuntimeStopReason(attempt.lastFinishReason))
	return resp
}

func emitDirectSteeringEvent(emit RuntimeStreamEmitter, steering *SteeringMessage) {
	switch steering.Type {
	case SteeringTypeGuide:
		emit(RuntimeStreamEvent{
			Type:            RuntimeStreamSteering,
			Timestamp:       time.Now(),
			SteeringContent: steering.Content,
			CurrentStage:    "responding",
			IterationCount:  1,
		})
	case SteeringTypeStopAndSend:
		emit(RuntimeStreamEvent{
			Type:           RuntimeStreamStopAndSend,
			Timestamp:      time.Now(),
			CurrentStage:   "responding",
			IterationCount: 1,
		})
	}
}

// chatCompletionWithTools executes a non-streaming ReAct loop with tools.
func (b *BaseAgent) chatCompletionWithTools(ctx context.Context, pr *preparedRequest) (*types.ChatResponse, error) {
	ctx = WithRuntimeConversationMessages(ctx, pr.req.Messages)
	reactReq := *pr.req
	reactReq.Model = effectiveToolModel(pr.req.Model, pr.options.Tools.ToolModel)
	reactIterationBudget := reactToolLoopBudget(pr)
	toolProtocol := b.toolProtocolRuntime().Prepare(b, pr)
	executor := llmtools.NewReActExecutor(
		pr.toolProvider,
		toolProtocol.Executor,
		llmtools.ReActConfig{MaxIterations: reactIterationBudget, StopOnError: false},
		b.logger,
	)
	resp, _, err := executor.Execute(ctx, &reactReq)
	if err != nil {
		return resp, NewErrorWithCause(types.ErrAgentExecution, "ReAct execution failed", err)
	}
	return resp, nil
}

func reactToolLoopBudget(pr *preparedRequest) int {
	if pr != nil && pr.maxReActIter > 0 {
		return pr.maxReActIter
	}
	return 1
}

// StreamCompletion 流式调用 LLM。
func (b *BaseAgent) StreamCompletion(ctx context.Context, messages []types.Message) (<-chan types.StreamChunk, error) {
	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	return pr.chatProvider.Stream(ctx, pr.req)
}

func emitCompletionLoopStatus(emit RuntimeStreamEmitter, iteration int, selectedMode, stopReason string) {
	normalizedStopReason := normalizeTopLevelStopReason(stopReason, stopReason)
	emitRuntimeStatus(emit, "completion_judge_decision", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "evaluate",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"decision":            "done",
			"solved":              normalizedStopReason == string(StopReasonSolved),
			"internal_stop_cause": stopReason,
		},
	})
	emitRuntimeStatus(emit, "loop_stopped", RuntimeStreamEvent{
		Timestamp:      time.Now(),
		CurrentStage:   "completed",
		IterationCount: iteration,
		SelectedMode:   selectedMode,
		StopReason:     normalizedStopReason,
		Data: map[string]any{
			"state":               "stopped",
			"internal_stop_cause": stopReason,
		},
	})
}

func normalizeRuntimeStopReasonFromResponse(resp *types.ChatResponse) string {
	if resp == nil || len(resp.Choices) == 0 {
		return normalizeRuntimeStopReason("")
	}
	return normalizeRuntimeStopReason(resp.Choices[0].FinishReason)
}

func normalizeRuntimeStopReason(finishReason string) string {
	normalized := strings.TrimSpace(finishReason)
	if normalized == "" {
		return string(StopReasonSolved)
	}
	return normalizeTopLevelStopReason(normalized, normalized)
}

func runtimeHandoffTargetsFromPreparedRequest(pr *preparedRequest) []RuntimeHandoffTarget {
	if pr == nil || len(pr.handoffTools) == 0 {
		return nil
	}
	out := make([]RuntimeHandoffTarget, 0, len(pr.handoffTools))
	for _, target := range pr.handoffTools {
		out = append(out, target)
	}
	return out
}

func runtimeHandoffToolRequested(pr *preparedRequest, toolName string) bool {
	if pr == nil || len(pr.handoffTools) == 0 {
		return false
	}
	_, ok := pr.handoffTools[toolName]
	return ok
}

// Merged from react.go.

// defaultCostCalc is a package-level cost calculator for estimating LLM call costs.
var defaultCostCalc = observability.NewCostCalculator()

const submitNumberedPlanTool = planningcap.SubmitNumberedPlanTool

// Plan 生成执行计划。
// 使用 LLM 分析任务并生成详细的执行步骤。
func (b *BaseAgent) Plan(ctx context.Context, input *Input) (*PlanResult, error) {
	if !b.hasMainExecutionSurface() {
		return nil, ErrProviderNotSet
	}

	planPrompt := fmt.Sprintf(`Plan the execution of this task for another agent.

Task:
%s

Use the %s tool to return the plan.

Requirements:
- Keep each step directly executable
- Prefer tool-first actions when tools are needed
- Mention dependencies or risks only when they affect execution
- Do not answer with prose outside the tool call`, input.Content, planningcap.SubmitNumberedPlanTool)

	messages := []types.Message{
		{
			Role:    types.RoleSystem,
			Content: b.promptBundle.RenderSystemPromptWithVars(input.Variables),
		},
		{
			Role:    types.RoleUser,
			Content: planPrompt,
		},
	}

	pr, err := b.prepareChatRequest(ctx, messages)
	if err != nil {
		return nil, err
	}
	nativeToolSupport := pr.chatProvider.SupportsNativeFunctionCalling()
	if !nativeToolSupport && b.mainProviderCompat != nil {
		nativeToolSupport = b.mainProviderCompat.SupportsNativeFunctionCalling()
	}
	pr.req.Tools = []types.ToolSchema{planningcap.NumberedPlanToolSchema()}
	pr.req.ToolChoice = &types.ToolChoice{Mode: types.ToolChoiceModeRequired}
	if nativeToolSupport {
		pr.req.ToolCallMode = types.ToolCallModeNative
	} else {
		pr.req.ToolCallMode = types.ToolCallModeXML
	}

	resp, err := pr.chatProvider.Completion(ctx, pr.req)
	if err != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation failed", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no choices")
	}
	choice := resp.FirstChoice()
	steps, parseErr := planningcap.ParseNumberedPlanToolCall(choice.Message)
	if parseErr != nil {
		return nil, NewErrorWithCause(types.ErrAgentExecution, "plan generation did not return tool call", parseErr)
	}
	if len(steps) == 0 {
		return nil, NewError(types.ErrLLMResponseEmpty, "plan generation returned no steps")
	}

	b.logger.Info("plan generated",
		zap.Int("steps", len(steps)),
		zap.String("trace_id", input.TraceID),
	)

	return &PlanResult{
		Steps: steps,
		Metadata: map[string]any{
			"tokens_used": resp.Usage.TotalTokens,
			"model":       resp.Model,
		},
	}, nil
}

// Execute 执行任务（完整的 ReAct 循环）。
// 这是 Agent 的核心执行方法，包含完整的推理-行动循环。
// Requirements 1.7: 集成输入验证
// Requirements 2.4: 输出验证失败时支持重试
func (b *BaseAgent) Execute(ctx context.Context, input *Input) (_ *Output, execErr error) {
	resumeInput, err := b.prepareResumeInput(ctx, input)
	if err != nil {
		return nil, err
	}
	return b.executeWithPipeline(ctx, resumeInput, b.configuredExecutionOptions())
}

func (b *BaseAgent) executeCore(ctx context.Context, input *Input) (_ *Output, execErr error) {
	if input == nil {
		return nil, NewError(types.ErrInputValidation, "input is nil")
	}
	if strings.TrimSpace(input.Content) == "" {
		return nil, NewError(types.ErrInputValidation, "input content is empty")
	}
	startTime := time.Now()

	if input.TraceID != "" {
		ctx = types.WithTraceID(ctx, input.TraceID)
	}
	if input.TenantID != "" {
		ctx = types.WithTenantID(ctx, input.TenantID)
	}
	if input.UserID != "" {
		ctx = types.WithUserID(ctx, input.UserID)
	}

	ctx = agentcontext.ApplyInputContext(ctx, input.Context)

	if runConfig := ResolveRunConfig(ctx, input); runConfig != nil {
		ctx = WithRunConfig(ctx, runConfig)
	}

	if !b.TryLockExec() {
		return nil, ErrAgentBusy
	}
	defer b.UnlockExec()

	if err := b.EnsureReady(); err != nil {
		return nil, err
	}

	if atomic.AddInt64(&b.execCount, 1) == 1 {
		if err := b.Transition(ctx, StateRunning); err != nil {
			atomic.AddInt64(&b.execCount, -1)
			return nil, err
		}
	}

	activeBundle := b.promptBundle
	if doc := b.persistence.LoadPrompt(ctx, b.config.Core.Type, b.config.Core.Name, ""); doc != nil {
		activeBundle.Version = doc.Version
		activeBundle.System = doc.System
		if len(doc.Constraints) > 0 {
			activeBundle.Constraints = doc.Constraints
		}
		b.logger.Info("loaded prompt from store",
			zap.String("version", doc.Version),
			zap.String("agent_type", b.config.Core.Type),
		)
	}

	runID := b.persistence.RecordRun(ctx, b.config.Core.ID, input.TenantID, input.TraceID, input.Content, startTime)

	conversationID := input.ChannelID
	restoredMessages := b.persistence.RestoreConversation(ctx, conversationID)
	defer func() {
		if runID != "" {
			if r := recover(); r != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, fmt.Sprintf("panic: %v", r)); updateErr != nil {
					b.logger.Warn("failed to mark run as failed after panic", zap.Error(updateErr))
				}
				b.logger.Error("panic during execution, run marked as failed",
					zap.Any("panic", r),
					zap.Error(agentcore.PanicPayloadToError(r)),
					zap.String("run_id", runID),
				)
				if execErr == nil {
					execErr = NewErrorWithCause(types.ErrAgentExecution, "react execution panic", agentcore.PanicPayloadToError(r))
				}
			}
			if execErr != nil {
				if updateErr := b.persistence.UpdateRunStatus(ctx, runID, "failed", nil, execErr.Error()); updateErr != nil {
					b.logger.Warn("failed to mark run as failed", zap.Error(updateErr))
				}
			}
		}
		if atomic.AddInt64(&b.execCount, -1) == 0 {
			if err := b.Transition(context.Background(), StateReady); err != nil {
				b.logger.Error("failed to transition to ready", zap.Error(err))
			}
		}
	}()

	b.logger.Info("executing task",
		zap.String("trace_id", input.TraceID),
		zap.String("agent_id", b.config.Core.ID),
		zap.String("agent_type", b.config.Core.Type),
	)

	b.configMu.RLock()
	guardrailsEnabled := b.guardrailsEnabled
	inputValidatorChain := b.inputValidatorChain
	runtimeGuardrailsCfg := b.runtimeGuardrailsCfg
	b.configMu.RUnlock()

	if guardrailsEnabled && inputValidatorChain != nil {
		validationResult, err := inputValidatorChain.Validate(ctx, input.Content)
		if err != nil {
			b.logger.Error("input validation error", zap.Error(err))
			return nil, NewErrorWithCause(types.ErrInputValidation, "input validation error", err)
		}

		if !validationResult.Valid {
			b.logger.Warn("input validation failed",
				zap.String("trace_id", input.TraceID),
				zap.Any("errors", validationResult.Errors),
			)

			failureAction := guardrails.FailureActionReject
			if runtimeGuardrailsCfg != nil {
				failureAction = runtimeGuardrailsCfg.OnInputFailure
			}

			switch failureAction {
			case guardrails.FailureActionReject:
				return nil, &GuardrailsError{
					Type:    GuardrailsErrorTypeInput,
					Message: "input validation failed",
					Errors:  validationResult.Errors,
				}
			case guardrails.FailureActionWarn:
				b.logger.Warn("input validation warning, continuing execution",
					zap.Any("warnings", validationResult.Errors),
				)
			}
		}
	}

	memoryContext := b.collectContextMemory(input.Context)
	conversation := restoredMessages
	if handoffMessages := handoffMessagesFromInputContext(input.Context); len(handoffMessages) > 0 {
		conversation = handoffMessages
	}

	systemContent := activeBundle.RenderSystemPromptWithVars(input.Variables)
	if publicCtx := agentcontext.AdditionalContextText(publicInputContext(input.Context)); publicCtx != "" {
		systemContent += "\n\n<additional_context>\n" + publicCtx + "\n</additional_context>"
	}
	skillContext := skillInstructionsFromInputContext(input.Context)
	if len(skillContext) == 0 {
		skillContext = normalizeInstructionList(agentcontext.SkillInstructionsFromContext(ctx))
	}
	publicContext := publicInputContext(input.Context)
	retrievalItems := retrievalItemsFromInputContext(input.Context)
	if len(retrievalItems) == 0 && b.retriever != nil {
		if records, err := b.retriever.Retrieve(ctx, input.Content, 5); err != nil {
			b.logger.Warn("failed to load retrieval context", zap.Error(err))
		} else {
			retrievalItems = retrievalItemsFromRecords(records)
		}
	}
	toolStates := toolStatesFromInputContext(input.Context)
	if len(toolStates) == 0 && b.toolState != nil {
		if snapshots, err := b.toolState.LoadToolState(ctx, b.ID()); err != nil {
			b.logger.Warn("failed to load tool state context", zap.Error(err))
		} else {
			toolStates = toolStatesFromSnapshots(snapshots)
		}
	}

	ephemeralLayers, traceFeedbackPlan := b.buildEphemeralPromptLayers(ctx, publicContext, input, systemContent, skillContext, memoryContext, conversation, retrievalItems, toolStates)
	messages, assembled := b.assembleMessages(ctx, systemContent, ephemeralLayers, skillContext, memoryContext, conversation, retrievalItems, toolStates, input.Content)
	if assembled != nil {
		b.logger.Debug("context assembled",
			zap.Int("tokens_before", assembled.TokensBefore),
			zap.Int("tokens_after", assembled.TokensAfter),
			zap.String("strategy", assembled.Plan.Strategy),
			zap.String("compression_reason", assembled.Plan.CompressionReason),
			zap.Int("applied_layers", len(assembled.Plan.AppliedLayers)),
		)
		if emit, ok := runtimeStreamEmitterFromContext(ctx); ok {
			emitRuntimeStatus(emit, "prompt_layers_built", RuntimeStreamEvent{
				Timestamp:    time.Now(),
				CurrentStage: "context",
				Data: map[string]any{
					"context_plan":   assembled.Plan,
					"applied_layers": assembled.Plan.AppliedLayers,
					"layer_ids":      promptLayerIDs(assembled.Plan.AppliedLayers),
				},
			})
		}
		b.recordPromptLayerTimeline(input.TraceID, assembled.Plan)
	}
	ctx = b.withApprovalExplainability(ctx, input)

	policy := b.loopControlPolicy()
	b.configMu.RLock()
	maxRetries := policy.RetryBudget
	outputValidator := b.outputValidator
	guardrailsEnabledForOutput := b.guardrailsEnabled
	runtimeGuardrailsCfgForOutput := runtimeGuardrailsFromPolicy(policy, b.runtimeGuardrailsCfg)
	b.configMu.RUnlock()

	var resp *types.ChatResponse
	var outputContent string
	var lastValidationResult *guardrails.ValidationResult
	var choice types.ChatChoice

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			b.logger.Info("retrying execution due to output validation failure",
				zap.Int("attempt", attempt),
				zap.String("trace_id", input.TraceID),
			)

			if lastValidationResult != nil {
				feedbackMsg := b.buildValidationFeedbackMessage(lastValidationResult)
				messages = append(messages, types.Message{
					Role:    types.RoleUser,
					Content: feedbackMsg,
				})
			}
		}

		var err error
		resp, err = b.ChatCompletion(ctx, messages)
		if err != nil {
			b.logger.Error("execution failed",
				zap.Error(err),
				zap.String("trace_id", input.TraceID),
			)
			return nil, NewErrorWithCause(types.ErrAgentExecution, "execution failed", err)
		}

		if resp == nil || len(resp.Choices) == 0 {
			return nil, NewError(types.ErrLLMResponseEmpty, "execution returned no choices")
		}
		choice = resp.FirstChoice()
		outputContent = choice.Message.Content

		if guardrailsEnabledForOutput && outputValidator != nil {
			var filteredContent string
			filteredContent, lastValidationResult, err = outputValidator.ValidateAndFilter(ctx, outputContent)
			if err != nil {
				b.logger.Error("output validation error", zap.Error(err))
				return nil, NewErrorWithCause(types.ErrOutputValidation, "output validation error", err)
			}

			if !lastValidationResult.Valid {
				b.logger.Warn("output validation failed",
					zap.String("trace_id", input.TraceID),
					zap.Int("attempt", attempt),
					zap.Any("errors", lastValidationResult.Errors),
				)

				failureAction := guardrails.FailureActionReject
				if runtimeGuardrailsCfgForOutput != nil {
					failureAction = runtimeGuardrailsCfgForOutput.OnOutputFailure
				}

				if failureAction == guardrails.FailureActionRetry && attempt < maxRetries {
					continue
				}

				switch failureAction {
				case guardrails.FailureActionReject:
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: "output validation failed",
						Errors:  lastValidationResult.Errors,
					}
				case guardrails.FailureActionWarn:
					b.logger.Warn("output validation warning, using filtered content")
					outputContent = filteredContent
				case guardrails.FailureActionRetry:
					return nil, &GuardrailsError{
						Type:    GuardrailsErrorTypeOutput,
						Message: fmt.Sprintf("output validation failed after %d retries", maxRetries),
						Errors:  lastValidationResult.Errors,
					}
				}
			} else {
				outputContent = filteredContent
			}
		}

		break
	}

	estimatedCost := defaultCostCalc.Calculate(resp.Provider, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

	if b.memoryRuntime != nil {
		if err := b.memoryRuntime.ObserveTurn(ctx, b.ID(), MemoryObservationInput{
			TraceID:          input.TraceID,
			UserContent:      input.Content,
			AssistantContent: outputContent,
			Metadata: map[string]any{
				"tokens":        resp.Usage.TotalTokens,
				"cost":          estimatedCost,
				"finish_reason": choice.FinishReason,
			},
		}); err != nil {
			b.logger.Warn("memory runtime observe turn failed", zap.Error(err))
		}
	} else {
		skipBaseMemory := b.memoryFacade != nil && b.memoryFacade.SkipBaseMemory()
		if b.memory != nil && !skipBaseMemory {
			if err := b.SaveMemory(ctx, input.Content, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "user",
			}); err != nil {
				b.logger.Warn("failed to save user input to memory", zap.Error(err))
			}
			if err := b.SaveMemory(ctx, outputContent, MemoryShortTerm, map[string]any{
				"trace_id": input.TraceID,
				"role":     "assistant",
			}); err != nil {
				b.logger.Warn("failed to save response to memory", zap.Error(err))
			}
		}
	}

	duration := time.Since(startTime)

	b.persistence.PersistConversation(ctx, conversationID, b.config.Core.ID, input.TenantID, input.UserID, input.Content, outputContent)

	if runID != "" {
		outputDoc := &RunOutputDoc{
			Content:      outputContent,
			TokensUsed:   resp.Usage.TotalTokens,
			Cost:         estimatedCost,
			FinishReason: choice.FinishReason,
		}
		if err := b.persistence.UpdateRunStatus(ctx, runID, "completed", outputDoc, ""); err != nil {
			b.logger.Warn("failed to update run status", zap.Error(err))
		}
	}

	b.logger.Info("execution completed",
		zap.String("trace_id", input.TraceID),
		zap.Duration("duration", duration),
		zap.Int("tokens_used", resp.Usage.TotalTokens),
	)

	outputMetadata := map[string]any{
		"model":    resp.Model,
		"provider": resp.Provider,
	}
	if assembled != nil {
		outputMetadata["context_plan"] = assembled.Plan
		outputMetadata["applied_prompt_layers"] = assembled.Plan.AppliedLayers
		outputMetadata["applied_prompt_layer_ids"] = promptLayerIDs(assembled.Plan.AppliedLayers)
	}
	outputMetadata["trace_feedback_plan"] = map[string]any{
		"planner_id":           traceFeedbackPlan.PlannerID,
		"planner_version":      traceFeedbackPlan.PlannerVersion,
		"confidence":           traceFeedbackPlan.Confidence,
		"goal":                 traceFeedbackPlan.Goal,
		"recommended_action":   string(traceFeedbackPlan.RecommendedAction),
		"inject_memory_recall": traceFeedbackPlan.InjectMemoryRecall,
		"primary_layer":        traceFeedbackPlan.PrimaryLayer,
		"secondary_layer":      traceFeedbackPlan.SecondaryLayer,
		"selected_layers":      cloneStringSlice(traceFeedbackPlan.SelectedLayers),
		"suppressed_layers":    cloneStringSlice(traceFeedbackPlan.SuppressedLayers),
		"reasons":              cloneStringSlice(traceFeedbackPlan.Reasons),
		"signals": map[string]any{
			"has_prior_synopsis":        traceFeedbackPlan.Signals.HasPriorSynopsis,
			"has_compressed_history":    traceFeedbackPlan.Signals.HasCompressedHistory,
			"resume":                    traceFeedbackPlan.Signals.Resume,
			"handoff":                   traceFeedbackPlan.Signals.Handoff,
			"multi_agent":               traceFeedbackPlan.Signals.MultiAgent,
			"verification":              traceFeedbackPlan.Signals.Verification,
			"complex_task":              traceFeedbackPlan.Signals.ComplexTask,
			"context_pressure":          traceFeedbackPlan.Signals.ContextPressure,
			"usage_ratio":               traceFeedbackPlan.Signals.UsageRatio,
			"acceptance_criteria_count": traceFeedbackPlan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    traceFeedbackPlan.Signals.CompressedEventCount,
		},
		"metadata": cloneAnyMap(traceFeedbackPlan.Metadata),
		"summary":  traceFeedbackPlan.Summary,
	}
	if extraMetadata, ok := choice.Message.Metadata.(map[string]any); ok {
		for key, value := range extraMetadata {
			outputMetadata[key] = value
		}
	}
	return &Output{
		TraceID:          input.TraceID,
		Content:          outputContent,
		ReasoningContent: choice.Message.ReasoningContent,
		Metadata:         outputMetadata,
		TokensUsed:       resp.Usage.TotalTokens,
		Cost:             estimatedCost,
		Duration:         duration,
		FinishReason:     choice.FinishReason,
	}, nil
}

func (b *BaseAgent) withApprovalExplainability(ctx context.Context, input *Input) context.Context {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityRecorder)
	if !ok || input == nil {
		return ctx
	}
	return withApprovalExplainabilityEmitter(ctx, recorder, strings.TrimSpace(input.TraceID))
}

func (b *BaseAgent) recordPromptLayerTimeline(traceID string, plan agentcontext.ContextPlan) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	recorder.AddExplainabilityTimeline(traceID, "prompt_layers", "Prompt layers assembled for this request", map[string]any{
		"context_plan":   plan,
		"applied_layers": plan.AppliedLayers,
		"layer_ids":      promptLayerIDs(plan.AppliedLayers),
	})
}

func (b *BaseAgent) assembleMessages(
	ctx context.Context,
	systemPrompt string,
	ephemeralLayers []agentcontext.PromptLayer,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	userInput string,
) ([]types.Message, *agentcontext.AssembleResult) {
	if manager, ok := b.contextManager.(interface {
		Assemble(context.Context, *agentcontext.AssembleRequest) (*agentcontext.AssembleResult, error)
	}); ok {
		result, err := manager.Assemble(ctx, &agentcontext.AssembleRequest{
			SystemPrompt:    systemPrompt,
			EphemeralLayers: ephemeralLayers,
			SkillContext:    skillContext,
			MemoryContext:   memoryContext,
			Conversation:    conversation,
			Retrieval:       retrieval,
			ToolState:       toolStates,
			UserInput:       userInput,
			Query:           userInput,
		})
		if err == nil && result != nil && len(result.Messages) > 0 {
			return result.Messages, result
		}
		if err != nil {
			b.logger.Warn("context assembly failed, falling back to legacy message construction", zap.Error(err))
		}
	}

	msgCap := 1 + len(ephemeralLayers) + len(skillContext) + len(memoryContext) + len(conversation) + 1
	messages := make([]types.Message, 0, msgCap)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, layer := range ephemeralLayers {
		if strings.TrimSpace(layer.Content) == "" {
			continue
		}
		role := layer.Role
		if role == "" {
			role = types.RoleSystem
		}
		messages = append(messages, types.Message{Role: role, Content: layer.Content, Metadata: layer.Metadata})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) == "" {
			continue
		}
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	for _, item := range memoryContext {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
	}
	messages = append(messages, conversation...)
	messages = append(messages, types.Message{Role: types.RoleUser, Content: userInput})
	return messages, nil
}

func (b *BaseAgent) buildEphemeralPromptLayers(
	ctx context.Context,
	publicContext map[string]any,
	input *Input,
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
) ([]agentcontext.PromptLayer, TraceFeedbackPlan) {
	if b.ephemeralPrompt == nil {
		return nil, TraceFeedbackPlan{}
	}
	status := b.estimateContextStatus(systemPrompt, skillContext, memoryContext, conversation, retrieval, toolStates, input)
	snapshot := b.latestTraceSynopsisSnapshot(input)
	plan := b.selectTraceFeedbackPlan(input, status, snapshot)
	checkpointID := ""
	if input != nil && input.Context != nil {
		if value, ok := input.Context["checkpoint_id"].(string); ok {
			checkpointID = strings.TrimSpace(value)
		}
	}
	b.recordTraceFeedbackDecision(input.TraceID, plan, status)
	layers := b.ephemeralPrompt.Build(EphemeralPromptLayerInput{
		PublicContext:            publicContext,
		TraceID:                  strings.TrimSpace(input.TraceID),
		TenantID:                 strings.TrimSpace(input.TenantID),
		UserID:                   strings.TrimSpace(input.UserID),
		ChannelID:                strings.TrimSpace(input.ChannelID),
		TraceFeedbackPlan:        &plan,
		TraceSynopsis:            conditionalTraceSynopsis(plan.InjectSynopsis, snapshot),
		TraceHistorySummary:      conditionalTraceHistory(plan.InjectHistory, snapshot),
		TraceHistoryEventCount:   conditionalTraceHistoryCount(plan.InjectHistory, snapshot),
		CheckpointID:             checkpointID,
		AllowedTools:             b.effectivePromptToolNames(ctx),
		ToolsDisabled:            promptToolsDisabled(ctx),
		AcceptanceCriteria:       acceptanceCriteriaForValidation(input, nil),
		ToolVerificationRequired: toolVerificationRequired(input, nil, nil),
		CodeVerificationRequired: codeTaskRequired(input, nil, nil),
		ContextStatus:            status,
	})
	if b.memoryRuntime != nil && plan.InjectMemoryRecall {
		recallLayers, err := b.memoryRuntime.RecallForPrompt(ctx, b.ID(), MemoryRecallOptions{
			Query:  input.Content,
			Status: status,
			TopK:   3,
		})
		if err != nil {
			b.logger.Warn("memory runtime recall failed", zap.Error(err))
		} else if len(recallLayers) > 0 {
			layers = append(layers, recallLayers...)
		}
	}
	return layers, plan
}

func (b *BaseAgent) estimateContextStatus(
	systemPrompt string,
	skillContext []string,
	memoryContext []string,
	conversation []types.Message,
	retrieval []agentcontext.RetrievalItem,
	toolStates []agentcontext.ToolState,
	input *Input,
) *agentcontext.Status {
	if b.contextManager == nil {
		return nil
	}
	messages := make([]types.Message, 0, 1+len(skillContext)+len(memoryContext)+len(conversation)+len(retrieval)+len(toolStates)+1)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, types.Message{Role: types.RoleSystem, Content: systemPrompt})
	}
	for _, item := range skillContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	for _, item := range memoryContext {
		if strings.TrimSpace(item) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item})
		}
	}
	messages = append(messages, conversation...)
	for _, item := range retrieval {
		if strings.TrimSpace(item.Content) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Content})
		}
	}
	for _, item := range toolStates {
		if strings.TrimSpace(item.Summary) != "" {
			messages = append(messages, types.Message{Role: types.RoleSystem, Content: item.Summary})
		}
	}
	if input != nil && strings.TrimSpace(input.Content) != "" {
		messages = append(messages, types.Message{Role: types.RoleUser, Content: input.Content})
	}
	status := b.contextManager.GetStatus(messages)
	return &status
}

func (b *BaseAgent) selectTraceFeedbackPlan(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot) TraceFeedbackPlan {
	planner := b.traceFeedbackPlanner
	if planner == nil {
		planner = NewComposedTraceFeedbackPlanner(NewRuleBasedTraceFeedbackPlanner(), NewHintTraceFeedbackAdapter())
	}
	sessionID := ""
	traceID := ""
	if input != nil {
		sessionID = strings.TrimSpace(input.ChannelID)
		traceID = strings.TrimSpace(input.TraceID)
	}
	if sessionID == "" {
		sessionID = traceID
	}
	return planner.Plan(&agentcontext.TraceFeedbackPlanningInput{
		AgentID:          b.ID(),
		TraceID:          traceID,
		SessionID:        sessionID,
		UserInputContext: cloneAnyMap(inputContext(input)),
		Signals:          collectTraceFeedbackSignals(input, status, snapshot, b.memoryRuntime != nil),
		Snapshot:         agentcontext.ExplainabilitySynopsisSnapshot(snapshot),
		Config:           TraceFeedbackConfigFromAgentConfig(b.config),
	})
}

func (b *BaseAgent) latestTraceSynopsis(input *Input) string {
	snapshot := b.latestTraceSynopsisSnapshot(input)
	if strings.TrimSpace(snapshot.Synopsis) != "" {
		return strings.TrimSpace(snapshot.Synopsis)
	}
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisReader)
	if !ok || input == nil {
		return ""
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return strings.TrimSpace(reader.GetLatestExplainabilitySynopsis(sessionID, b.ID(), strings.TrimSpace(input.TraceID)))
}

func (b *BaseAgent) latestTraceHistorySummary(input *Input) string {
	return strings.TrimSpace(b.latestTraceSynopsisSnapshot(input).CompressedHistory)
}

func (b *BaseAgent) latestTraceHistoryEventCount(input *Input) int {
	return b.latestTraceSynopsisSnapshot(input).CompressedEventCount
}

func (b *BaseAgent) latestTraceSynopsisSnapshot(input *Input) ExplainabilitySynopsisSnapshot {
	reader, ok := b.extensions.ObservabilitySystemExt().(ExplainabilitySynopsisSnapshotReader)
	if !ok || input == nil {
		return ExplainabilitySynopsisSnapshot{}
	}
	sessionID := strings.TrimSpace(input.ChannelID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(input.TraceID)
	}
	return reader.GetLatestExplainabilitySynopsisSnapshot(sessionID, b.ID(), strings.TrimSpace(input.TraceID))
}

func conditionalTraceSynopsis(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.Synopsis)
}

func conditionalTraceHistory(enabled bool, snapshot ExplainabilitySynopsisSnapshot) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(snapshot.CompressedHistory)
}

func conditionalTraceHistoryCount(enabled bool, snapshot ExplainabilitySynopsisSnapshot) int {
	if !enabled {
		return 0
	}
	return snapshot.CompressedEventCount
}

func (b *BaseAgent) recordTraceFeedbackDecision(traceID string, plan TraceFeedbackPlan, status *agentcontext.Status) {
	recorder, ok := b.extensions.ObservabilitySystemExt().(ExplainabilityTimelineRecorder)
	if !ok || strings.TrimSpace(traceID) == "" {
		return
	}
	metadata := map[string]any{
		"inject_synopsis":         plan.InjectSynopsis,
		"inject_history":          plan.InjectHistory,
		"inject_memory_recall":    plan.InjectMemoryRecall,
		"score":                   plan.Score,
		"synopsis_threshold":      plan.SynopsisThreshold,
		"history_threshold":       plan.HistoryThreshold,
		"memory_recall_threshold": plan.MemoryRecallThreshold,
		"reasons":                 cloneStringSlice(plan.Reasons),
		"selected_layers":         cloneStringSlice(plan.SelectedLayers),
		"suppressed_layers":       cloneStringSlice(plan.SuppressedLayers),
		"goal":                    plan.Goal,
		"recommended_action":      string(plan.RecommendedAction),
		"primary_layer":           plan.PrimaryLayer,
		"secondary_layer":         plan.SecondaryLayer,
		"planner_id":              plan.PlannerID,
		"planner_version":         plan.PlannerVersion,
		"confidence":              plan.Confidence,
		"planner_metadata":        cloneAnyMap(plan.Metadata),
		"signals": map[string]any{
			"has_prior_synopsis":        plan.Signals.HasPriorSynopsis,
			"has_compressed_history":    plan.Signals.HasCompressedHistory,
			"resume":                    plan.Signals.Resume,
			"handoff":                   plan.Signals.Handoff,
			"multi_agent":               plan.Signals.MultiAgent,
			"verification":              plan.Signals.Verification,
			"complex_task":              plan.Signals.ComplexTask,
			"context_pressure":          plan.Signals.ContextPressure,
			"usage_ratio":               plan.Signals.UsageRatio,
			"acceptance_criteria_count": plan.Signals.AcceptanceCriteriaCount,
			"compressed_event_count":    plan.Signals.CompressedEventCount,
		},
	}
	if status != nil {
		metadata["usage_ratio"] = status.UsageRatio
		metadata["pressure_level"] = status.Level.String()
	}
	recorder.AddExplainabilityTimeline(traceID, "trace_feedback_decision", plan.Summary, metadata)
}

func (b *BaseAgent) effectivePromptToolNames(ctx context.Context) []string {
	rc := GetRunConfig(ctx)
	if rc != nil && rc.DisableTools {
		return nil
	}
	var names []string
	if b.toolManager != nil {
		for _, schema := range b.toolManager.GetAllowedTools(b.config.Core.ID) {
			names = append(names, schema.Name)
		}
	}
	if rc != nil && len(rc.ToolWhitelist) > 0 {
		names = filterStringWhitelist(names, rc.ToolWhitelist)
	} else if allowed := b.config.ExecutionOptions().Tools.AllowedTools; len(allowed) > 0 {
		names = filterStringWhitelist(names, allowed)
	}
	for _, target := range runtimeHandoffTargetsFromContext(ctx, b.config.Core.ID) {
		names = append(names, runtimeHandoffToolSchema(target).Name)
	}
	return normalizeStringSlice(names)
}

func promptToolsDisabled(ctx context.Context) bool {
	rc := GetRunConfig(ctx)
	return rc != nil && rc.DisableTools
}

func filterStringWhitelist(values []string, whitelist []string) []string {
	if len(values) == 0 || len(whitelist) == 0 {
		return normalizeStringSlice(values)
	}
	allowed := make(map[string]struct{}, len(whitelist))
	for _, value := range whitelist {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		allowed[trimmed] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[strings.TrimSpace(value)]; ok {
			filtered = append(filtered, value)
		}
	}
	return normalizeStringSlice(filtered)
}

func promptLayerIDs(layers []agentcontext.PromptLayerMeta) []string {
	if len(layers) == 0 {
		return nil
	}
	ids := make([]string, 0, len(layers))
	for _, layer := range layers {
		if trimmed := strings.TrimSpace(layer.ID); trimmed != "" {
			ids = append(ids, trimmed)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return ids
}

func retrievalItemsFromInputContext(values map[string]any) []agentcontext.RetrievalItem {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["retrieval_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.RetrievalItem)
	if !ok {
		return nil
	}
	return append([]agentcontext.RetrievalItem(nil), items...)
}

func skillInstructionsFromInputContext(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["skill_context"]
	if !ok {
		return nil
	}
	items, ok := raw.([]string)
	if !ok {
		return nil
	}
	return normalizeInstructionList(items)
}

func retrievalItemsFromRecords(records []types.RetrievalRecord) []agentcontext.RetrievalItem {
	if len(records) == 0 {
		return nil
	}
	items := make([]agentcontext.RetrievalItem, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.Content) == "" {
			continue
		}
		items = append(items, agentcontext.RetrievalItem{
			Title:   record.DocID,
			Content: record.Content,
			Source:  record.Source,
			Score:   record.Score,
		})
	}
	return items
}

func toolStatesFromInputContext(values map[string]any) []agentcontext.ToolState {
	if len(values) == 0 {
		return nil
	}
	raw, ok := values["tool_state"]
	if !ok {
		return nil
	}
	items, ok := raw.([]agentcontext.ToolState)
	if !ok {
		return nil
	}
	return append([]agentcontext.ToolState(nil), items...)
}

func toolStatesFromSnapshots(items []types.ToolStateSnapshot) []agentcontext.ToolState {
	if len(items) == 0 {
		return nil
	}
	out := make([]agentcontext.ToolState, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Summary) == "" {
			continue
		}
		out = append(out, agentcontext.ToolState{
			ToolName:   item.ToolName,
			Summary:    item.Summary,
			ArtifactID: item.ArtifactID,
		})
	}
	return out
}

func (b *BaseAgent) prepareResumeInput(ctx context.Context, input *Input) (*Input, error) {
	if input == nil || b.checkpointManager == nil {
		return input, nil
	}
	checkpointID, resumeLatest := resumeDirective(input)
	if checkpointID == "" && !resumeLatest {
		return input, nil
	}

	var (
		checkpoint *Checkpoint
		err        error
	)
	if checkpointID != "" {
		checkpoint, err = b.checkpointManager.LoadCheckpoint(ctx, checkpointID)
	} else {
		threadID := resumeThreadID(input, b.ID())
		checkpoint, err = b.checkpointManager.LoadLatestCheckpoint(ctx, threadID)
	}
	if err != nil {
		return nil, err
	}
	if checkpoint != nil && checkpoint.AgentID != "" && checkpoint.AgentID != b.ID() {
		return nil, NewError(types.ErrInputValidation,
			fmt.Sprintf("checkpoint agent ID mismatch: checkpoint belongs to %s, current agent is %s", checkpoint.AgentID, b.ID()))
	}
	return mergeInputWithCheckpoint(input, checkpoint), nil
}

func resumeDirective(input *Input) (string, bool) {
	if input == nil || len(input.Context) == 0 {
		return "", false
	}
	if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
		return strings.TrimSpace(checkpointID), true
	}
	if enabled, ok := input.Context["resume_latest"].(bool); ok && enabled {
		return "", true
	}
	if enabled, ok := input.Context["resume"].(bool); ok && enabled {
		return "", true
	}
	return "", false
}

func resumeThreadID(input *Input, fallbackAgentID string) string {
	if input == nil {
		return fallbackAgentID
	}
	if threadID := strings.TrimSpace(input.ChannelID); threadID != "" {
		return threadID
	}
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		return traceID
	}
	return fallbackAgentID
}

func mergeInputWithCheckpoint(input *Input, checkpoint *Checkpoint) *Input {
	merged := shallowCopyInput(input)
	if merged.Context == nil {
		merged.Context = make(map[string]any)
	}
	if checkpoint == nil {
		return merged
	}

	if strings.TrimSpace(merged.ChannelID) == "" {
		merged.ChannelID = checkpoint.ThreadID
	}
	merged.Context["checkpoint_id"] = checkpoint.ID
	merged.Context["resume_from_checkpoint"] = true
	merged.Context["resumable"] = true

	for key, value := range checkpoint.Metadata {
		merged.Context[key] = value
	}
	if checkpoint.ExecutionContext != nil {
		if strings.TrimSpace(checkpoint.ExecutionContext.CurrentNode) != "" {
			merged.Context["current_stage"] = checkpoint.ExecutionContext.CurrentNode
		}
		for key, value := range checkpoint.ExecutionContext.Variables {
			merged.Context[key] = value
		}
	}
	if strings.TrimSpace(merged.Content) == "" {
		if goal, ok := merged.Context["goal"].(string); ok && strings.TrimSpace(goal) != "" {
			merged.Content = goal
		}
	}
	return merged
}

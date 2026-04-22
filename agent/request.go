package agent

import (
	"context"
	"encoding/json"
	"fmt"
	agenthandoff "github.com/BaSui01/agentflow/agent/adapters/handoff"
	toolcap "github.com/BaSui01/agentflow/agent/capabilities/tools"
	agentcontext "github.com/BaSui01/agentflow/agent/execution/context"
	"github.com/BaSui01/agentflow/llm"
	llmtools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	"github.com/BaSui01/agentflow/types"
	"go.uber.org/zap"
	"strconv"
	"strings"
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
	toolRiskSafeRead         = "safe_read"
	toolRiskRequiresApproval = "requires_approval"
	toolRiskUnknown          = "unknown"
)

func classifyToolRiskByName(name string) string {
	switch strings.TrimSpace(name) {
	case "web_search", "file_search", "retrieval", "read_file", "list_directory":
		return toolRiskSafeRead
	case "write_file", "edit_file", "run_command", "code_execution":
		return toolRiskRequiresApproval
	default:
		if strings.HasPrefix(strings.TrimSpace(name), "mcp_") {
			return toolRiskRequiresApproval
		}
		return toolRiskUnknown
	}
}

func groupToolRisks(names []string) map[string][]string {
	grouped := map[string][]string{
		toolRiskSafeRead:         {},
		toolRiskRequiresApproval: {},
		toolRiskUnknown:          {},
	}
	for _, name := range normalizeStringSlice(names) {
		risk := classifyToolRiskByName(name)
		grouped[risk] = append(grouped[risk], name)
	}
	return grouped
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

// EphemeralPromptLayerBuilder builds request-scoped prompt layers that should
// not mutate the stable system prompt bundle.
type EphemeralPromptLayerBuilder struct{}

type EphemeralPromptLayerInput struct {
	PublicContext            map[string]any
	TraceID                  string
	TenantID                 string
	UserID                   string
	ChannelID                string
	TraceFeedbackPlan        *TraceFeedbackPlan
	TraceSynopsis            string
	TraceHistorySummary      string
	TraceHistoryEventCount   int
	CheckpointID             string
	AllowedTools             []string
	ToolsDisabled            bool
	AcceptanceCriteria       []string
	ToolVerificationRequired bool
	CodeVerificationRequired bool
	ContextStatus            *agentcontext.Status
}

func NewEphemeralPromptLayerBuilder() *EphemeralPromptLayerBuilder {
	return &EphemeralPromptLayerBuilder{}
}

func (b *EphemeralPromptLayerBuilder) Build(input EphemeralPromptLayerInput) []agentcontext.PromptLayer {
	layers := make([]agentcontext.PromptLayer, 0, 7)
	if layer := buildSessionOverlayLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceFeedbackPlanLayer(input.TraceFeedbackPlan); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceSynopsisLayer(input.TraceSynopsis); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildTraceHistoryLayer(input.TraceHistorySummary, input.TraceHistoryEventCount); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildToolGuidanceLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildVerificationGateLayer(input); layer != nil {
		layers = append(layers, *layer)
	}
	if layer := buildContextPressureLayer(input.ContextStatus); layer != nil {
		layers = append(layers, *layer)
	}
	if len(layers) == 0 {
		return nil
	}
	return layers
}

func buildSessionOverlayLayer(input EphemeralPromptLayerInput) *agentcontext.PromptLayer {
	payload := make(map[string]any, len(input.PublicContext)+5)
	if traceID := strings.TrimSpace(input.TraceID); traceID != "" {
		payload["trace_id"] = traceID
	}
	if tenantID := strings.TrimSpace(input.TenantID); tenantID != "" {
		payload["tenant_id"] = tenantID
	}
	if userID := strings.TrimSpace(input.UserID); userID != "" {
		payload["user_id"] = userID
	}
	if channelID := strings.TrimSpace(input.ChannelID); channelID != "" {
		payload["channel_id"] = channelID
	}
	for key, value := range input.PublicContext {
		payload[key] = value
	}
	checkpointID := strings.TrimSpace(input.CheckpointID)
	if checkpointID != "" {
		payload["checkpoint_id"] = checkpointID
	}
	if len(payload) == 0 {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil || len(raw) == 0 {
		return nil
	}
	return &agentcontext.PromptLayer{
		ID:       "session_overlay",
		Type:     agentcontext.SegmentEphemeral,
		Content:  "<session_overlay>\n" + string(raw) + "\n</session_overlay>",
		Priority: 90,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":     "session_overlay",
			"checkpoint_id":  checkpointID,
			"session_fields": sortedKeys(payload),
		},
	}
}

func buildTraceFeedbackPlanLayer(plan *TraceFeedbackPlan) *agentcontext.PromptLayer {
	if plan == nil || strings.TrimSpace(plan.Summary) == "" {
		return nil
	}
	var body strings.Builder
	body.WriteString("<trace_feedback_plan>\n")
	if strings.TrimSpace(plan.Goal) != "" {
		body.WriteString("Goal: " + plan.Goal + "\n")
	}
	if plan.RecommendedAction != "" {
		body.WriteString("Recommended action: " + string(plan.RecommendedAction) + "\n")
	}
	if strings.TrimSpace(plan.PrimaryLayer) != "" {
		body.WriteString("Primary layer: " + plan.PrimaryLayer + "\n")
	}
	if strings.TrimSpace(plan.SecondaryLayer) != "" {
		body.WriteString("Secondary layer: " + plan.SecondaryLayer + "\n")
	}
	if plan.InjectMemoryRecall {
		body.WriteString("Memory recall: enabled\n")
	}
	if strings.TrimSpace(plan.PlannerID) != "" {
		body.WriteString("Planner: " + plan.PlannerID)
		if strings.TrimSpace(plan.PlannerVersion) != "" {
			body.WriteString("@" + plan.PlannerVersion)
		}
		body.WriteString("\n")
	}
	if plan.Confidence > 0 {
		body.WriteString("Confidence: " + formatTraceFeedbackFloat(plan.Confidence) + "\n")
	}
	if len(plan.Reasons) > 0 {
		body.WriteString("Reasons: " + strings.Join(plan.Reasons, ", ") + "\n")
	}
	body.WriteString("Decision: " + plan.Summary + "\n")
	body.WriteString("</trace_feedback_plan>")
	return &agentcontext.PromptLayer{
		ID:       "trace_feedback_plan",
		Type:     agentcontext.SegmentEphemeral,
		Content:  body.String(),
		Priority: 89,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":           "trace_feedback_plan",
			"goal":                 plan.Goal,
			"recommended_action":   string(plan.RecommendedAction),
			"primary_layer":        plan.PrimaryLayer,
			"secondary_layer":      plan.SecondaryLayer,
			"inject_memory_recall": plan.InjectMemoryRecall,
			"planner_id":           plan.PlannerID,
			"planner_version":      plan.PlannerVersion,
			"confidence":           plan.Confidence,
			"selected_layers":      cloneStringSlice(plan.SelectedLayers),
			"suppressed_layers":    cloneStringSlice(plan.SuppressedLayers),
			"score":                plan.Score,
			"planner_metadata":     cloneAnyMap(plan.Metadata),
		},
	}
}

func formatTraceFeedbackFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
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

func buildTraceSynopsisLayer(synopsis string) *agentcontext.PromptLayer {
	synopsis = strings.TrimSpace(synopsis)
	if synopsis == "" {
		return nil
	}
	return &agentcontext.PromptLayer{
		ID:       "trace_synopsis",
		Type:     agentcontext.SegmentEphemeral,
		Content:  "<trace_synopsis>\nRecent completed execution summary for this session: " + synopsis + "\n</trace_synopsis>",
		Priority: 89,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind": "trace_synopsis",
			"source":     "explainability",
		},
	}
}

func buildTraceHistoryLayer(summary string, eventCount int) *agentcontext.PromptLayer {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	countText := ""
	if eventCount > 0 {
		countText = fmt.Sprintf(" (%d earlier timeline events compressed)", eventCount)
	}
	return &agentcontext.PromptLayer{
		ID:       "trace_history",
		Type:     agentcontext.SegmentEphemeral,
		Content:  "<trace_history>\nCompressed prior execution history" + countText + ": " + summary + "\n</trace_history>",
		Priority: 84,
		Sticky:   false,
		Metadata: map[string]any{
			"layer_kind":                "trace_history",
			"source":                    "explainability",
			"compressed_timeline_count": eventCount,
		},
	}
}

func buildToolGuidanceLayer(input EphemeralPromptLayerInput) *agentcontext.PromptLayer {
	if input.ToolsDisabled {
		return &agentcontext.PromptLayer{
			ID:       "tool_guidance",
			Type:     agentcontext.SegmentEphemeral,
			Content:  "<tool_guidance>\nTools are disabled for this request. Do not plan around tool usage.\n</tool_guidance>",
			Priority: 88,
			Sticky:   true,
			Metadata: map[string]any{"layer_kind": "tool_guidance", "tools_disabled": true},
		}
	}
	tools := normalizeStringSlice(input.AllowedTools)
	if len(tools) == 0 {
		return nil
	}
	grouped := groupToolRisks(tools)
	var body strings.Builder
	body.WriteString("<tool_guidance>\n")
	body.WriteString("Available tools are grouped by permission risk for this request.\n")
	if len(grouped[toolRiskSafeRead]) > 0 {
		body.WriteString("Safe read tools: " + strings.Join(grouped[toolRiskSafeRead], ", ") + ".\n")
	}
	if len(grouped[toolRiskRequiresApproval]) > 0 {
		body.WriteString("Approval-required tools: " + strings.Join(grouped[toolRiskRequiresApproval], ", ") + ". Request approval before relying on mutating, execution, or MCP actions.\n")
	}
	if len(grouped[toolRiskUnknown]) > 0 {
		body.WriteString("Unknown-risk tools: " + strings.Join(grouped[toolRiskUnknown], ", ") + ". Treat them conservatively and avoid them unless clearly needed.\n")
	}
	body.WriteString("</tool_guidance>")
	return &agentcontext.PromptLayer{
		ID:       "tool_guidance",
		Type:     agentcontext.SegmentEphemeral,
		Content:  body.String(),
		Priority: 88,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":              "tool_guidance",
			"allowed_tools":           tools,
			"safe_read_tools":         grouped[toolRiskSafeRead],
			"approval_required_tools": grouped[toolRiskRequiresApproval],
			"unknown_risk_tools":      grouped[toolRiskUnknown],
			"tools_disabled":          false,
		},
	}
}

func buildVerificationGateLayer(input EphemeralPromptLayerInput) *agentcontext.PromptLayer {
	criteria := normalizeStringSlice(input.AcceptanceCriteria)
	if len(criteria) == 0 && !input.ToolVerificationRequired && !input.CodeVerificationRequired {
		return nil
	}
	var body strings.Builder
	body.WriteString("<verification_gate>\n")
	body.WriteString("Do not treat the task as complete until all applicable verification gates are satisfied.\n")
	if len(criteria) > 0 {
		body.WriteString("Acceptance criteria:\n")
		for _, item := range criteria {
			body.WriteString("- " + item + "\n")
		}
	}
	if input.ToolVerificationRequired {
		body.WriteString("- Tool-backed claims require verification before completion.\n")
	}
	if input.CodeVerificationRequired {
		body.WriteString("- Code changes require implementation-oriented verification before completion.\n")
	}
	body.WriteString("</verification_gate>")
	return &agentcontext.PromptLayer{
		ID:       "verification_gate",
		Type:     agentcontext.SegmentEphemeral,
		Content:  body.String(),
		Priority: 87,
		Sticky:   true,
		Metadata: map[string]any{
			"layer_kind":                 "verification_gate",
			"acceptance_criteria":        criteria,
			"acceptance_criteria_count":  len(criteria),
			"tool_verification_required": input.ToolVerificationRequired,
			"code_verification_required": input.CodeVerificationRequired,
		},
	}
}

func buildContextPressureLayer(status *agentcontext.Status) *agentcontext.PromptLayer {
	if status == nil || status.Level < agentcontext.LevelNormal {
		return nil
	}
	level := strings.ToLower(status.Level.String())
	usagePercent := 0
	if status.UsageRatio > 0 {
		usagePercent = int(status.UsageRatio * 100)
	}
	return &agentcontext.PromptLayer{
		ID:   "context_pressure",
		Type: agentcontext.SegmentEphemeral,
		Content: fmt.Sprintf(
			"<context_pressure>\nContext usage is at %d%% of the available budget (%s). Be concise, avoid repeating prior context, and focus on unresolved items only.\n</context_pressure>",
			usagePercent,
			level,
		),
		Priority: 75,
		Sticky:   false,
		Metadata: map[string]any{
			"usage_ratio":     status.UsageRatio,
			"level":           status.Level.String(),
			"recommendation":  status.Recommendation,
			"current_tokens":  status.CurrentTokens,
			"max_tokens":      status.MaxTokens,
			"ephemeral_layer": "context_pressure",
		},
	}
}

func sortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	// Keys are tiny here; a stable O(n^2) insertion sort is enough and keeps this helper local.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// Merged from trace_feedback.go.

type TraceFeedbackAction string

const (
	TraceFeedbackSkip               TraceFeedbackAction = "skip"
	TraceFeedbackSynopsisOnly       TraceFeedbackAction = "synopsis_only"
	TraceFeedbackHistoryOnly        TraceFeedbackAction = "history_only"
	TraceFeedbackMemoryRecallOnly   TraceFeedbackAction = "memory_recall_only"
	TraceFeedbackSynopsisAndHistory TraceFeedbackAction = "synopsis_and_history"
)

type TraceFeedbackSignals struct {
	HasPriorSynopsis        bool
	HasCompressedHistory    bool
	HasMemoryRuntime        bool
	Resume                  bool
	Handoff                 bool
	MultiAgent              bool
	Verification            bool
	ComplexTask             bool
	ContextPressure         string
	UsageRatio              float64
	AcceptanceCriteriaCount int
	CompressedEventCount    int
}

type TraceFeedbackPlan struct {
	PlannerID             string
	PlannerVersion        string
	Confidence            float64
	Metadata              map[string]any
	InjectSynopsis        bool
	InjectHistory         bool
	InjectMemoryRecall    bool
	Score                 int
	SynopsisThreshold     int
	HistoryThreshold      int
	MemoryRecallThreshold int
	Signals               TraceFeedbackSignals
	Reasons               []string
	SelectedLayers        []string
	SuppressedLayers      []string
	Goal                  string
	RecommendedAction     TraceFeedbackAction
	PrimaryLayer          string
	SecondaryLayer        string
	Summary               string
}

type TraceFeedbackPlanningInput struct {
	AgentID   string
	TraceID   string
	SessionID string

	UserInput *Input

	Signals  TraceFeedbackSignals
	Snapshot ExplainabilitySynopsisSnapshot
	Config   TraceFeedbackConfig
}

type TraceFeedbackPlanner interface {
	Plan(input *TraceFeedbackPlanningInput) TraceFeedbackPlan
}

type TraceFeedbackPlanAdapter interface {
	ID() string
	Supports(input *TraceFeedbackPlanningInput) bool
	Apply(input *TraceFeedbackPlanningInput, plan *TraceFeedbackPlan)
}

type TraceFeedbackConfig struct {
	Enabled              bool
	ComplexityThreshold  int
	SynopsisMinScore     int
	HistoryMinScore      int
	MemoryRecallMinScore int
	HistoryMaxUsageRatio float64
}

type RuleBasedTraceFeedbackPlanner struct{}

type ComposedTraceFeedbackPlanner struct {
	Base     TraceFeedbackPlanner
	Adapters []TraceFeedbackPlanAdapter
}

type HintTraceFeedbackAdapter struct{}

func NewRuleBasedTraceFeedbackPlanner() *RuleBasedTraceFeedbackPlanner {
	return &RuleBasedTraceFeedbackPlanner{}
}

func NewComposedTraceFeedbackPlanner(base TraceFeedbackPlanner, adapters ...TraceFeedbackPlanAdapter) *ComposedTraceFeedbackPlanner {
	if base == nil {
		base = NewRuleBasedTraceFeedbackPlanner()
	}
	filtered := make([]TraceFeedbackPlanAdapter, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter != nil {
			filtered = append(filtered, adapter)
		}
	}
	return &ComposedTraceFeedbackPlanner{Base: base, Adapters: filtered}
}

func NewHintTraceFeedbackAdapter() *HintTraceFeedbackAdapter {
	return &HintTraceFeedbackAdapter{}
}

func (a *HintTraceFeedbackAdapter) ID() string { return "hint_trace_feedback_adapter" }

func (a *HintTraceFeedbackAdapter) Supports(input *TraceFeedbackPlanningInput) bool {
	return input != nil && input.UserInput != nil && input.UserInput.Context != nil
}

func DefaultTraceFeedbackConfig() TraceFeedbackConfig {
	return TraceFeedbackConfig{
		Enabled:              true,
		ComplexityThreshold:  2,
		SynopsisMinScore:     2,
		HistoryMinScore:      3,
		MemoryRecallMinScore: 2,
		HistoryMaxUsageRatio: 0.85,
	}
}

func TraceFeedbackConfigFromAgentConfig(cfg types.AgentConfig) TraceFeedbackConfig {
	out := DefaultTraceFeedbackConfig()
	contextCfg := cfg.ExecutionOptions().Control.Context
	if contextCfg == nil {
		return out
	}
	out.Enabled = contextCfg.TraceFeedbackEnabled
	if contextCfg.TraceFeedbackComplexityThreshold > 0 {
		out.ComplexityThreshold = contextCfg.TraceFeedbackComplexityThreshold
	}
	if contextCfg.TraceSynopsisMinScore > 0 {
		out.SynopsisMinScore = contextCfg.TraceSynopsisMinScore
	}
	if contextCfg.TraceHistoryMinScore > 0 {
		out.HistoryMinScore = contextCfg.TraceHistoryMinScore
	}
	if contextCfg.TraceMemoryRecallMinScore > 0 {
		out.MemoryRecallMinScore = contextCfg.TraceMemoryRecallMinScore
	}
	if contextCfg.TraceHistoryMaxUsageRatio > 0 {
		out.HistoryMaxUsageRatio = contextCfg.TraceHistoryMaxUsageRatio
	}
	return out
}

func (p *RuleBasedTraceFeedbackPlanner) Plan(in *TraceFeedbackPlanningInput) TraceFeedbackPlan {
	if in == nil {
		return TraceFeedbackPlan{}
	}
	plan := TraceFeedbackPlan{
		PlannerID:             "rule_based_trace_feedback_planner",
		PlannerVersion:        "v1",
		Confidence:            0.8,
		Metadata:              map[string]any{"planner_kind": "rule_based"},
		SynopsisThreshold:     in.Config.SynopsisMinScore,
		HistoryThreshold:      in.Config.HistoryMinScore,
		MemoryRecallThreshold: in.Config.MemoryRecallMinScore,
		Signals:               in.Signals,
	}
	if !in.Config.Enabled || (!plan.Signals.HasPriorSynopsis && !plan.Signals.HasCompressedHistory) {
		plan.RecommendedAction = TraceFeedbackSkip
		plan.Goal = "fresh_turn"
		plan.Summary = "trace feedback disabled or no prior synopsis available"
		plan.Confidence = 1.0
		plan.Metadata["decision_basis"] = "disabled_or_missing_snapshot"
		return plan
	}

	if plan.Signals.Resume {
		plan.Score += 3
		plan.Reasons = append(plan.Reasons, "resume")
	}
	if plan.Signals.Handoff {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "handoff")
	}
	if plan.Signals.MultiAgent {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "multi_agent")
	}
	if plan.Signals.Verification {
		plan.Score += 2
		plan.Reasons = append(plan.Reasons, "verification_gate")
	}
	if plan.Signals.ComplexTask {
		plan.Score += 1
		plan.Reasons = append(plan.Reasons, "complex_task")
	}
	if plan.Signals.ContextPressure != "" && plan.Signals.ContextPressure != agentcontext.LevelNone.String() {
		plan.Score += 1
		plan.Reasons = append(plan.Reasons, "context_pressure")
	}

	hardSignals := plan.Signals.Resume || plan.Signals.Verification || plan.Signals.Handoff || plan.Signals.MultiAgent
	plan.InjectSynopsis = plan.Signals.HasPriorSynopsis && plan.Score >= plan.SynopsisThreshold
	plan.InjectHistory = plan.Signals.HasCompressedHistory &&
		plan.Score >= plan.HistoryThreshold &&
		(plan.Signals.UsageRatio == 0 || plan.Signals.UsageRatio <= in.Config.HistoryMaxUsageRatio)
	plan.InjectMemoryRecall = plan.Signals.HasMemoryRuntime &&
		plan.Score >= plan.MemoryRecallThreshold &&
		(plan.Signals.ContextPressure == "" ||
			(plan.Signals.ContextPressure != agentcontext.LevelAggressive.String() &&
				plan.Signals.ContextPressure != agentcontext.LevelEmergency.String()))

	if !hardSignals && plan.Score < in.Config.ComplexityThreshold {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.Reasons = append(plan.Reasons, "below_complexity_threshold")
	}

	if plan.Signals.ContextPressure == agentcontext.LevelAggressive.String() || plan.Signals.ContextPressure == agentcontext.LevelEmergency.String() {
		plan.InjectHistory = false
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_history")
		plan.Reasons = append(plan.Reasons, "history_suppressed_by_pressure")
	}
	if plan.Signals.ContextPressure == agentcontext.LevelAggressive.String() || plan.Signals.ContextPressure == agentcontext.LevelEmergency.String() {
		plan.InjectMemoryRecall = false
		plan.SuppressedLayers = append(plan.SuppressedLayers, "memory_recall")
		plan.Reasons = append(plan.Reasons, "memory_recall_suppressed_by_pressure")
	}

	if plan.InjectSynopsis {
		plan.SelectedLayers = append(plan.SelectedLayers, "trace_synopsis")
	} else if plan.Signals.HasPriorSynopsis {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_synopsis")
	}
	if plan.InjectHistory {
		plan.SelectedLayers = append(plan.SelectedLayers, "trace_history")
	} else if plan.Signals.HasCompressedHistory && !containsString(plan.SuppressedLayers, "trace_history") {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "trace_history")
	}
	if plan.InjectMemoryRecall {
		plan.SelectedLayers = append(plan.SelectedLayers, "memory_recall")
	} else if plan.Signals.HasMemoryRuntime && !containsString(plan.SuppressedLayers, "memory_recall") {
		plan.SuppressedLayers = append(plan.SuppressedLayers, "memory_recall")
	}

	plan.SelectedLayers = normalizeStringSlice(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStringSlice(plan.SuppressedLayers)
	plan.Goal = deriveTraceFeedbackGoal(plan.Signals)
	plan.RecommendedAction, plan.PrimaryLayer, plan.SecondaryLayer = deriveTraceFeedbackAction(plan)
	plan.Confidence = deriveTraceFeedbackConfidence(plan)
	plan.Metadata["decision_basis"] = "rule_based_scoring"
	plan.Metadata["complexity_threshold"] = in.Config.ComplexityThreshold
	plan.Metadata["synopsis_available"] = plan.Signals.HasPriorSynopsis
	plan.Metadata["history_available"] = plan.Signals.HasCompressedHistory
	plan.Summary = buildTraceFeedbackSummary(plan, in.Config)
	return plan
}

func (p *ComposedTraceFeedbackPlanner) Plan(input *TraceFeedbackPlanningInput) TraceFeedbackPlan {
	plan := p.Base.Plan(input)
	applied := make([]string, 0, len(p.Adapters))
	for _, adapter := range p.Adapters {
		if adapter == nil || !adapter.Supports(input) {
			continue
		}
		adapter.Apply(input, &plan)
		applied = append(applied, adapter.ID())
	}
	if plan.Metadata == nil {
		plan.Metadata = map[string]any{}
	}
	plan.Metadata["adapter_ids"] = applied
	return plan
}

func (a *HintTraceFeedbackAdapter) Apply(input *TraceFeedbackPlanningInput, plan *TraceFeedbackPlan) {
	if !a.Supports(input) || plan == nil {
		return
	}
	ctx := input.UserInput.Context
	if contextBool(input.UserInput, "trace_feedback_force_skip") {
		plan.InjectSynopsis = false
		plan.InjectHistory = false
		plan.InjectMemoryRecall = false
		plan.SelectedLayers = nil
		plan.SuppressedLayers = normalizeStringSlice([]string{"trace_synopsis", "trace_history", "memory_recall"})
		plan.RecommendedAction = TraceFeedbackSkip
		plan.Goal = "operator_forced_skip"
		plan.Reasons = appendUniqueString(plan.Reasons, "force_skip")
	}
	if contextBool(input.UserInput, "trace_feedback_force_synopsis") {
		plan.InjectSynopsis = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_synopsis")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_synopsis")
		if plan.RecommendedAction == TraceFeedbackSkip {
			plan.RecommendedAction = TraceFeedbackSynopsisOnly
		}
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_synopsis")
	}
	if contextBool(input.UserInput, "trace_feedback_force_history") {
		plan.InjectHistory = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "trace_history")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "trace_history")
		if plan.InjectSynopsis {
			plan.RecommendedAction = TraceFeedbackSynopsisAndHistory
		} else {
			plan.RecommendedAction = TraceFeedbackHistoryOnly
		}
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_history")
	}
	if contextBool(input.UserInput, "trace_feedback_force_memory_recall") {
		plan.InjectMemoryRecall = true
		plan.SelectedLayers = appendUniqueString(plan.SelectedLayers, "memory_recall")
		plan.SuppressedLayers = removeString(plan.SuppressedLayers, "memory_recall")
		plan.Goal = fallbackString(contextString(input.UserInput, "trace_feedback_goal"), plan.Goal)
		plan.Reasons = appendUniqueString(plan.Reasons, "force_memory_recall")
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_primary_layer")); value != "" {
		plan.PrimaryLayer = value
		plan.Metadata["hint_primary_layer"] = value
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_secondary_layer")); value != "" {
		plan.SecondaryLayer = value
		plan.Metadata["hint_secondary_layer"] = value
	}
	if value := strings.TrimSpace(contextString(input.UserInput, "trace_feedback_goal")); value != "" {
		plan.Goal = value
		plan.Metadata["hint_goal"] = value
	}
	if len(ctx) > 0 {
		plan.Metadata["adapter_count"] = 1
	}
	plan.SelectedLayers = normalizeStringSlice(plan.SelectedLayers)
	plan.SuppressedLayers = normalizeStringSlice(plan.SuppressedLayers)
	plan.PlannerID = "composed_trace_feedback_planner"
	plan.PlannerVersion = "v1"
	plan.Metadata["planner_kind"] = "composed"
	plan.Summary = buildTraceFeedbackSummary(*plan, input.Config)
}

func collectTraceFeedbackSignals(input *Input, status *agentcontext.Status, snapshot ExplainabilitySynopsisSnapshot, hasMemoryRuntime bool) TraceFeedbackSignals {
	signals := TraceFeedbackSignals{
		HasPriorSynopsis:     strings.TrimSpace(snapshot.Synopsis) != "",
		HasCompressedHistory: strings.TrimSpace(snapshot.CompressedHistory) != "",
		HasMemoryRuntime:     hasMemoryRuntime,
		CompressedEventCount: snapshot.CompressedEventCount,
	}
	if input != nil && input.Context != nil {
		if checkpointID, ok := input.Context["checkpoint_id"].(string); ok && strings.TrimSpace(checkpointID) != "" {
			signals.Resume = true
		}
		signals.Verification = contextBool(input, "tool_verification_required") || contextBool(input, "code_task") || contextBool(input, "requires_code")
		signals.ComplexTask = contextBool(input, "complex_task") || contextString(input, "task_type") != ""
		if value, ok := intOverrideFromContext(input.Context, "top_level_loop_budget"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if value, ok := intOverrideFromContext(input.Context, "max_loop_iterations"); ok && value > 1 {
			signals.ComplexTask = true
		}
		if _, ok := input.Context["agent_ids"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.Context["aggregation_strategy"]; ok {
			signals.MultiAgent = true
		}
		if _, ok := input.Context["max_rounds"]; ok {
			signals.MultiAgent = true
		}
		signals.Handoff = len(handoffMessagesFromInputContext(input.Context)) > 0
	}
	if input != nil {
		signals.AcceptanceCriteriaCount = len(normalizeStringSlice(acceptanceCriteriaForValidation(input, nil)))
		if signals.AcceptanceCriteriaCount > 0 {
			signals.Verification = true
		}
	}
	if status != nil {
		signals.ContextPressure = status.Level.String()
		signals.UsageRatio = status.UsageRatio
	}
	return signals
}

func buildTraceFeedbackSummary(plan TraceFeedbackPlan, cfg TraceFeedbackConfig) string {
	parts := make([]string, 0, 8)
	if strings.TrimSpace(plan.Goal) != "" {
		parts = append(parts, "goal="+plan.Goal)
	}
	if plan.RecommendedAction != "" {
		parts = append(parts, "action="+string(plan.RecommendedAction))
	}
	if strings.TrimSpace(plan.PrimaryLayer) != "" {
		parts = append(parts, "primary="+plan.PrimaryLayer)
	}
	if strings.TrimSpace(plan.SecondaryLayer) != "" {
		parts = append(parts, "secondary="+plan.SecondaryLayer)
	}
	if len(plan.SelectedLayers) > 0 {
		parts = append(parts, "inject="+strings.Join(plan.SelectedLayers, ","))
	}
	if len(plan.SuppressedLayers) > 0 {
		parts = append(parts, "suppress="+strings.Join(plan.SuppressedLayers, ","))
	}
	parts = append(parts, "score="+itoa(plan.Score))
	parts = append(parts, "thresholds="+itoa(cfg.SynopsisMinScore)+"/"+itoa(cfg.HistoryMinScore)+"/"+itoa(cfg.MemoryRecallMinScore))
	if len(plan.Reasons) > 0 {
		parts = append(parts, "reasons="+strings.Join(plan.Reasons, ","))
	}
	return strings.Join(parts, " | ")
}

func deriveTraceFeedbackGoal(signals TraceFeedbackSignals) string {
	switch {
	case signals.Resume:
		return "resume_prior_execution"
	case signals.Handoff:
		return "continue_handoff_context"
	case signals.MultiAgent:
		return "preserve_collaboration_context"
	case signals.Verification:
		return "preserve_verification_context"
	case signals.ComplexTask:
		return "retain_complex_task_context"
	case signals.ContextPressure == agentcontext.LevelNormal.String() ||
		signals.ContextPressure == agentcontext.LevelAggressive.String() ||
		signals.ContextPressure == agentcontext.LevelEmergency.String():
		return "preserve_essential_context_only"
	default:
		return "fresh_turn"
	}
}

func deriveTraceFeedbackAction(plan TraceFeedbackPlan) (action TraceFeedbackAction, primary, secondary string) {
	switch {
	case plan.InjectSynopsis && plan.InjectHistory:
		return TraceFeedbackSynopsisAndHistory, "trace_synopsis", "trace_history"
	case plan.InjectSynopsis:
		return TraceFeedbackSynopsisOnly, "trace_synopsis", ""
	case plan.InjectHistory:
		return TraceFeedbackHistoryOnly, "trace_history", ""
	case plan.InjectMemoryRecall:
		return TraceFeedbackMemoryRecallOnly, "memory_recall", ""
	default:
		return TraceFeedbackSkip, "", ""
	}
}

func deriveTraceFeedbackConfidence(plan TraceFeedbackPlan) float64 {
	confidence := 0.45
	if len(plan.SelectedLayers) > 0 {
		confidence += 0.2
	}
	if plan.Score >= plan.SynopsisThreshold {
		confidence += 0.15
	}
	if plan.Score >= plan.HistoryThreshold {
		confidence += 0.1
	}
	if len(plan.Reasons) > 0 {
		confidence += 0.05
	}
	if confidence > 1.0 {
		return 1.0
	}
	return confidence
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(target) {
			continue
		}
		out = append(out, value)
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	var digits [20]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + (v % 10))
		v /= 10
	}
	return sign + string(digits[i:])
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

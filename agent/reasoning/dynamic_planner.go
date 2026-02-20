package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/tools"
	"go.uber.org/zap"
)

// ============================================================
// 动态规划与后跟踪
// ============================================================

// DynamicPlannerConfig配置了动态规划器.
type DynamicPlannerConfig struct {
	MaxBacktracks       int           // Maximum backtrack attempts
	MaxPlanDepth        int           // Maximum plan depth
	ConfidenceThreshold float64       // Minimum confidence to proceed
	Timeout             time.Duration // Overall timeout
	EnableParallel      bool          // Enable parallel path exploration
	MaxParallelPaths    int           // Maximum parallel paths to explore
}

// 默认 DynamicPlannerConfig 返回合理的默认值 。
func DefaultDynamicPlannerConfig() DynamicPlannerConfig {
	return DynamicPlannerConfig{
		MaxBacktracks:       5,
		MaxPlanDepth:        20,
		ConfidenceThreshold: 0.4,
		Timeout:             180 * time.Second,
		EnableParallel:      true,
		MaxParallelPaths:    3,
	}
}

// PlanNode代表了执行计划中树上的一个节点.
type PlanNode struct {
	ID           string      `json:"id"`
	ParentID     string      `json:"parent_id,omitempty"`
	Action       string      `json:"action"`
	Description  string      `json:"description"`
	Status       NodeStatus  `json:"status"`
	Result       string      `json:"result,omitempty"`
	Error        string      `json:"error,omitempty"`
	Confidence   float64     `json:"confidence"`
	Children     []*PlanNode `json:"children,omitempty"`
	Alternatives []*PlanNode `json:"alternatives,omitempty"` // Alternative paths if this fails
	CreatedAt    time.Time   `json:"created_at"`
	CompletedAt  *time.Time  `json:"completed_at,omitempty"`
}

// 节点状态代表计划节点状态.
type NodeStatus string

const (
	NodeStatusPending   NodeStatus = "pending"
	NodeStatusRunning   NodeStatus = "running"
	NodeStatusCompleted NodeStatus = "completed"
	NodeStatusFailed    NodeStatus = "failed"
	NodeStatusSkipped   NodeStatus = "skipped"
	NodeStatusBacktrack NodeStatus = "backtrack"
)

// Dynamic Planner执行动态规划并进行回溯跟踪.
type DynamicPlanner struct {
	provider     llm.Provider
	toolExecutor tools.ToolExecutor
	toolSchemas  []llm.ToolSchema
	config       DynamicPlannerConfig
	logger       *zap.Logger

	// 状态
	mu          sync.RWMutex
	rootNode    *PlanNode
	currentNode *PlanNode
	backtracks  int
	nodeCounter int
}

// NewDynamic Planner创建了新的动态计划.
func NewDynamicPlanner(provider llm.Provider, executor tools.ToolExecutor, schemas []llm.ToolSchema, config DynamicPlannerConfig, logger *zap.Logger) *DynamicPlanner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DynamicPlanner{
		provider:     provider,
		toolExecutor: executor,
		toolSchemas:  schemas,
		config:       config,
		logger:       logger,
	}
}

func (d *DynamicPlanner) Name() string { return "dynamic_planner" }

// 执行运行动态规划并进行回溯跟踪.
func (d *DynamicPlanner) Execute(ctx context.Context, task string) (*ReasoningResult, error) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, d.config.Timeout)
	defer cancel()

	result := &ReasoningResult{
		Pattern:  d.Name(),
		Task:     task,
		Metadata: make(map[string]any),
	}

	// 初始化根节点
	d.mu.Lock()
	d.rootNode = &PlanNode{
		ID:          d.nextNodeID(),
		Action:      "root",
		Description: task,
		Status:      NodeStatusPending,
		Confidence:  1.0,
		CreatedAt:   time.Now(),
	}
	d.currentNode = d.rootNode
	d.backtracks = 0
	d.mu.Unlock()

	// 生成初始计划
	initialPlan, planTokens, err := d.generateNextSteps(ctx, task, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate initial plan: %w", err)
	}
	result.TotalTokens += planTokens

	d.mu.Lock()
	d.rootNode.Children = initialPlan
	d.mu.Unlock()

	// 以动态调整执行计划
	finalResult, execTokens, err := d.executePlan(ctx, task)
	result.TotalTokens += execTokens

	if err != nil {
		result.Metadata["error"] = err.Error()
	}

	// 从计划树上建立步骤
	result.Steps = d.collectSteps(d.rootNode)
	result.FinalAnswer = finalResult
	result.TotalLatency = time.Since(start)
	result.Metadata["backtracks"] = d.backtracks
	result.Metadata["total_nodes"] = d.nodeCounter

	return result, nil
}

func (d *DynamicPlanner) nextNodeID() string {
	d.nodeCounter++
	return fmt.Sprintf("node_%d", d.nodeCounter)
}

func (d *DynamicPlanner) generateNextSteps(ctx context.Context, task string, currentState *PlanNode) ([]*PlanNode, int, error) {
	var contextInfo string
	if currentState != nil {
		contextInfo = fmt.Sprintf("\nCurrent state: %s\nResult so far: %s", currentState.Description, currentState.Result)
	}

	// 构建工具描述
	var toolDescs []string
	for _, t := range d.toolSchemas {
		toolDescs = append(toolDescs, fmt.Sprintf("- %s: %s", t.Name, t.Description))
	}

	prompt := fmt.Sprintf(`You are a planning agent. Generate the next steps to accomplish the task.
Consider multiple approaches and provide alternatives in case the primary approach fails.

Available tools:
%s

Task: %s%s

Generate 1-3 next steps with alternatives. Output as JSON:
{
  "steps": [
    {
      "action": "tool_name or 'think'",
      "description": "what to do",
      "confidence": 0.8,
      "alternatives": [
        {"action": "alt_tool", "description": "alternative approach", "confidence": 0.6}
      ]
    }
  ]
}`, joinStrings(toolDescs, "\n"), task, contextInfo)

	resp, err := d.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.4,
		MaxTokens:   1500,
	})
	if err != nil {
		return nil, 0, err
	}

	content := extractJSONObject(resp.Choices[0].Message.Content)
	tokens := resp.Usage.TotalTokens

	var planData struct {
		Steps []struct {
			Action       string  `json:"action"`
			Description  string  `json:"description"`
			Confidence   float64 `json:"confidence"`
			Alternatives []struct {
				Action      string  `json:"action"`
				Description string  `json:"description"`
				Confidence  float64 `json:"confidence"`
			} `json:"alternatives"`
		} `json:"steps"`
	}

	if err := json.Unmarshal([]byte(content), &planData); err != nil {
		d.logger.Warn("failed to parse plan", zap.Error(err))
		return nil, tokens, nil
	}

	var nodes []*PlanNode
	for _, step := range planData.Steps {
		node := &PlanNode{
			ID:          d.nextNodeID(),
			Action:      step.Action,
			Description: step.Description,
			Status:      NodeStatusPending,
			Confidence:  step.Confidence,
			CreatedAt:   time.Now(),
		}

		// 添加替代品
		for _, alt := range step.Alternatives {
			altNode := &PlanNode{
				ID:          d.nextNodeID(),
				Action:      alt.Action,
				Description: alt.Description,
				Status:      NodeStatusPending,
				Confidence:  alt.Confidence,
				CreatedAt:   time.Now(),
			}
			node.Alternatives = append(node.Alternatives, altNode)
		}

		nodes = append(nodes, node)
	}

	return nodes, tokens, nil
}

func (d *DynamicPlanner) executePlan(ctx context.Context, task string) (string, int, error) {
	totalTokens := 0
	var lastResult string

	for {
		select {
		case <-ctx.Done():
			return lastResult, totalTokens, ctx.Err()
		default:
		}

		// 查找下一个可执行节点
		node := d.findNextNode()
		if node == nil {
			// 不再有节点,检查是否有结果
			break
		}

		d.logger.Debug("executing node",
			zap.String("id", node.ID),
			zap.String("action", node.Action),
			zap.Float64("confidence", node.Confidence))

		// 检查信任阈值
		if node.Confidence < d.config.ConfidenceThreshold {
			d.logger.Debug("skipping low confidence node", zap.String("id", node.ID))
			node.Status = NodeStatusSkipped
			continue
		}

		// 执行节点
		node.Status = NodeStatusRunning
		result, execTokens, err := d.executeNode(ctx, node)
		totalTokens += execTokens

		if err != nil {
			node.Status = NodeStatusFailed
			node.Error = err.Error()

			// 尝试其它选项或回路
			if !d.tryAlternativeOrBacktrack(ctx, node) {
				d.logger.Warn("no alternatives available, stopping")
				break
			}
			continue
		}

		node.Status = NodeStatusCompleted
		node.Result = result
		now := time.Now()
		node.CompletedAt = &now
		lastResult = result

		// 根据结果生成下一步
		if d.shouldContinue(ctx, task, node) {
			nextSteps, genTokens, err := d.generateNextSteps(ctx, task, node)
			totalTokens += genTokens
			if err == nil && len(nextSteps) > 0 {
				node.Children = nextSteps
			}
		}
	}

	// 合成最终答案
	if lastResult == "" {
		answer, synthTokens, err := d.synthesizeFinalAnswer(ctx, task)
		totalTokens += synthTokens
		if err == nil {
			lastResult = answer
		}
	}

	return lastResult, totalTokens, nil
}

func (d *DynamicPlanner) findNextNode() *PlanNode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.findNextNodeRecursive(d.rootNode)
}

func (d *DynamicPlanner) findNextNodeRecursive(node *PlanNode) *PlanNode {
	if node == nil {
		return nil
	}

	// 先检查孩子( 深度第一 )
	for _, child := range node.Children {
		if child.Status == NodeStatusPending {
			return child
		}
		if found := d.findNextNodeRecursive(child); found != nil {
			return found
		}
	}

	return nil
}

func (d *DynamicPlanner) executeNode(ctx context.Context, node *PlanNode) (string, int, error) {
	if node.Action == "think" || node.Action == "reason" {
		// 思考时使用 LLM
		return d.executeLLMNode(ctx, node)
	}

	// 作为工具执行
	argsJSON, _ := json.Marshal(map[string]string{"input": node.Description})
	call := llm.ToolCall{
		ID:        node.ID,
		Name:      node.Action,
		Arguments: argsJSON,
	}

	results := d.toolExecutor.Execute(ctx, []llm.ToolCall{call})
	if len(results) > 0 {
		if results[0].Error != "" {
			return "", 0, fmt.Errorf("tool error: %s", results[0].Error)
		}
		return string(results[0].Result), 0, nil
	}

	return "", 0, fmt.Errorf("no result from tool")
}

func (d *DynamicPlanner) executeLLMNode(ctx context.Context, node *PlanNode) (string, int, error) {
	prompt := fmt.Sprintf(`Task: %s

Think through this step and provide your reasoning and conclusion.`, node.Description)

	resp, err := d.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   1000,
	})
	if err != nil {
		return "", 0, err
	}

	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}

func (d *DynamicPlanner) tryAlternativeOrBacktrack(ctx context.Context, failedNode *PlanNode) bool {
	// 先试试别的
	for _, alt := range failedNode.Alternatives {
		if alt.Status == NodeStatusPending {
			d.logger.Info("trying alternative",
				zap.String("failed", failedNode.ID),
				zap.String("alternative", alt.ID))

			// 将失败的节点替换为父母子女中的选项
			d.replaceNodeWithAlternative(failedNode, alt)
			return true
		}
	}

	// 没有替代品, 尝试回溯
	d.mu.Lock()
	d.backtracks++
	canBacktrack := d.backtracks <= d.config.MaxBacktracks
	d.mu.Unlock()

	if !canBacktrack {
		d.logger.Warn("max backtracks reached")
		return false
	}

	d.logger.Info("backtracking", zap.Int("count", d.backtracks))
	failedNode.Status = NodeStatusBacktrack

	// 找到父公司并尝试其替代品
	parent := d.findParent(d.rootNode, failedNode.ID)
	if parent != nil {
		for _, alt := range parent.Alternatives {
			if alt.Status == NodeStatusPending {
				d.replaceNodeWithAlternative(parent, alt)
				return true
			}
		}
	}

	return false
}

func (d *DynamicPlanner) replaceNodeWithAlternative(original, alternative *PlanNode) {
	alternative.ParentID = original.ParentID
	original.Status = NodeStatusSkipped
}

func (d *DynamicPlanner) findParent(root *PlanNode, childID string) *PlanNode {
	if root == nil {
		return nil
	}

	for _, child := range root.Children {
		if child.ID == childID {
			return root
		}
		if found := d.findParent(child, childID); found != nil {
			return found
		}
	}

	return nil
}

func (d *DynamicPlanner) shouldContinue(ctx context.Context, task string, node *PlanNode) bool {
	// 检查结果是否显示完成
	if len(node.Result) > 100 && containsCompletionIndicator(node.Result) {
		return false
	}

	// 检查深度
	depth := d.getNodeDepth(node)
	return depth < d.config.MaxPlanDepth
}

func (d *DynamicPlanner) getNodeDepth(node *PlanNode) int {
	depth := 0
	current := node
	for current.ParentID != "" {
		depth++
		current = d.findNodeByID(d.rootNode, current.ParentID)
		if current == nil {
			break
		}
	}
	return depth
}

func (d *DynamicPlanner) findNodeByID(root *PlanNode, id string) *PlanNode {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, child := range root.Children {
		if found := d.findNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}

func (d *DynamicPlanner) synthesizeFinalAnswer(ctx context.Context, task string) (string, int, error) {
	// 收集所有已完成的结果
	var results []string
	d.collectResults(d.rootNode, &results)

	prompt := fmt.Sprintf(`Task: %s

Execution results:
%s

Synthesize a final answer based on these results.`, task, joinStrings(results, "\n"))

	resp, err := d.provider.Completion(ctx, &llm.ChatRequest{
		Model: "gpt-4o",
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   1000,
	})
	if err != nil {
		return "", 0, err
	}

	return resp.Choices[0].Message.Content, resp.Usage.TotalTokens, nil
}

func (d *DynamicPlanner) collectResults(node *PlanNode, results *[]string) {
	if node == nil {
		return
	}
	if node.Status == NodeStatusCompleted && node.Result != "" {
		*results = append(*results, fmt.Sprintf("- %s: %s", node.Description, truncate(node.Result, 200)))
	}
	for _, child := range node.Children {
		d.collectResults(child, results)
	}
}

func (d *DynamicPlanner) collectSteps(node *PlanNode) []ReasoningStep {
	var steps []ReasoningStep
	d.collectStepsRecursive(node, &steps)
	return steps
}

func (d *DynamicPlanner) collectStepsRecursive(node *PlanNode, steps *[]ReasoningStep) {
	if node == nil {
		return
	}

	stepType := "action"
	if node.Status == NodeStatusBacktrack {
		stepType = "backtrack"
	} else if node.Action == "think" || node.Action == "reason" {
		stepType = "thought"
	}

	*steps = append(*steps, ReasoningStep{
		StepID:  node.ID,
		Type:    stepType,
		Content: node.Description,
		Score:   node.Confidence,
	})

	for _, child := range node.Children {
		d.collectStepsRecursive(child, steps)
	}
}

func containsCompletionIndicator(s string) bool {
	indicators := []string{"final answer", "conclusion", "result is", "the answer is", "in summary"}
	lower := toLower(s)
	for _, ind := range indicators {
		if containsString(lower, ind) {
			return true
		}
	}
	return false
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func joinStrings(parts []string, sep string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += sep + parts[i]
	}
	return result
}

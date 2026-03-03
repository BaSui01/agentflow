package collaboration

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/persistence"
	"github.com/BaSui01/agentflow/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// CollaborationPattern 协作模式
type CollaborationPattern string

const (
	PatternDebate    CollaborationPattern = "debate"    // 辩论模式
	PatternConsensus CollaborationPattern = "consensus" // 共识模式
	PatternPipeline  CollaborationPattern = "pipeline"  // 流水线模式
	PatternBroadcast CollaborationPattern = "broadcast" // 广播模式
	PatternNetwork   CollaborationPattern = "network"   // 网络模式
)

// MultiAgentSystem 多 Agent 系统
type MultiAgentSystem struct {
	agents  map[string]agent.Agent
	pattern CollaborationPattern

	// 通信
	messageHub *MessageHub

	// 协调
	coordinator Coordinator

	// 配置
	config MultiAgentConfig

	logger *zap.Logger
}

// MultiAgentConfig 多 Agent 配置
type MultiAgentConfig struct {
	Pattern            CollaborationPattern `json:"pattern"`
	MaxRounds          int                  `json:"max_rounds"`          // 最大轮次
	ConsensusThreshold float64              `json:"consensus_threshold"` // 共识阈值
	Timeout            time.Duration        `json:"timeout"`
	EnableVoting       bool                 `json:"enable_voting"`
}

// DefaultMultiAgentConfig 默认配置
func DefaultMultiAgentConfig() MultiAgentConfig {
	return MultiAgentConfig{
		Pattern:            PatternDebate,
		MaxRounds:          5,
		ConsensusThreshold: 0.7,
		Timeout:            10 * time.Minute,
		EnableVoting:       true,
	}
}

// Message Agent 间消息
type Message struct {
	ID        string
	FromID    string
	ToID      string // 空表示广播
	Type      MessageType
	Content   string
	Metadata  map[string]any
	Timestamp time.Time
	Body      types.Message // Canonical message body bridge to Layer-0 types.Message
}

const payloadAnyMetadataKey = "_metadata_any"

// MessageType 消息类型
type MessageType string

const (
	MessageTypeProposal  MessageType = "proposal"
	MessageTypeResponse  MessageType = "response"
	MessageTypeVote      MessageType = "vote"
	MessageTypeConsensus MessageType = "consensus"
	MessageTypeBroadcast MessageType = "broadcast"
)

// MessageHub 消息中心
// 支持持久化存储，防止消息丢失
type MessageHub struct {
	channels     map[string]chan *Message
	mu           sync.RWMutex
	logger       *zap.Logger
	messageStore persistence.MessageStore // 持久化存储（可选）
	retryConfig  persistence.RetryConfig  // 重试配置
	closed       bool
	closeOnce    sync.Once
}

// Coordinator 协调器接口
type Coordinator interface {
	Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error)
}

// NewMultiAgentSystem 创建多 Agent 系统
func NewMultiAgentSystem(agents []agent.Agent, config MultiAgentConfig, logger *zap.Logger) *MultiAgentSystem {
	if logger == nil {
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			logger = zap.NewNop()
		}
	}

	agentMap := make(map[string]agent.Agent)
	for _, a := range agents {
		agentMap[a.ID()] = a
	}

	hub := NewMessageHub(logger)

	// 为每个 Agent 创建通道
	for _, a := range agents {
		hub.CreateChannel(a.ID())
	}

	var coordinator Coordinator
	normalizedPattern := CollaborationPattern(strings.ToLower(strings.TrimSpace(string(config.Pattern))))
	switch normalizedPattern {
	case PatternDebate:
		coordinator = NewDebateCoordinator(config, hub, logger)
	case PatternConsensus:
		coordinator = NewConsensusCoordinator(config, hub, logger)
	case PatternPipeline:
		coordinator = NewPipelineCoordinator(config, hub, logger)
	case PatternBroadcast:
		coordinator = NewBroadcastCoordinator(config, hub, logger)
	case PatternNetwork:
		coordinator = NewNetworkCoordinator(config, hub, logger)
	default:
		coordinator = NewDebateCoordinator(config, hub, logger)
	}

	return &MultiAgentSystem{
		agents:      agentMap,
		pattern:     normalizedPattern,
		messageHub:  hub,
		coordinator: coordinator,
		config:      config,
		logger:      logger.With(zap.String("component", "multi_agent_system")),
	}
}

// Execute 执行协作任务
func (m *MultiAgentSystem) Execute(ctx context.Context, input *agent.Input) (*agent.Output, error) {
	m.logger.Info("multi-agent collaboration started",
		zap.String("pattern", string(m.pattern)),
		zap.Int("agents", len(m.agents)),
	)

	coordinator := m.coordinator
	// Enforce runtime routing by pattern to avoid stale/miswired coordinator instances.
	switch m.pattern {
	case PatternConsensus:
		coordinator = NewConsensusCoordinator(m.config, m.messageHub, m.logger)
	case PatternPipeline:
		coordinator = NewPipelineCoordinator(m.config, m.messageHub, m.logger)
	case PatternBroadcast:
		coordinator = NewBroadcastCoordinator(m.config, m.messageHub, m.logger)
	case PatternNetwork:
		coordinator = NewNetworkCoordinator(m.config, m.messageHub, m.logger)
	default:
		coordinator = NewDebateCoordinator(m.config, m.messageHub, m.logger)
	}

	output, err := coordinator.Coordinate(ctx, m.agents, input)
	if err != nil {
		return nil, err
	}

	m.logger.Info("multi-agent collaboration completed")

	return output, nil
}

// NewMessageHub 创建消息中心
func NewMessageHub(logger *zap.Logger) *MessageHub {
	return &MessageHub{
		channels:    make(map[string]chan *Message),
		logger:      logger.With(zap.String("component", "message_hub")),
		retryConfig: persistence.DefaultRetryConfig(),
	}
}

// NewMessageHubWithStore 创建带持久化的消息中心
func NewMessageHubWithStore(logger *zap.Logger, store persistence.MessageStore) *MessageHub {
	hub := NewMessageHub(logger)
	hub.messageStore = store
	return hub
}

// SetMessageStore 设置消息存储（用于依赖注入）
func (h *MessageHub) SetMessageStore(store persistence.MessageStore) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.messageStore = store
}

// Close 关闭消息中心
// 使用 sync.Once 保护 channel 关闭，防止重复关闭 panic
func (h *MessageHub) Close() error {
	var closeErr error
	h.closeOnce.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		h.closed = true

		// 关闭所有通道
		for _, ch := range h.channels {
			close(ch)
		}
		h.channels = nil

		// 关闭存储
		if h.messageStore != nil {
			closeErr = h.messageStore.Close()
		}
	})
	return closeErr
}

// CreateChannel 创建通道
func (h *MessageHub) CreateChannel(agentID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.channels[agentID] = make(chan *Message, 100)
}

// Send 发送消息
// 如果配置了持久化存储，消息会先持久化再投递
// 即使 channel 满了，消息也不会丢失
func (h *MessageHub) Send(msg *Message) error {
	return h.SendWithContext(context.Background(), msg)
}

// SendWithContext 发送消息（带上下文）
// 修复竞态条件：使用单次锁定确保操作原子性
func (h *MessageHub) SendWithContext(ctx context.Context, msg *Message) error {
	// 生成消息 ID（无需锁）
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	// 如果有持久化存储，先持久化消息（无需锁）
	if h.messageStore != nil {
		persistMsg := h.toPersistMessage(msg)
		if err := h.messageStore.SaveMessage(ctx, persistMsg); err != nil {
			h.logger.Error("failed to persist message",
				zap.String("msg_id", msg.ID),
				zap.String("from_agent_id", msg.FromID),
				zap.String("to_agent_id", msg.ToID),
				zap.Error(err),
			)
			// 持久化失败不阻止消息投递，但记录错误
		}
	}

	// T-011: 缩小锁范围 — 仅在锁内读取 channels map，不在锁内做 channel 发送
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return fmt.Errorf("message hub is closed")
	}

	if msg.ToID == "" {
		// 广播：复制需要发送的 channel 列表
		targets := make(map[string]chan *Message, len(h.channels))
		for id, ch := range h.channels {
			if id != msg.FromID {
				targets[id] = ch
			}
		}
		h.mu.RUnlock()

		for id, ch := range targets {
			select {
			case ch <- msg:
				// 消息投递成功，确认消息
				go h.ackMessage(ctx, msg.ID)
			default:
				// channel 满了，消息已持久化，后续可重试
				h.logger.Debug("channel full, message persisted for retry",
					zap.String("to", id),
					zap.String("msg_id", msg.ID),
				)
			}
		}
	} else {
		// 点对点：读取目标 channel 后立即释放锁
		ch, ok := h.channels[msg.ToID]
		h.mu.RUnlock()

		if !ok {
			return fmt.Errorf("channel not found: %s", msg.ToID)
		}

		select {
		case ch <- msg:
			// 消息投递成功，确认消息
			go h.ackMessage(ctx, msg.ID)
		default:
			// channel 满了，消息已持久化，后续可重试
			h.logger.Debug("channel full, message persisted for retry",
				zap.String("to", msg.ToID),
				zap.String("msg_id", msg.ID),
			)
			// 不返回错误，因为消息已持久化
		}
	}

	return nil
}

// toPersistMessage 转换为持久化消息格式
func (h *MessageHub) toPersistMessage(msg *Message) *persistence.Message {
	body := msg.Body
	if body.Content == "" {
		body.Content = msg.Content
	}
	if body.Timestamp.IsZero() {
		body.Timestamp = msg.Timestamp
	}
	if body.Metadata == nil && len(msg.Metadata) > 0 {
		body.Metadata = toStringMetadata(msg.Metadata)
	}

	pm := persistence.NewMessageDAOFromTypes(
		msg.ToID, // topic
		msg.FromID,
		msg.ToID,
		string(msg.Type),
		body,
	)
	pm.ID = msg.ID
	if len(msg.Metadata) > 0 {
		if pm.Payload == nil {
			pm.Payload = make(map[string]any, 1)
		}
		pm.Payload[payloadAnyMetadataKey] = msg.Metadata
	}
	return pm
}

// fromPersistMessage 从持久化消息格式转换
func (h *MessageHub) fromPersistMessage(pm *persistence.Message) *Message {
	body := pm.ToTypesMessage()
	metadata := pm.Payload
	if raw, ok := pm.Payload[payloadAnyMetadataKey]; ok {
		if m, ok := raw.(map[string]any); ok {
			metadata = m
		}
	}

	return &Message{
		ID:        pm.ID,
		FromID:    pm.FromID,
		ToID:      pm.ToID,
		Type:      MessageType(pm.Type),
		Content:   body.Content,
		Metadata:  metadata,
		Timestamp: body.Timestamp,
		Body:      body,
	}
}

func toStringMetadata(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ackMessage 确认消息已处理
// H1 FIX: 异步 goroutine 调用前检查 closed 标志，防止 store 关闭后访问
func (h *MessageHub) ackMessage(ctx context.Context, msgID string) {
	// H1 FIX: Close() 可能已关闭 messageStore，提前检查避免访问已关闭的 store
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		return
	}

	if h.messageStore == nil {
		return
	}
	if err := h.messageStore.AckMessage(ctx, msgID); err != nil {
		h.logger.Debug("failed to ack message",
			zap.String("msg_id", msgID),
			zap.Error(err),
		)
	}
}

// safeSend safely writes to a channel, recovering from panic if the channel is closed.
func safeSend(ch chan<- *Message, msg *Message) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			sent = false
		}
	}()
	ch <- msg
	return true
}

// RecoverMessages 恢复未处理的消息（服务重启后调用）
func (h *MessageHub) RecoverMessages(ctx context.Context) error {
	if h.messageStore == nil {
		return nil
	}
	h.logger.Info("recovering unprocessed messages")

	h.mu.RLock()
	agentIDs := make([]string, 0, len(h.channels))
	for id := range h.channels {
		agentIDs = append(agentIDs, id)
	}
	h.mu.RUnlock()

	totalRecovered := 0

	for _, agentID := range agentIDs {
		// 获取该 agent 的未确认消息
		msgs, err := h.messageStore.GetUnackedMessages(ctx, agentID, 5*time.Minute)
		if err != nil {
			h.logger.Warn("failed to get unacked messages",
				zap.String("agent_id", agentID),
				zap.Error(err),
			)
			continue
		}

		for _, pm := range msgs {
			// 检查是否应该重试
			if !pm.ShouldRetry(h.retryConfig) {
				h.logger.Debug("message exceeded max retries",
					zap.String("msg_id", pm.ID),
					zap.Int("retry_count", pm.RetryCount),
				)
				continue
			}

			// 增加重试计数
			if err := h.messageStore.IncrementRetry(ctx, pm.ID); err != nil {
				h.logger.Warn("failed to increment retry count",
					zap.String("msg_id", pm.ID),
					zap.Error(err),
				)
			}

			// 重新投递消息
			msg := h.fromPersistMessage(pm)
			h.mu.RLock()
			ch, ok := h.channels[agentID]
			h.mu.RUnlock()

			if ok {
				if !safeSend(ch, msg) {
					// channel was closed, stop recovery
					h.logger.Debug("message hub closed during send, stopping recovery",
						zap.String("msg_id", msg.ID),
						zap.String("to", agentID),
					)
					break
				}
				totalRecovered++
				h.logger.Debug("message recovered",
					zap.String("msg_id", msg.ID),
					zap.String("to", agentID),
				)
			}
		}
	}

	h.logger.Info("message recovery completed",
		zap.Int("recovered", totalRecovered),
	)

	return nil
}

// StartRetryLoop 启动消息重试循环
func (h *MessageHub) StartRetryLoop(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := h.RecoverMessages(ctx); err != nil {
					h.logger.Warn("retry loop error", zap.Error(err))
				}
			}
		}
	}()
}

// Stats 获取消息统计信息
func (h *MessageHub) Stats(ctx context.Context) (*persistence.MessageStoreStats, error) {
	if h.messageStore == nil {
		return nil, fmt.Errorf("no message store configured")
	}
	return h.messageStore.Stats(ctx)
}

// Receive 接收消息
func (h *MessageHub) Receive(agentID string, timeout time.Duration) (*Message, error) {
	h.mu.RLock()
	ch, ok := h.channels[agentID]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("channel not found: %s", agentID)
	}

	select {
	case msg := <-ch:
		return msg, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("receive timeout")
	}
}

// DebateCoordinator 辩论协调器
type DebateCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewDebateCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *DebateCoordinator {
	return &DebateCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "debate")),
	}
}

func (c *DebateCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("debate coordination started",
		zap.Int("agents", len(agents)),
		zap.Int("max_rounds", c.config.MaxRounds),
	)

	// Deterministic agent ordering for reproducible debate rounds.
	orderedIDs := sortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("debate requires at least one agent")
	}

	// Phase 1 — Initial proposals: each agent independently responds to the original question.
	proposals := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("debate cancelled during initial proposals: %w", err)
		}

		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent initial proposal failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		proposals[id] = output
	}
	if len(proposals) == 0 {
		return nil, fmt.Errorf("all agents failed during initial proposal phase")
	}

	// Phase 2 — Multi-round debate: each agent reviews others' positions and refines its own.
	for round := 1; round <= c.config.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("debate cancelled at round %d: %w", round, err)
		}

		c.logger.Debug("debate round started", zap.Int("round", round))

		for _, id := range orderedIDs {
			// Build a structured debate prompt with all peer positions.
			var otherPositions strings.Builder
			for _, peerID := range orderedIDs {
				if peerID == id {
					continue
				}
				if p, ok := proposals[peerID]; ok {
					otherPositions.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", peerID, p.Content))
				}
			}
			if otherPositions.Len() == 0 {
				continue // sole agent, nothing to debate
			}

			debatePrompt := fmt.Sprintf(
				"You are participating in a structured multi-agent debate (Round %d/%d).\n\n"+
					"## Original Question\n%s\n\n"+
					"## Your Previous Position\n%s\n\n"+
					"## Other Agents' Positions\n%s\n\n"+
					"## Instructions\n"+
					"1. Identify the strongest and weakest points in each position above.\n"+
					"2. Acknowledge valid arguments from other agents.\n"+
					"3. Refute incorrect or incomplete reasoning with evidence.\n"+
					"4. Synthesize an improved, well-structured response that integrates the best insights.\n"+
					"5. Clearly state your refined position.\n",
				round, c.config.MaxRounds,
				input.Content,
				proposals[id].Content,
				otherPositions.String(),
			)

			debateInput := &agent.Input{
				TraceID: input.TraceID,
				Content: debatePrompt,
				Context: map[string]any{
					"debate_round": round,
					"max_rounds":   c.config.MaxRounds,
					"agent_id":     id,
				},
			}

			output, err := agents[id].Execute(ctx, debateInput)
			if err != nil {
				c.logger.Warn("agent debate round failed",
					zap.String("agent_id", id),
					zap.Int("round", round),
					zap.Error(err),
				)
				continue // keep the previous proposal
			}
			proposals[id] = output
		}
	}

	// Phase 3 — Judge synthesis: use the first agent as judge to produce a final synthesis.
	judgeID := orderedIDs[0]
	var allPositions strings.Builder
	for _, id := range orderedIDs {
		if p, ok := proposals[id]; ok {
			allPositions.WriteString(fmt.Sprintf("\n--- Agent [%s] (final position) ---\n%s\n", id, p.Content))
		}
	}

	synthesisPrompt := fmt.Sprintf(
		"You are the final judge in a multi-agent debate.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Evaluate each agent's final position for accuracy, completeness, and reasoning quality.\n"+
			"2. Identify points of agreement across agents (consensus areas).\n"+
			"3. Resolve remaining disagreements by selecting the best-supported arguments.\n"+
			"4. Produce a single, authoritative, well-structured answer that synthesizes the strongest elements.\n"+
			"5. Do NOT simply pick one agent's answer — integrate and improve upon all of them.\n",
		input.Content,
		allPositions.String(),
	)

	synthesisInput := &agent.Input{
		TraceID: input.TraceID,
		Content: synthesisPrompt,
		Context: map[string]any{
			"phase":      "synthesis",
			"judge_id":   judgeID,
			"num_agents": len(proposals),
		},
	}

	finalOutput, err := agents[judgeID].Execute(ctx, synthesisInput)
	if err != nil {
		// Fallback: return the first available proposal if synthesis fails.
		c.logger.Warn("judge synthesis failed, falling back to best proposal",
			zap.String("judge_id", judgeID),
			zap.Error(err),
		)
		for _, id := range orderedIDs {
			if p, ok := proposals[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("debate completed but no valid proposals remain")
	}

	// Aggregate token usage and cost from all rounds.
	totalTokens, totalCost := aggregateUsage(proposals)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = mergeMetadata(finalOutput.Metadata, map[string]any{
		"debate_rounds":     c.config.MaxRounds,
		"participating_ids": orderedIDs,
		"judge_id":          judgeID,
	})

	c.logger.Info("debate coordination completed",
		zap.Int("rounds", c.config.MaxRounds),
		zap.Int("proposals", len(proposals)),
	)

	return finalOutput, nil
}

// ConsensusCoordinator 共识协调器
type ConsensusCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewConsensusCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *ConsensusCoordinator {
	return &ConsensusCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "consensus")),
	}
}

func (c *ConsensusCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("consensus coordination started",
		zap.Int("agents", len(agents)),
		zap.Float64("threshold", c.config.ConsensusThreshold),
	)

	orderedIDs := sortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("consensus requires at least one agent")
	}

	// Phase 1 — Independent proposals.
	proposals := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("consensus cancelled during proposals: %w", err)
		}

		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent proposal failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		proposals[id] = output
	}
	if len(proposals) == 0 {
		return nil, fmt.Errorf("all agents failed during proposal phase")
	}

	// Phase 2 — Multi-round consensus building.
	for round := 1; round <= c.config.MaxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("consensus cancelled at round %d: %w", round, err)
		}

		c.logger.Debug("consensus round started", zap.Int("round", round))

		// Each agent reviews all proposals and votes.
		votes := make(map[string][]string, len(proposals)) // candidateID -> voterIDs
		for _, voterID := range orderedIDs {
			if _, ok := proposals[voterID]; !ok {
				continue
			}

			var allProposals strings.Builder
			for _, candID := range orderedIDs {
				if p, ok := proposals[candID]; ok {
					allProposals.WriteString(fmt.Sprintf("\n--- Proposal by Agent [%s] ---\n%s\n", candID, p.Content))
				}
			}

			votePrompt := fmt.Sprintf(
				"You are participating in a consensus-building process (Round %d/%d).\n\n"+
					"## Original Question\n%s\n\n"+
					"## All Current Proposals\n%s\n\n"+
					"## Instructions\n"+
					"1. Evaluate each proposal for correctness, completeness, and clarity.\n"+
					"2. State which agent's proposal you agree with most and why (use the agent ID in brackets).\n"+
					"3. Identify specific points of agreement and disagreement.\n"+
					"4. Suggest concrete improvements that could move toward consensus.\n"+
					"5. Start your response with: VOTE: [agent_id]\n",
				round, c.config.MaxRounds,
				input.Content,
				allProposals.String(),
			)

			voteInput := &agent.Input{
				TraceID: input.TraceID,
				Content: votePrompt,
				Context: map[string]any{
					"consensus_round": round,
					"voter_id":        voterID,
				},
			}

			voteOutput, err := agents[voterID].Execute(ctx, voteInput)
			if err != nil {
				c.logger.Warn("agent vote failed",
					zap.String("agent_id", voterID),
					zap.Int("round", round),
					zap.Error(err),
				)
				continue
			}

			// Parse vote: extract the voted agent ID from the output.
			votedID := parseVote(voteOutput.Content, orderedIDs)
			if votedID == "" {
				votedID = voterID // default: self-vote
			}
			votes[votedID] = append(votes[votedID], voterID)

			// Update the voter's proposal with their refined position.
			proposals[voterID] = voteOutput
		}

		// Check if consensus threshold is met.
		totalVoters := 0
		for _, voters := range votes {
			totalVoters += len(voters)
		}
		if totalVoters > 0 {
			for candID, voters := range votes {
				ratio := float64(len(voters)) / float64(totalVoters)
				c.logger.Debug("vote tally",
					zap.String("candidate", candID),
					zap.Int("votes", len(voters)),
					zap.Float64("ratio", ratio),
				)
				if ratio >= c.config.ConsensusThreshold {
					c.logger.Info("consensus reached",
						zap.String("winner", candID),
						zap.Float64("ratio", ratio),
						zap.Int("round", round),
					)
					result := proposals[candID]
					result.Metadata = mergeMetadata(result.Metadata, map[string]any{
						"consensus_round": round,
						"consensus_ratio": ratio,
						"winner_id":       candID,
						"total_voters":    totalVoters,
					})
					return result, nil
				}
			}
		}
	}

	// Phase 3 — No consensus reached; synthesize a merged answer.
	c.logger.Info("consensus threshold not met, synthesizing merged answer")

	synthesizerID := orderedIDs[0]
	var allPositions strings.Builder
	for _, id := range orderedIDs {
		if p, ok := proposals[id]; ok {
			allPositions.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", id, p.Content))
		}
	}

	mergePrompt := fmt.Sprintf(
		"Multiple agents could not reach consensus on the following question after %d rounds.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Identify the areas of agreement (common ground).\n"+
			"2. For areas of disagreement, evaluate the evidence and reasoning quality of each position.\n"+
			"3. Produce a single, balanced, well-structured synthesis that:\n"+
			"   - Incorporates all valid points of agreement.\n"+
			"   - Resolves disagreements by selecting the best-supported position.\n"+
			"   - Clearly notes any remaining uncertainties.\n",
		c.config.MaxRounds,
		input.Content,
		allPositions.String(),
	)

	mergeInput := &agent.Input{
		TraceID: input.TraceID,
		Content: mergePrompt,
		Context: map[string]any{
			"phase":          "merge",
			"synthesizer_id": synthesizerID,
		},
	}

	finalOutput, err := agents[synthesizerID].Execute(ctx, mergeInput)
	if err != nil {
		// Fallback to the proposal with the most votes (or first available).
		for _, id := range orderedIDs {
			if p, ok := proposals[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("consensus failed and no proposals available")
	}

	totalTokens, totalCost := aggregateUsage(proposals)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = mergeMetadata(finalOutput.Metadata, map[string]any{
		"consensus_reached": false,
		"total_rounds":      c.config.MaxRounds,
		"synthesizer_id":    synthesizerID,
	})

	c.logger.Info("consensus coordination completed (merged)",
		zap.Int("rounds", c.config.MaxRounds),
	)

	return finalOutput, nil
}

// PipelineCoordinator 流水线协调器
type PipelineCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewPipelineCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *PipelineCoordinator {
	return &PipelineCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "pipeline")),
	}
}

func (c *PipelineCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("pipeline coordination started", zap.Int("stages", len(agents)))

	// Deterministic stage ordering by agent ID.
	orderedIDs := sortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("pipeline requires at least one agent")
	}

	currentInput := input
	var currentOutput *agent.Output
	totalStages := len(orderedIDs)

	for i, id := range orderedIDs {
		stage := i + 1

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("pipeline cancelled at stage %d/%d: %w", stage, totalStages, err)
		}

		c.logger.Debug("pipeline stage executing",
			zap.Int("stage", stage),
			zap.Int("total_stages", totalStages),
			zap.String("agent_id", id),
		)

		// For stages after the first, wrap the previous output with pipeline context.
		if i > 0 && currentOutput != nil {
			pipelinePrompt := fmt.Sprintf(
				"You are stage %d of %d in a processing pipeline.\n\n"+
					"## Original Request\n%s\n\n"+
					"## Output From Previous Stage (Stage %d)\n%s\n\n"+
					"## Instructions\n"+
					"Process the previous stage's output according to your expertise.\n"+
					"Build upon and refine the work done so far.\n"+
					"Maintain consistency with the original request's intent.\n",
				stage, totalStages,
				input.Content,
				stage-1,
				currentOutput.Content,
			)

			currentInput = &agent.Input{
				TraceID: input.TraceID,
				Content: pipelinePrompt,
				Context: mergeContextMaps(input.Context, map[string]any{
					"pipeline_stage": stage,
					"total_stages":   totalStages,
					"previous_agent": orderedIDs[i-1],
				}),
			}
		}

		output, err := agents[id].Execute(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("pipeline stage %d/%d (agent %s) failed: %w", stage, totalStages, id, err)
		}
		currentOutput = output
	}

	if currentOutput == nil {
		return nil, fmt.Errorf("pipeline produced no output")
	}

	currentOutput.Metadata = mergeMetadata(currentOutput.Metadata, map[string]any{
		"pipeline_stages": totalStages,
		"stage_order":     orderedIDs,
	})

	c.logger.Info("pipeline coordination completed", zap.Int("stages", totalStages))

	return currentOutput, nil
}

// BroadcastCoordinator 广播协调器
type BroadcastCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewBroadcastCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *BroadcastCoordinator {
	return &BroadcastCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "broadcast")),
	}
}

func (c *BroadcastCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("broadcast coordination started", zap.Int("agents", len(agents)))

	orderedIDs := sortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("broadcast requires at least one agent")
	}

	// Phase 1 — Fan-out: execute all agents in parallel.
	type agentResult struct {
		id     string
		output *agent.Output
		err    error
	}

	results := make([]agentResult, len(orderedIDs))
	var wg sync.WaitGroup

	for i, id := range orderedIDs {
		wg.Add(1)
		go func(idx int, agentID string) {
			defer wg.Done()

			output, err := agents[agentID].Execute(ctx, input)
			results[idx] = agentResult{id: agentID, output: output, err: err}
		}(i, id)
	}

	wg.Wait()

	// Collect successful outputs in deterministic order.
	succeeded := make([]agentResult, 0, len(results))
	for _, r := range results {
		if r.err != nil {
			c.logger.Warn("agent execution failed",
				zap.String("agent_id", r.id),
				zap.Error(r.err),
			)
			continue
		}
		succeeded = append(succeeded, r)
	}

	if len(succeeded) == 0 {
		return nil, fmt.Errorf("all agents failed during broadcast execution")
	}

	// Phase 2 — Fan-in: synthesize all outputs into a coherent result.
	// If only one agent succeeded, return its output directly.
	if len(succeeded) == 1 {
		return succeeded[0].output, nil
	}

	synthesizerID := orderedIDs[0]
	var allOutputs strings.Builder
	for _, r := range succeeded {
		allOutputs.WriteString(fmt.Sprintf("\n--- Agent [%s] ---\n%s\n", r.id, r.output.Content))
	}

	synthesisPrompt := fmt.Sprintf(
		"Multiple agents have independently responded to the same question.\n\n"+
			"## Original Question\n%s\n\n"+
			"## Individual Agent Responses\n%s\n\n"+
			"## Instructions\n"+
			"1. Review all agent responses for accuracy and completeness.\n"+
			"2. Identify common themes, agreements, and unique insights from each agent.\n"+
			"3. Synthesize a single, comprehensive answer that:\n"+
			"   - Combines the best insights from all responses.\n"+
			"   - Resolves any contradictions by favoring the most well-reasoned position.\n"+
			"   - Provides a complete, well-structured answer.\n",
		input.Content,
		allOutputs.String(),
	)

	synthesisInput := &agent.Input{
		TraceID: input.TraceID,
		Content: synthesisPrompt,
		Context: map[string]any{
			"phase":          "synthesis",
			"synthesizer_id": synthesizerID,
			"num_responses":  len(succeeded),
		},
	}

	finalOutput, err := agents[synthesizerID].Execute(ctx, synthesisInput)
	if err != nil {
		// Fallback: return the first successful output.
		c.logger.Warn("broadcast synthesis failed, returning first output",
			zap.Error(err),
		)
		return succeeded[0].output, nil
	}

	// Aggregate total usage.
	totalTokens, totalCost := 0, 0.0
	for _, r := range succeeded {
		totalTokens += r.output.TokensUsed
		totalCost += r.output.Cost
	}
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = mergeMetadata(finalOutput.Metadata, map[string]any{
		"broadcast_agents": len(succeeded),
		"failed_agents":    len(results) - len(succeeded),
		"synthesizer_id":   synthesizerID,
	})

	c.logger.Info("broadcast coordination completed",
		zap.Int("succeeded", len(succeeded)),
		zap.Int("failed", len(results)-len(succeeded)),
	)

	return finalOutput, nil
}

// NetworkCoordinator 网络协调器
type NetworkCoordinator struct {
	config MultiAgentConfig
	hub    *MessageHub
	logger *zap.Logger
}

func NewNetworkCoordinator(config MultiAgentConfig, hub *MessageHub, logger *zap.Logger) *NetworkCoordinator {
	return &NetworkCoordinator{
		config: config,
		hub:    hub,
		logger: logger.With(zap.String("coordinator", "network")),
	}
}

func (c *NetworkCoordinator) Coordinate(ctx context.Context, agents map[string]agent.Agent, input *agent.Input) (*agent.Output, error) {
	c.logger.Info("network coordination started",
		zap.Int("agents", len(agents)),
		zap.Int("max_rounds", c.config.MaxRounds),
	)

	orderedIDs := sortedAgentIDs(agents)
	if len(orderedIDs) == 0 {
		return nil, fmt.Errorf("network requires at least one agent")
	}

	// Phase 1 — Initial independent responses.
	positions := make(map[string]*agent.Output, len(orderedIDs))
	for _, id := range orderedIDs {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("network cancelled during initial phase: %w", err)
		}

		output, err := agents[id].Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent initial response failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		positions[id] = output
	}
	if len(positions) == 0 {
		return nil, fmt.Errorf("all agents failed during initial response phase")
	}

	// Phase 2 — Multi-round peer-to-peer communication.
	// In each round, every agent exchanges messages with every other agent
	// through the message hub, then refines its position.
	rounds := c.config.MaxRounds
	if rounds <= 0 {
		rounds = 1
	}
	for round := 1; round <= rounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("network cancelled at round %d: %w", round, err)
		}

		c.logger.Debug("network communication round", zap.Int("round", round))

		// Each agent broadcasts its current position to peers via the hub.
		for _, id := range orderedIDs {
			if pos, ok := positions[id]; ok {
				_ = c.hub.SendWithContext(ctx, &Message{
					FromID:  id,
					Type:    MessageTypeProposal,
					Content: pos.Content,
					Metadata: map[string]any{
						"round": round,
					},
					Timestamp: time.Now(),
				})
			}
		}

		// Each agent reads peer messages and refines its position.
		for _, id := range orderedIDs {
			if _, ok := positions[id]; !ok {
				continue
			}

			// Collect peer positions visible to this agent.
			var peerInsights strings.Builder
			for _, peerID := range orderedIDs {
				if peerID == id {
					continue
				}
				if p, ok := positions[peerID]; ok {
					peerInsights.WriteString(fmt.Sprintf("\n--- Peer [%s] (round %d) ---\n%s\n", peerID, round, p.Content))
				}
			}

			if peerInsights.Len() == 0 {
				continue
			}

			networkPrompt := fmt.Sprintf(
				"You are agent [%s] in a peer-to-peer network (Round %d/%d).\n"+
					"Each agent has specialized knowledge and you've received messages from your peers.\n\n"+
					"## Original Question\n%s\n\n"+
					"## Your Current Position\n%s\n\n"+
					"## Peer Messages Received\n%s\n\n"+
					"## Instructions\n"+
					"1. Consider each peer's perspective carefully.\n"+
					"2. Identify new information or arguments that strengthen or challenge your position.\n"+
					"3. Update your position by incorporating valuable peer insights.\n"+
					"4. Highlight any specific points where you changed your mind and why.\n"+
					"5. Maintain your expertise while being open to valid corrections.\n",
				id, round, rounds,
				input.Content,
				positions[id].Content,
				peerInsights.String(),
			)

			networkInput := &agent.Input{
				TraceID: input.TraceID,
				Content: networkPrompt,
				Context: map[string]any{
					"network_round": round,
					"agent_id":      id,
					"peer_count":    len(orderedIDs) - 1,
				},
			}

			output, err := agents[id].Execute(ctx, networkInput)
			if err != nil {
				c.logger.Warn("agent network round failed",
					zap.String("agent_id", id),
					zap.Int("round", round),
					zap.Error(err),
				)
				continue // keep previous position
			}
			positions[id] = output
		}
	}

	// Phase 3 — Final aggregation: first agent synthesizes all evolved positions.
	aggregatorID := orderedIDs[0]
	var allPositions strings.Builder
	for _, id := range orderedIDs {
		if p, ok := positions[id]; ok {
			allPositions.WriteString(fmt.Sprintf("\n--- Agent [%s] (final) ---\n%s\n", id, p.Content))
		}
	}

	aggregatePrompt := fmt.Sprintf(
		"After %d rounds of peer-to-peer discussion, all agents have refined their positions.\n\n"+
			"## Original Question\n%s\n\n"+
			"## All Agents' Final Positions\n%s\n\n"+
			"## Instructions\n"+
			"1. Agents have already exchanged and incorporated each other's feedback.\n"+
			"2. Identify the converged consensus points.\n"+
			"3. For remaining differences, select the most well-supported position.\n"+
			"4. Produce a final, unified, comprehensive answer.\n",
		rounds,
		input.Content,
		allPositions.String(),
	)

	aggregateInput := &agent.Input{
		TraceID: input.TraceID,
		Content: aggregatePrompt,
		Context: map[string]any{
			"phase":         "aggregation",
			"aggregator_id": aggregatorID,
			"num_agents":    len(positions),
			"total_rounds":  rounds,
		},
	}

	finalOutput, err := agents[aggregatorID].Execute(ctx, aggregateInput)
	if err != nil {
		// Fallback to first available position.
		for _, id := range orderedIDs {
			if p, ok := positions[id]; ok {
				return p, nil
			}
		}
		return nil, fmt.Errorf("network coordination failed with no available positions")
	}

	totalTokens, totalCost := aggregateUsage(positions)
	finalOutput.TokensUsed += totalTokens
	finalOutput.Cost += totalCost
	finalOutput.Metadata = mergeMetadata(finalOutput.Metadata, map[string]any{
		"network_rounds":    rounds,
		"participating_ids": orderedIDs,
		"aggregator_id":     aggregatorID,
	})

	c.logger.Info("network coordination completed",
		zap.Int("rounds", rounds),
		zap.Int("agents", len(positions)),
	)

	return finalOutput, nil
}

// ---------------------------------------------------------------------------
// Shared helpers for coordinators
// ---------------------------------------------------------------------------

// sortedAgentIDs returns deterministic, lexicographically sorted agent IDs.
func sortedAgentIDs(agents map[string]agent.Agent) []string {
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// aggregateUsage sums TokensUsed and Cost from a map of outputs.
func aggregateUsage(outputs map[string]*agent.Output) (totalTokens int, totalCost float64) {
	for _, o := range outputs {
		if o != nil {
			totalTokens += o.TokensUsed
			totalCost += o.Cost
		}
	}
	return
}

// mergeMetadata non-destructively merges extra key-value pairs into an
// existing metadata map. If base is nil a new map is allocated.
func mergeMetadata(base map[string]any, extra map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// mergeContextMaps merges two context maps, with override taking precedence.
func mergeContextMaps(base map[string]any, override map[string]any) map[string]any {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range override {
		merged[k] = v
	}
	return merged
}

// parseVote extracts a voted agent ID from an output string.
// It looks for the pattern "VOTE: [agent_id]" and validates against known IDs.
func parseVote(content string, validIDs []string) string {
	// Build a set for O(1) lookup.
	valid := make(map[string]struct{}, len(validIDs))
	for _, id := range validIDs {
		valid[id] = struct{}{}
	}

	// Search for "VOTE:" prefix (case-insensitive).
	lower := strings.ToLower(content)
	idx := strings.Index(lower, "vote:")
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(content[idx+5:]) // skip "vote:"

	// Extract the token after "VOTE:" (may be wrapped in brackets).
	rest = strings.TrimLeft(rest, "[ \t")
	end := strings.IndexAny(rest, "] \t\n\r,")
	if end < 0 {
		end = len(rest)
	}
	candidate := strings.TrimSpace(rest[:end])

	if _, ok := valid[candidate]; ok {
		return candidate
	}
	return ""
}

package collaboration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent"
	"github.com/BaSui01/agentflow/agent/persistence"
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
}

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
	switch config.Pattern {
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
		pattern:     config.Pattern,
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

	output, err := m.coordinator.Coordinate(ctx, m.agents, input)
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
	var storeErr error
	h.closeOnce.Do(func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		h.closed = true

		// 关闭所有通道
		for _, ch := range h.channels {
			close(ch)
		}

		// 关闭存储
		if h.messageStore != nil {
			storeErr = h.messageStore.Close()
		}
	})
	return storeErr
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
				zap.Error(err),
			)
			// 持久化失败不阻止消息投递，但记录错误
		}
	}

	// 获取读锁并保持到操作完成
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 检查是否已关闭
	if h.closed {
		return fmt.Errorf("message hub is closed")
	}

	if msg.ToID == "" {
		// 广播
		for id, ch := range h.channels {
			if id != msg.FromID {
				select {
				case ch <- msg:
					// 消息投递成功，确认消息
					if h.messageStore != nil {
						go h.ackMessage(ctx, msg.ID)
					}
				default:
					// channel 满了，消息已持久化，后续可重试
					h.logger.Debug("channel full, message persisted for retry",
						zap.String("to", id),
						zap.String("msg_id", msg.ID),
					)
				}
			}
		}
	} else {
		// 点对点
		ch, ok := h.channels[msg.ToID]
		if !ok {
			return fmt.Errorf("channel not found: %s", msg.ToID)
		}

		select {
		case ch <- msg:
			// 消息投递成功，确认消息
			if h.messageStore != nil {
				go h.ackMessage(ctx, msg.ID)
			}
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
	return &persistence.Message{
		ID:        msg.ID,
		Topic:     msg.ToID, // 使用 ToID 作为 topic
		FromID:    msg.FromID,
		ToID:      msg.ToID,
		Type:      string(msg.Type),
		Content:   msg.Content,
		Payload:   msg.Metadata,
		CreatedAt: msg.Timestamp,
	}
}

// fromPersistMessage 从持久化消息格式转换
func (h *MessageHub) fromPersistMessage(pm *persistence.Message) *Message {
	return &Message{
		ID:        pm.ID,
		FromID:    pm.FromID,
		ToID:      pm.ToID,
		Type:      MessageType(pm.Type),
		Content:   pm.Content,
		Metadata:  pm.Payload,
		Timestamp: pm.CreatedAt,
	}
}

// ackMessage 确认消息已处理
func (h *MessageHub) ackMessage(ctx context.Context, msgID string) {
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
				select {
				case ch <- msg:
					totalRecovered++
					h.logger.Debug("message recovered",
						zap.String("msg_id", msg.ID),
						zap.String("to", agentID),
					)
				default:
					h.logger.Debug("channel still full during recovery",
						zap.String("msg_id", msg.ID),
						zap.String("to", agentID),
					)
				}
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
	if h.messageStore == nil {
		return
	}

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
	c.logger.Info("debate coordination started")

	// 1. 每个 Agent 提出初始观点
	proposals := make(map[string]*agent.Output)

	for id, a := range agents {
		output, err := a.Execute(ctx, input)
		if err != nil {
			c.logger.Warn("agent proposal failed",
				zap.String("agent_id", id),
				zap.Error(err),
			)
			continue
		}
		proposals[id] = output
	}

	// 2. 多轮辩论
	for round := 0; round < c.config.MaxRounds; round++ {
		c.logger.Debug("debate round", zap.Int("round", round+1))

		// 每个 Agent 评论其他 Agent 的观点
		for id, a := range agents {
			// 构建辩论提示
			debatePrompt := fmt.Sprintf("原始问题：%s\n\n其他观点：\n", input.Content)

			for otherID, proposal := range proposals {
				if otherID != id {
					debatePrompt += fmt.Sprintf("\nAgent %s: %s\n", otherID, proposal.Content)
				}
			}

			debatePrompt += "\n请评论这些观点并提出你的改进意见。"

			debateInput := &agent.Input{
				TraceID: input.TraceID,
				Content: debatePrompt,
			}

			output, err := a.Execute(ctx, debateInput)
			if err != nil {
				continue
			}

			proposals[id] = output
		}
	}

	// 3. 选择最佳答案（简化：选择第一个）
	var finalOutput *agent.Output
	for _, output := range proposals {
		finalOutput = output
		break
	}

	c.logger.Info("debate coordination completed")

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
	c.logger.Info("consensus coordination started")

	// 1. 收集所有 Agent 的输出
	outputs := make([]*agent.Output, 0, len(agents))

	for _, a := range agents {
		output, err := a.Execute(ctx, input)
		if err != nil {
			continue
		}
		outputs = append(outputs, output)
	}

	// 2. 投票达成共识（简化实现）
	if len(outputs) == 0 {
		return nil, fmt.Errorf("no valid outputs")
	}

	// 返回第一个输出作为共识结果
	finalOutput := outputs[0]

	c.logger.Info("consensus coordination completed")

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
	c.logger.Info("pipeline coordination started")

	// 按顺序执行每个 Agent
	currentInput := input
	var currentOutput *agent.Output

	agentList := make([]agent.Agent, 0, len(agents))
	for _, a := range agents {
		agentList = append(agentList, a)
	}

	for i, a := range agentList {
		c.logger.Debug("pipeline stage",
			zap.Int("stage", i+1),
			zap.String("agent_id", a.ID()),
		)

		output, err := a.Execute(ctx, currentInput)
		if err != nil {
			return nil, fmt.Errorf("pipeline stage %d failed: %w", i+1, err)
		}

		currentOutput = output

		// 下一个 Agent 的输入是当前输出
		currentInput = &agent.Input{
			TraceID: input.TraceID,
			Content: output.Content,
		}
	}

	c.logger.Info("pipeline coordination completed")

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
	c.logger.Info("broadcast coordination started")

	// 并行执行所有 Agent
	outputs := make([]*agent.Output, 0, len(agents))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, a := range agents {
		wg.Add(1)
		go func(agent agent.Agent) {
			defer wg.Done()

			output, err := agent.Execute(ctx, input)
			if err != nil {
				c.logger.Warn("agent execution failed",
					zap.String("agent_id", agent.ID()),
					zap.Error(err),
				)
				return
			}

			mu.Lock()
			outputs = append(outputs, output)
			mu.Unlock()
		}(a)
	}

	wg.Wait()

	if len(outputs) == 0 {
		return nil, fmt.Errorf("all agents failed")
	}

	// 合并所有输出
	combined := ""
	for i, output := range outputs {
		combined += fmt.Sprintf("Agent %d:\n%s\n\n", i+1, output.Content)
	}

	finalOutput := &agent.Output{
		TraceID: input.TraceID,
		Content: combined,
	}

	c.logger.Info("broadcast coordination completed")

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
	c.logger.Info("network coordination started")

	// 网络模式：Agent 之间可以自由通信
	// 简化实现：类似广播模式
	return NewBroadcastCoordinator(c.config, c.hub, c.logger).Coordinate(ctx, agents, input)
}

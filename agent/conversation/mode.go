package conversation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 对话 模式定义代理如何互动 。
type ConversationMode string

const (
	ModeRoundRobin   ConversationMode = "round_robin"  // Agents take turns
	ModeSelector     ConversationMode = "selector"     // Selector chooses next speaker
	ModeGroupChat    ConversationMode = "group_chat"   // Free-form group discussion
	ModeHierarchical ConversationMode = "hierarchical" // Manager delegates
	ModeAutoReply    ConversationMode = "auto_reply"   // Automatic response chain
)

// 对话代理界面 。
type ConversationAgent interface {
	ID() string
	Name() string
	SystemPrompt() string
	Reply(ctx context.Context, messages []ChatMessage) (*ChatMessage, error)
	ShouldTerminate(messages []ChatMessage) bool
}

// ChatMessage代表谈话中的信息.
type ChatMessage struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"` // user, assistant, system
	SenderID  string         `json:"sender_id"`
	Content   string         `json:"content"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

// 发言人选择下一位发言人.
type SpeakerSelector interface {
	SelectNext(ctx context.Context, agents []ConversationAgent, messages []ChatMessage) (ConversationAgent, error)
}

// 对话管弦乐会多代理对话.
type Conversation struct {
	ID       string
	Mode     ConversationMode
	Agents   []ConversationAgent
	Messages []ChatMessage
	Config   ConversationConfig
	Selector SpeakerSelector
	logger   *zap.Logger
	mu       sync.RWMutex
}

// 对话 Config 配置对话 。
type ConversationConfig struct {
	MaxRounds        int           `json:"max_rounds"`
	MaxMessages      int           `json:"max_messages"`
	Timeout          time.Duration `json:"timeout"`
	AllowInterrupts  bool          `json:"allow_interrupts"`
	TerminationWords []string      `json:"termination_words"`
}

// 默认 Conversation Config 返回默认配置 。
func DefaultConversationConfig() ConversationConfig {
	return ConversationConfig{
		MaxRounds:        10,
		MaxMessages:      50,
		Timeout:          10 * time.Minute,
		AllowInterrupts:  true,
		TerminationWords: []string{"TERMINATE", "DONE", "EXIT"},
	}
}

// 新联想创造了一个新的对话。
func NewConversation(mode ConversationMode, agents []ConversationAgent, config ConversationConfig, logger *zap.Logger) *Conversation {
	if logger == nil {
		logger = zap.NewNop()
	}
	c := &Conversation{
		ID:       fmt.Sprintf("conv_%d", time.Now().UnixNano()),
		Mode:     mode,
		Agents:   agents,
		Messages: make([]ChatMessage, 0),
		Config:   config,
		logger:   logger.With(zap.String("component", "conversation")),
	}
	// 基于模式设置默认选择器
	switch mode {
	case ModeRoundRobin:
		c.Selector = &RoundRobinSelector{}
	case ModeSelector:
		c.Selector = &LLMSelector{}
	default:
		c.Selector = &RoundRobinSelector{}
	}
	return c
}

// 以初始消息启动对话 。
func (c *Conversation) Start(ctx context.Context, initialMessage string) (*ConversationResult, error) {
	c.logger.Info("conversation started", zap.String("mode", string(c.Mode)), zap.Int("agents", len(c.Agents)))

	// 添加初始信件
	c.addMessage(ChatMessage{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:      "user",
		Content:   initialMessage,
		Timestamp: time.Now(),
	})

	ctx, cancel := context.WithTimeout(ctx, c.Config.Timeout)
	defer cancel()

	result := &ConversationResult{
		ConversationID: c.ID,
		StartTime:      time.Now(),
	}

	round := 0
	for round < c.Config.MaxRounds && len(c.Messages) < c.Config.MaxMessages {
		select {
		case <-ctx.Done():
			result.EndTime = time.Now()
			result.TerminationReason = "timeout"
			return result, ctx.Err()
		default:
		}

		// 选择下一个扬声器
		speaker, err := c.Selector.SelectNext(ctx, c.Agents, c.Messages)
		if err != nil {
			c.logger.Warn("speaker selection failed", zap.Error(err))
			break
		}

		// 得到回复
		reply, err := speaker.Reply(ctx, c.Messages)
		if err != nil {
			c.logger.Warn("agent reply failed", zap.String("agent", speaker.ID()), zap.Error(err))
			continue
		}

		reply.SenderID = speaker.ID()
		c.addMessage(*reply)

		// 检查终止
		if c.shouldTerminate(reply.Content) || speaker.ShouldTerminate(c.Messages) {
			result.TerminationReason = "agent_terminated"
			break
		}

		round++
	}

	result.EndTime = time.Now()
	result.Messages = c.Messages
	result.TotalRounds = round

	if result.TerminationReason == "" {
		result.TerminationReason = "max_rounds"
	}

	c.logger.Info("conversation ended", zap.String("reason", result.TerminationReason), zap.Int("rounds", round))
	return result, nil
}

func (c *Conversation) addMessage(msg ChatMessage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("msg_%d", time.Now().UnixNano())
	}
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}
	c.Messages = append(c.Messages, msg)
}

func (c *Conversation) shouldTerminate(content string) bool {
	for _, word := range c.Config.TerminationWords {
		if content == word {
			return true
		}
	}
	return false
}

// GetMessages 返回所有信件 。
func (c *Conversation) GetMessages() []ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]ChatMessage{}, c.Messages...)
}

// 对话Result包含对话结果.
type ConversationResult struct {
	ConversationID    string        `json:"conversation_id"`
	Messages          []ChatMessage `json:"messages"`
	TotalRounds       int           `json:"total_rounds"`
	StartTime         time.Time     `json:"start_time"`
	EndTime           time.Time     `json:"end_time"`
	TerminationReason string        `json:"termination_reason"`
}

// roundRobinSelector按顺序选择代理.
type RoundRobinSelector struct {
	current int
}

func (s *RoundRobinSelector) SelectNext(ctx context.Context, agents []ConversationAgent, messages []ChatMessage) (ConversationAgent, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}
	agent := agents[s.current%len(agents)]
	s.current++
	return agent, nil
}

// LLMSelector使用LLM来选择下一位扬声器.
type LLMSelector struct {
	LLM LLMClient
}

// LLM电话的LLMClient接口.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

func (s *LLMSelector) SelectNext(ctx context.Context, agents []ConversationAgent, messages []ChatMessage) (ConversationAgent, error) {
	if len(agents) == 0 {
		return nil, fmt.Errorf("no agents available")
	}
	// 如果没有 LLM , 回落到圆边
	if s.LLM == nil {
		return agents[len(messages)%len(agents)], nil
	}
	// 构建选择提示
	prompt := "Based on the conversation, select the next speaker:\n"
	for i, a := range agents {
		prompt += fmt.Sprintf("%d. %s: %s\n", i+1, a.Name(), a.SystemPrompt())
	}
	// 简而言之,返回第一代理
	return agents[0], nil
}

// GroupChatManager管理分组聊天对话.
type GroupChatManager struct {
	conversations map[string]*Conversation
	logger        *zap.Logger
	mu            sync.RWMutex
}

// NewGroupChatManager创建了新的分组聊天管理器.
func NewGroupChatManager(logger *zap.Logger) *GroupChatManager {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &GroupChatManager{
		conversations: make(map[string]*Conversation),
		logger:        logger.With(zap.String("component", "group_chat_manager")),
	}
}

// CreateChat 创建了新的分组聊天.
func (m *GroupChatManager) CreateChat(agents []ConversationAgent, config ConversationConfig) *Conversation {
	conv := NewConversation(ModeGroupChat, agents, config, m.logger)
	m.mu.Lock()
	m.conversations[conv.ID] = conv
	m.mu.Unlock()
	return conv
}

// GetChat通过身份证检索谈话内容.
func (m *GroupChatManager) GetChat(id string) (*Conversation, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conv, ok := m.conversations[id]
	return conv, ok
}

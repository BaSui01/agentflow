package multiagent

import (
	"context"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	agent "github.com/BaSui01/agentflow/agent/runtime"
	"github.com/BaSui01/agentflow/types"
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

// MultiAgentConfig 多 Agent 配置
type MultiAgentConfig struct {
	Pattern            CollaborationPattern `json:"pattern"`
	MaxRounds          int                  `json:"max_rounds"`
	ConsensusThreshold float64              `json:"consensus_threshold"`
	Timeout            time.Duration        `json:"timeout"`
	EnableVoting       bool                 `json:"enable_voting"`
	SharedState        SharedState          `json:"-"`
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

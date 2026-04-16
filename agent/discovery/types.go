package discovery

import (
	"context"
	"encoding/json"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
)

// 能力 地位代表一种能力的地位。
type CapabilityStatus string

const (
	// 能力状态表示该能力是主动的和可用的。
	CapabilityStatusActive CapabilityStatus = "active"
	// 能力现状 不活动表明暂时没有能力。
	CapabilityStatusInactive CapabilityStatus = "inactive"
	// 降级表明,该能力是可用的,但性能有所降低。
	CapabilityStatusDegraded CapabilityStatus = "degraded"
	// 能力状态不明表示能力状态不明.
	CapabilityStatusUnknown CapabilityStatus = "unknown"
)

// 代理状态代表代理状态.
type AgentStatus string

const (
	// Agent Statistics Online表示该代理在线健康.
	AgentStatusOnline AgentStatus = "online"
	// Agent StatusOffline表示代理机已下线.
	AgentStatusOffline AgentStatus = "offline"
	// Agent StatusBusy表示代理正在忙于处理任务.
	AgentStatusBusy AgentStatus = "busy"
	// 状态不健康 显示该剂是不健康的。
	AgentStatusUnhealthy AgentStatus = "unhealthy"
)

// 能力 信息包含关于某一能力的详细信息。
type CapabilityInfo struct {
	// 能力是A2A协议中的基础能力定义.
	Capability a2a.Capability `json:"capability"`

	// AgentID是提供这种能力的代理的身份.
	AgentID string `json:"agent_id"`

	// 代理名称是提供这种能力的代理的名称 。
	AgentName string `json:"agent_name"`

	// 地位是这一能力的现状。
	Status CapabilityStatus `json:"status"`

	// 得分是依据历史表现(0-100)而得的能力分.
	Score float64 `json:"score"`

	// 负载是代理的当前负载 (0-1).
	Load float64 `json:"load"`

	// 标记是能力分类的附加标记.
	Tags []string `json:"tags,omitempty"`

	// 元数据包含额外的元数据.
	Metadata map[string]string `json:"metadata,omitempty"`

	// 注册是登记这种能力的时候。
	RegisteredAt time.Time `json:"registered_at"`

	// 上次更新是上次更新时。
	LastUpdatedAt time.Time `json:"last_updated_at"`

	// 最后一次健康检查是最后一次健康检查的时间。
	LastHealthCheck time.Time `json:"last_health_check"`

	// 成功处决是成功处决的数量。
	SuccessCount int64 `json:"success_count"`

	// 失败是被处决的次数。
	FailureCount int64 `json:"failure_count"`

	// AvgLatency是平均行刑时间.
	AvgLatency time.Duration `json:"avg_latency"`
}

// AgentInfo包含了注册代理的详细信息.
type AgentInfo struct {
	// 卡为A2A代理卡.
	Card *a2a.AgentCard `json:"card"`

	// 状态是代理人的现状.
	Status AgentStatus `json:"status"`

	// 能力是这种代理人提供的能力清单。
	Capabilities []CapabilityInfo `json:"capabilities"`

	// 负载是代理的当前负载 (0-1).
	Load float64 `json:"load"`

	// 优先权是代理人对任务分配的优先权.
	Priority int `json:"priority"`

	// 端点是代理商的端点URL(用于远程代理).
	Endpoint string `json:"endpoint,omitempty"`

	// Is Local 表示是否为本地( 正在处理) 代理 。
	IsLocal bool `json:"is_local"`

	// 注册的At是该代理注册时.
	RegisteredAt time.Time `json:"registered_at"`

	// 最后的心跳是收到最后的心跳的时候.
	LastHeartbeat time.Time `json:"last_heartbeat"`

	// 元数据包含额外的元数据.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Match Request 是寻找匹配代理的请求 。
type MatchRequest struct {
	// TaskDescription是任务的自然语言描述.
	TaskDescription string `json:"task_description"`

	// 所需能力是所需能力名称的清单。
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`

	// 首选能力是首选能力名列表.
	PreferredCapabilities []string `json:"preferred_capabilities,omitempty"`

	// 需求 标记是需要标记的列表.
	RequiredTags []string `json:"required_tags,omitempty"`

	// 被排除的代理人是被排除的代理人身份列表.
	ExcludedAgents []string `json:"excluded_agents,omitempty"`

	// MinScore是所需的最低能力分数.
	MinScore float64 `json:"min_score,omitempty"`

	// MaxLoad是最大可接受负载.
	MaxLoad float64 `json:"max_load,omitempty"`

	// 限制是返回的最大结果数。
	Limit int `json:"limit,omitempty"`

	// 战略是使用的匹配战略。
	Strategy MatchStrategy `json:"strategy,omitempty"`

	// 超时是匹配操作的超时.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// MatchStrategy定义了匹配代理的战略.
type MatchStrategy string

const (
	// MatchStrategyBestMatch返回最佳匹配代理.
	MatchStrategyBestMatch MatchStrategy = "best_match"
	// MatchStrategyLeastLoaded 返回最小装入的匹配代理 。
	MatchStrategyLeastLoaded MatchStrategy = "least_loaded"
	// MatchStrategy 最高分返回最高分匹配代理.
	MatchStrategyHighestScore MatchStrategy = "highest_score"
	// Match Strategy Round Robin 以圆形Robin顺序返回代理.
	MatchStrategyRoundRobin MatchStrategy = "round_robin"
	// MatchStrategyRandom 返回随机匹配代理.
	MatchStrategyRandom MatchStrategy = "random"
)

// MatchResult代表能力匹配的结果.
type MatchResult struct {
	// 代理是匹配的代理信息。
	Agent *AgentInfo `json:"agent"`

	// 匹配能力是匹配能力列表.
	MatchedCapabilities []CapabilityInfo `json:"matched_capabilities"`

	// 得分为总比分(0-100).
	Score float64 `json:"score"`

	// 信心是比赛的信心水平 (0-1).
	Confidence float64 `json:"confidence"`

	// 理由就是比赛的原因
	Reason string `json:"reason,omitempty"`
}

// 要求构成要求构成能力。
type CompositionRequest struct {
	// TaskDescription是任务的自然语言描述.
	TaskDescription string `json:"task_description"`

	// 所需能力是所需能力名称的清单。
	RequiredCapabilities []string `json:"required_capabilities"`

	// 如果并非所有能力都具备,允许参与则允许部分组成。
	AllowPartial bool `json:"allow_partial"`

	// MaxAgents是包含在成分中的最大剂数.
	MaxAgents int `json:"max_agents,omitempty"`

	// 超时是组成操作的超时.
	Timeout time.Duration `json:"timeout,omitempty"`
}

// 能力构成的结果。
type CompositionResult struct {
	// 代理是构成中的代理名单.
	Agents []*AgentInfo `json:"agents"`

	// 能力映射能力名称到代理ID.
	CapabilityMap map[string]string `json:"capability_map"`

	// 依赖是能力之间的依赖图.
	Dependencies map[string][]string `json:"dependencies,omitempty"`

	// 执行命令是推荐的执行命令.
	ExecutionOrder []string `json:"execution_order,omitempty"`

	// 冲突是已发现的冲突清单。
	Conflicts []Conflict `json:"conflicts,omitempty"`

	// 完整表示是否满足所有要求的能力。
	Complete bool `json:"complete"`

	// 缺失能力是缺失能力列表.
	MissingCapabilities []string `json:"missing_capabilities,omitempty"`
}

// 冲突是能力之间的冲突。
type Conflict struct {
	// 类型是冲突的类型。
	Type ConflictType `json:"type"`

	// 能力是相互冲突的能力清单。
	Capabilities []string `json:"capabilities"`

	// 特工是卷入冲突的特工名单.
	Agents []string `json:"agents"`

	// 描述是对冲突的描述.
	Description string `json:"description"`

	// 决议就是所建议的决议。
	Resolution string `json:"resolution,omitempty"`
}

// 相冲突Type定义了相冲突的类型.
type ConflictType string

const (
	// 冲突类型资源表示资源冲突.
	ConflictTypeResource ConflictType = "resource"
	// 冲突类型依赖表示依赖冲突。
	ConflictTypeDependency ConflictType = "dependency"
	// 冲突类型排除表明相互排斥的能力。
	ConflictTypeExclusive ConflictType = "exclusive"
	// 相冲突TypeVersion表示版本冲突.
	ConflictTypeVersion ConflictType = "version"
)

// 发现Event代表了发现系统中的一个事件.
type DiscoveryEvent struct {
	// 类型是事件类型 。
	Type DiscoveryEventType `json:"type"`

	// AgentID是涉案特工的身份证明.
	AgentID string `json:"agent_id"`

	// 能力是所涉及的能力(如果适用的话)。
	Capability string `json:"capability,omitempty"`

	// 数据包含额外事件数据.
	Data json.RawMessage `json:"data,omitempty"`

	// 时间戳是事件发生的时间 。
	Timestamp time.Time `json:"timestamp"`
}

// 发现EventType定义了发现事件的类型.
type DiscoveryEventType string

const (
	// 发现Event Agent Registered表示某代理公司注册.
	DiscoveryEventAgentRegistered DiscoveryEventType = "agent_registered"
	// 发现Event Agent 注册未注册显示某代理未注册.
	DiscoveryEventAgentUnregistered DiscoveryEventType = "agent_unregistered"
	// 发现Event Agent Updated 显示一个代理被更新.
	DiscoveryEventAgentUpdated DiscoveryEventType = "agent_updated"
	// 发现Event Capability added表示增加了一种能力.
	DiscoveryEventCapabilityAdded DiscoveryEventType = "capability_added"
	// 发现Event Capability Removed表示能力被移除.
	DiscoveryEventCapabilityRemoved DiscoveryEventType = "capability_removed"
	// 更新后显示已更新能力。
	DiscoveryEventCapabilityUpdated DiscoveryEventType = "capability_updated"
	// 发现EventHealth检查失败 显示健康检查失败 。
	DiscoveryEventHealthCheckFailed DiscoveryEventType = "health_check_failed"
	// 发现Event Health Check Recovered 表示恢复了健康检查.
	DiscoveryEventHealthCheckRecovered DiscoveryEventType = "health_check_recovered"
)

// 发现EventHandler是一个处理发现事件的函数.
type DiscoveryEventHandler func(event *DiscoveryEvent)

// 健康检查结果代表健康检查的结果。
type HealthCheckResult struct {
	// 代理ID是代理的身份证明.
	AgentID string `json:"agent_id"`

	// 健康指示剂是否健康.
	Healthy bool `json:"healthy"`

	// 状态是代理状态.
	Status AgentStatus `json:"status"`

	// 短暂性是健康检查的短暂性。
	Latency time.Duration `json:"latency"`

	// 信件是可选信件 。
	Message string `json:"message,omitempty"`

	// 时间戳是进行健康检查的时候。
	Timestamp time.Time `json:"timestamp"`
}

// 登记册界定了能力登记册业务的接口。
type Registry interface {
	// 代理人对具有其能力的代理人进行登记。
	RegisterAgent(ctx context.Context, info *AgentInfo) error

	// 未注册代理 未经注册代理。
	UnregisterAgent(ctx context.Context, agentID string) error

	// 更新代理更新一个代理的信息 。
	UpdateAgent(ctx context.Context, info *AgentInfo) error

	// Get Agent通过身份识别找到一个特工.
	GetAgent(ctx context.Context, agentID string) (*AgentInfo, error)

	// ListAgents列出所有注册代理.
	ListAgents(ctx context.Context) ([]*AgentInfo, error)

	// 注册能力登记一种代理的能力。
	RegisterCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error

	// 未注册能力不注册 一种能力。
	UnregisterCapability(ctx context.Context, agentID string, capabilityName string) error

	// 更新能力更新一个能力.
	UpdateCapability(ctx context.Context, agentID string, cap *CapabilityInfo) error

	// Get Capability通过代理身份和姓名检索能力.
	GetCapability(ctx context.Context, agentID string, capabilityName string) (*CapabilityInfo, error)

	// List Capabilitys 列出一个代理的所有能力.
	ListCapabilities(ctx context.Context, agentID string) ([]CapabilityInfo, error)

	// Find Capabilitys 在所有特工中按名称找到能力.
	FindCapabilities(ctx context.Context, capabilityName string) ([]CapabilityInfo, error)

	// 更新代理状态更新代理状态 。
	UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error

	// 更新 AgentLoad 更新一个代理的负载 。
	UpdateAgentLoad(ctx context.Context, agentID string, load float64) error

	// 记录 Execution 记录一个执行结果 一个能力。
	RecordExecution(ctx context.Context, agentID string, capabilityName string, success bool, latency time.Duration) error

	// 订阅了发现事件。
	Subscribe(handler DiscoveryEventHandler) string

	// 不订阅来自发现事件的用户 。
	Unsubscribe(subscriptionID string)

	// 关闭注册 。
	Close() error
}

// Matcher定义了能力匹配操作的接口.
type Matcher interface {
	// Match 找到匹配给定请求的代理 。
	Match(ctx context.Context, req *MatchRequest) ([]*MatchResult, error)

	// MatchOne 找到指定请求的最佳匹配代理 。
	MatchOne(ctx context.Context, req *MatchRequest) (*MatchResult, error)

	// 分数根据请求计算代理商的比分。
	Score(ctx context.Context, agent *AgentInfo, req *MatchRequest) (float64, error)
}

// 作曲家定义了能力构成操作的接口.
type Composer interface {
	// 作曲从多个代理中产生能力组成.
	Compose(ctx context.Context, req *CompositionRequest) (*CompositionResult, error)

	// 解决依赖解决了能力之间的依赖.
	ResolveDependencies(ctx context.Context, capabilities []string) (map[string][]string, error)

	// 侦测冲突能发现能力之间的冲突。
	DetectConflicts(ctx context.Context, capabilities []string) ([]Conflict, error)
}

// 协议定义了服务发现协议操作的接口.
type Protocol interface {
	// 启动发现协议 。
	Start(ctx context.Context) error

	// 停止停止发现协议。
	Stop(ctx context.Context) error

	// 公告向网络宣布本地代理.
	Announce(ctx context.Context, info *AgentInfo) error

	// 发现在网络上发现了特工.
	Discover(ctx context.Context, filter *DiscoveryFilter) ([]*AgentInfo, error)

	// 订阅代理通知 。
	Subscribe(handler func(*AgentInfo)) string

	// 从代理通知中取消订阅。
	Unsubscribe(subscriptionID string)
}

// DiscoveryFilter定义了特工发现的过滤器.
type DiscoveryFilter struct {
	// 能力过滤 通过能力名称。
	Capabilities []string `json:"capabilities,omitempty"`

	// 通过标签过滤标记 。
	Tags []string `json:"tags,omitempty"`

	// 按代理状态进行状态过滤.
	Status []AgentStatus `json:"status,omitempty"`

	// 仅针对本地代理的本地过滤器 。
	Local *bool `json:"local,omitempty"`

	// 仅用于远程代理的远程过滤器 。
	Remote *bool `json:"remote,omitempty"`
}

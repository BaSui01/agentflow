package a2a

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/BaSui01/agentflow/agent/persistence"
	"go.uber.org/zap"
)

type A2AServer interface {
	// 注册代理在服务器上注册本地代理 。
	RegisterAgent(agent Agent) error
	// Unregister Agent 从服务器中删除一个代理 。
	UnregisterAgent(agentID string) error
	// ServiHTTP 执行 http. 服务A2A请求的掌上电脑
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// Get AgentCard为注册代理人取回代理卡.
	GetAgentCard(agentID string) (*AgentCard, error)
}

// 服务器Config持有A2A服务器的配置.
type ServerConfig struct {
	// BaseURL 是此服务器可访问的基础 URL 。
	BaseURL string
	// 默认代理ID是在没有特定代理目标时使用的代理ID.
	DefaultAgentID string
	// 请求超时是处理请求的超时.
	RequestTimeout time.Duration
	// StrictRouting 启用严格路由：目标代理不存在时直接返回错误，不回退默认代理。
	StrictRouting bool
	// 启用 Auth 允许对收到的请求进行认证 。
	EnableAuth bool
	// AuthToken 是预期的认证符( 如果 EullAuth 是真实的) 。
	AuthToken string
	// logger 是日志实例 。
	Logger *zap.Logger
}

// 默认ServerConfig 返回带有合理默认的服务器Config 。
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BaseURL:        "http://localhost:8080",
		RequestTimeout: 30 * time.Second,
		StrictRouting:  true,
		EnableAuth:     false,
		Logger:         zap.NewNop(),
	}
}

// HTTPServer是A2AServer使用HTTP的默认执行.
// 支持任务持续在服务重启后恢复 。
type HTTPServer struct {
	config *ServerConfig
	logger *zap.Logger

	// 代理通过身份证储存注册代理
	agents   map[string]Agent
	agentsMu sync.RWMutex

	// 代理卡缓存生成代理卡
	agentCards   map[string]*AgentCard
	agentCardsMu sync.RWMutex

	// asyncTasks 存储 async 任务状态( 在记忆缓存中)
	asyncTasks   map[string]*asyncTask
	asyncTasksMu sync.RWMutex

	// 任务Store 为同步任务提供持续存储
	taskStore persistence.TaskStore

	// 从代理生成代理卡
	cardGenerator *AgentCardGenerator

	// lifecycleCtx 由 InitLifecycle 派生，Shutdown 时取消。
	// 所有 Background() 回退点（async task、cleanup 等）改为从此派生，使
	// 服务关闭时飞行中的 IO 能立即终止（issue #12）。
	lifecycleMu     sync.Mutex
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	lifecycleOnce   sync.Once
}

// asyncTask 代表正在处理的 A2A 异步任务。
// Status 与 persistence.TaskStatus 语义重叠：pending/processing 对应 TaskStatusPending/TaskStatusRunning，
// completed/failed 对应 TaskStatusCompleted/TaskStatusFailed。asyncTask 为协议层内存状态，TaskStatus 为持久化层状态。
type asyncTask struct {
	ID        string      `json:"id"`
	AgentID   string      `json:"agent_id"`
	Message   *A2AMessage `json:"message"`
	Status    string      `json:"status"` // pending, processing, completed, failed
	Result    *A2AMessage `json:"result,omitempty"`
	Error     string      `json:"error,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	cancel    context.CancelFunc
}

// Async task status constants.
const (
	asyncTaskStatusPending    = "pending"
	asyncTaskStatusProcessing = "processing"
	asyncTaskStatusCompleted  = "completed"
	asyncTaskStatusFailed     = "failed"
	maxA2ARequestBodyBytes    = 1 << 20 // 1 MiB
)

// NewHTTPServer用给定的配置创建了新的HTTPServer.
func NewHTTPServer(config *ServerConfig) *HTTPServer {
	if config == nil {
		config = DefaultServerConfig()
	}
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	return &HTTPServer{
		config:        config,
		logger:        config.Logger,
		agents:        make(map[string]Agent),
		agentCards:    make(map[string]*AgentCard),
		asyncTasks:    make(map[string]*asyncTask),
		cardGenerator: NewAgentCardGenerator(),
	}
}

// NewHTTPServer With TaskStore创建了新的HTTPServer,任务持续.
func NewHTTPServerWithTaskStore(config *ServerConfig, taskStore persistence.TaskStore) *HTTPServer {
	server := NewHTTPServer(config)
	server.taskStore = taskStore
	return server
}

// SetTaskStore 设置任务存储,用于持久性(依赖性注射).

// InitLifecycle initializes the server's lifecycle context derived from parent.
// Must be called before the server starts handling requests.
// All async task contexts and cleanup operations will respect this lifecycle (#12).
func (s *HTTPServer) InitLifecycle(parent context.Context) {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	s.lifecycleCtx, s.lifecycleCancel = context.WithCancel(parent)
}

// Shutdown cancels the lifecycle context, terminating in-flight async tasks and
// cleanup loops. Safe to call multiple times.
func (s *HTTPServer) Shutdown() {
	s.lifecycleOnce.Do(func() {
		s.lifecycleMu.Lock()
		if s.lifecycleCancel != nil {
			s.lifecycleCancel()
		}
		s.lifecycleMu.Unlock()
	})
}

// lifecycleContext returns the server's lifecycle context, falling back to
// context.Background() if InitLifecycle was never called.
func (s *HTTPServer) lifecycleContext() context.Context {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.lifecycleCtx != nil {
		return s.lifecycleCtx
	}
	return context.Background()
}

// =============================================================================
// 🧪 E2E 测试设置
// =============================================================================
// 端到端测试的通用设置和辅助函数
//
// 运行方式:
//
//	go test ./tests/e2e/... -v -tags=e2e
// =============================================================================
//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/testutil"
	"github.com/BaSui01/agentflow/testutil/mocks"
)

// =============================================================================
// 🎯 测试环境
// =============================================================================

// TestEnv E2E 测试环境
type TestEnv struct {
	Config   *config.Config
	Logger   *zap.Logger
	Provider *mocks.MockProvider
	Memory   *mocks.MockMemoryManager
	Tools    *mocks.MockToolManager

	ctx    context.Context
	cancel context.CancelFunc
}

// =============================================================================
// 🔧 环境设置
// =============================================================================

// NewTestEnv 创建新的测试环境
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	// 加载配置
	cfg := config.DefaultConfig()

	// 从环境变量覆盖（用于 CI/CD）
	if envCfg, err := config.LoadFromEnv(); err == nil {
		cfg = envCfg
	}

	// 创建 logger
	logger, _ := zap.NewDevelopment()

	// 创建 mock 组件
	provider := mocks.NewMockProvider()
	memory := mocks.NewMockMemoryManager()
	tools := mocks.NewMockToolManager()

	env := &TestEnv{
		Config:   cfg,
		Logger:   logger,
		Provider: provider,
		Memory:   memory,
		Tools:    tools,
		ctx:      ctx,
		cancel:   cancel,
	}

	// 注册清理函数
	t.Cleanup(func() {
		env.Cleanup()
	})

	return env
}

// Context 返回测试上下文
func (e *TestEnv) Context() context.Context {
	return e.ctx
}

// Cleanup 清理测试环境
func (e *TestEnv) Cleanup() {
	e.cancel()
	if e.Logger != nil {
		e.Logger.Sync()
	}
}

// Reset 重置所有 mock 状态
func (e *TestEnv) Reset() {
	e.Provider.Reset()
	e.Memory.Reset()
	e.Tools.Reset()
}

// =============================================================================
// 🔍 环境检查
// =============================================================================

// SkipIfNoDocker 如果没有 Docker 则跳过测试
func SkipIfNoDocker(t *testing.T) {
	t.Helper()
	if os.Getenv("DOCKER_HOST") == "" && !fileExists("/var/run/docker.sock") {
		t.Skip("Skipping test: Docker not available")
	}
}

// SkipIfNoRedis 如果没有 Redis 则跳过测试
func SkipIfNoRedis(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTFLOW_REDIS_ADDR") == "" {
		t.Skip("Skipping test: Redis not configured (set AGENTFLOW_REDIS_ADDR)")
	}
}

// SkipIfNoPostgres 如果没有 PostgreSQL 则跳过测试
func SkipIfNoPostgres(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTFLOW_DATABASE_HOST") == "" {
		t.Skip("Skipping test: PostgreSQL not configured (set AGENTFLOW_DATABASE_HOST)")
	}
}

// SkipIfNoQdrant 如果没有 Qdrant 则跳过测试
func SkipIfNoQdrant(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTFLOW_QDRANT_HOST") == "" {
		t.Skip("Skipping test: Qdrant not configured (set AGENTFLOW_QDRANT_HOST)")
	}
}

// SkipIfNoLLMKey 如果没有 LLM API Key 则跳过测试
func SkipIfNoLLMKey(t *testing.T) {
	t.Helper()
	if os.Getenv("AGENTFLOW_LLM_API_KEY") == "" && os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping test: LLM API key not configured")
	}
}

// SkipIfShort 如果是短测试模式则跳过
func SkipIfShort(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping long-running test in short mode")
	}
}

// =============================================================================
// 🎭 测试辅助
// =============================================================================

// WaitForCondition 等待条件满足
func WaitForCondition(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()
	if !testutil.WaitFor(condition, timeout) {
		t.Fatalf("Condition not met within %v: %s", timeout, msg)
	}
}

// AssertEventually 断言条件最终满足
func AssertEventually(t *testing.T, condition func() bool, timeout time.Duration) {
	t.Helper()
	testutil.AssertEventuallyTrue(t, condition, timeout)
}

// =============================================================================
// 📁 文件辅助
// =============================================================================

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// CreateTempDir 创建临时目录
func CreateTempDir(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
	})
	return dir
}

// CreateTempFile 创建临时文件
func CreateTempFile(t *testing.T, dir, pattern, content string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	return f.Name()
}

// =============================================================================
// 🏃 测试运行器
// =============================================================================

// RunWithTimeout 在超时内运行测试函数
func RunWithTimeout(t *testing.T, timeout time.Duration, fn func(ctx context.Context)) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(ctx)
	}()

	select {
	case <-done:
		// 测试完成
	case <-ctx.Done():
		t.Fatalf("Test timed out after %v", timeout)
	}
}

// RunParallel 并行运行多个测试函数
func RunParallel(t *testing.T, fns ...func(t *testing.T)) {
	t.Helper()

	for i, fn := range fns {
		fn := fn // 捕获变量
		t.Run("parallel_"+string(rune('0'+i)), func(t *testing.T) {
			t.Parallel()
			fn(t)
		})
	}
}

// =============================================================================
// 📊 指标收集
// =============================================================================

// TestMetrics 测试指标收集器
type TestMetrics struct {
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Iterations   int
	Errors       int
	SuccessRate  float64
	CustomValues map[string]any
}

// NewTestMetrics 创建新的指标收集器
func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		StartTime:    time.Now(),
		CustomValues: make(map[string]any),
	}
}

// Start 开始计时
func (m *TestMetrics) Start() {
	m.StartTime = time.Now()
}

// Stop 停止计时
func (m *TestMetrics) Stop() {
	m.EndTime = time.Now()
	m.Duration = m.EndTime.Sub(m.StartTime)
}

// RecordIteration 记录一次迭代
func (m *TestMetrics) RecordIteration(success bool) {
	m.Iterations++
	if !success {
		m.Errors++
	}
	m.SuccessRate = float64(m.Iterations-m.Errors) / float64(m.Iterations)
}

// Set 设置自定义值
func (m *TestMetrics) Set(key string, value any) {
	m.CustomValues[key] = value
}

// Report 报告指标
func (m *TestMetrics) Report(t *testing.T) {
	t.Helper()
	t.Logf("Test Metrics:")
	t.Logf("  Duration: %v", m.Duration)
	t.Logf("  Iterations: %d", m.Iterations)
	t.Logf("  Errors: %d", m.Errors)
	t.Logf("  Success Rate: %.2f%%", m.SuccessRate*100)
	for k, v := range m.CustomValues {
		t.Logf("  %s: %v", k, v)
	}
}

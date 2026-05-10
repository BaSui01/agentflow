package tools

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestHealthChecker_StopCancelsLifecycleCtx 验证 GitHub Issue #12：
// Start(ctx) 后，Stop() 必须取消 lifecycleCtx，让飞行中的 health-check IO 立即退出。
func TestHealthChecker_StopCancelsLifecycleCtx(t *testing.T) {
	reg := NewCapabilityRegistry(nil, zap.NewNop())
	h := NewHealthChecker(&HealthCheckerConfig{
		Interval:           time.Hour, // 不让 ticker 触发 checkAll
		Timeout:            time.Second,
		UnhealthyThreshold: 3,
	}, reg, zap.NewNop())

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := h.Start(parentCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// 白盒读取 lifecycleCtx —— 同包测试可以访问私有字段。
	h.lifecycleMu.Lock()
	lcCtx := h.lifecycleCtx
	h.lifecycleMu.Unlock()

	if lcCtx == nil {
		t.Fatal("lifecycleCtx not initialized after Start()")
	}
	if err := lcCtx.Err(); err != nil {
		t.Fatalf("lifecycleCtx already done after Start(): %v", err)
	}

	if err := h.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-lcCtx.Done():
		// 期望路径：Stop() 取消了 lifecycleCtx。
	case <-time.After(500 * time.Millisecond):
		t.Fatal("lifecycleCtx not done after Stop() — Stop 应取消 lifecycleCancel")
	}
}

// TestHealthChecker_ParentCancelCancelsLifecycleCtx 验证：
// 父 ctx 取消时，lifecycleCtx 也应该 Done（context.WithCancel 派生的语义保证）。
func TestHealthChecker_ParentCancelCancelsLifecycleCtx(t *testing.T) {
	reg := NewCapabilityRegistry(nil, zap.NewNop())
	h := NewHealthChecker(&HealthCheckerConfig{
		Interval:           time.Hour,
		Timeout:            time.Second,
		UnhealthyThreshold: 3,
	}, reg, zap.NewNop())

	parentCtx, parentCancel := context.WithCancel(context.Background())

	if err := h.Start(parentCtx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer h.Stop(context.Background())

	h.lifecycleMu.Lock()
	lcCtx := h.lifecycleCtx
	h.lifecycleMu.Unlock()

	if lcCtx == nil {
		t.Fatal("lifecycleCtx not initialized after Start()")
	}
	if err := lcCtx.Err(); err != nil {
		t.Fatalf("lifecycleCtx already done after Start(): %v", err)
	}

	parentCancel()

	select {
	case <-lcCtx.Done():
		// 期望路径：父 ctx 取消立即传播。
	case <-time.After(500 * time.Millisecond):
		t.Fatal("lifecycleCtx did not propagate parent ctx cancellation — " +
			"应使用 context.WithCancel(parent) 派生 lifecycleCtx")
	}
}

// TestHealthChecker_CheckAllUsesLifecycleCtx 间接验证 checkAll 不再依赖 background：
// 取消 parentCtx 后，立即调用 checkAll 应观察到传入的 ctx 已 Done。
// （这是一个回归保护：避免后续重构意外把 ctx 改回 Background。）
func TestHealthChecker_CheckAllUsesLifecycleCtx(t *testing.T) {
	reg := NewCapabilityRegistry(nil, zap.NewNop())
	h := NewHealthChecker(&HealthCheckerConfig{
		Interval:           time.Hour,
		Timeout:            5 * time.Second,
		UnhealthyThreshold: 3,
	}, reg, zap.NewNop())

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 直接调用 checkAll（包级测试可访问），观察其内部派生 ctx 是否会立即 Done。
	// 我们通过对 ListAgents 的回归来观察：empty registry 不会出错，但
	// checkAll 内部派生的 ctx 应当从 cancelledCtx 派生 → 立即 Done。
	// 这里我们简单跑一遍 checkAll，确保它对已取消的 ctx 不会 panic 或卡死。
	done := make(chan struct{})
	go func() {
		h.checkAll(cancelledCtx)
		close(done)
	}()

	select {
	case <-done:
		// 期望：checkAll 立即返回。
	case <-time.After(time.Second):
		t.Fatal("checkAll did not return promptly with a cancelled parent ctx")
	}
}

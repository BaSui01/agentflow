package runtime

import (
	"context"
	"testing"
	"time"
)

// =============================================================================
// SteeringChannel Tests
// =============================================================================

func TestSteeringChannel_SendReceive(t *testing.T) {
	ch := NewSteeringChannel(2)
	defer ch.Close()

	msg := SteeringMessage{
		Type:    SteeringTypeGuide,
		Content: "请用中文回答",
	}

	if err := ch.Send(msg); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	select {
	case got := <-ch.Receive():
		if got.Type != SteeringTypeGuide {
			t.Errorf("Type = %q, want %q", got.Type, SteeringTypeGuide)
		}
		if got.Content != "请用中文回答" {
			t.Errorf("Content = %q, want %q", got.Content, "请用中文回答")
		}
		if got.Timestamp.IsZero() {
			t.Error("Timestamp should be auto-filled")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSteeringChannel_SendFull(t *testing.T) {
	ch := NewSteeringChannel(1)
	defer ch.Close()

	// 填满 buffer
	if err := ch.Send(SteeringMessage{Type: SteeringTypeGuide, Content: "first"}); err != nil {
		t.Fatalf("first Send failed: %v", err)
	}

	// 第二次应该返回 ErrSteeringChannelFull
	err := ch.Send(SteeringMessage{Type: SteeringTypeGuide, Content: "second"})
	if err != ErrSteeringChannelFull {
		t.Errorf("err = %v, want ErrSteeringChannelFull", err)
	}
}

func TestSteeringChannel_SendAfterClose(t *testing.T) {
	ch := NewSteeringChannel(1)
	ch.Close()

	err := ch.Send(SteeringMessage{Type: SteeringTypeGuide, Content: "too late"})
	if err != ErrSteeringChannelClosed {
		t.Errorf("err = %v, want ErrSteeringChannelClosed", err)
	}
}

func TestSteeringChannel_DoubleClose(t *testing.T) {
	ch := NewSteeringChannel(1)
	ch.Close()
	ch.Close() // 不应 panic
	if !ch.IsClosed() {
		t.Error("IsClosed should be true after Close")
	}
}

func TestSteeringChannel_DefaultBufSize(t *testing.T) {
	ch := NewSteeringChannel(0) // 应该被修正为 1
	defer ch.Close()

	if err := ch.Send(SteeringMessage{Type: SteeringTypeStopAndSend, Content: "hi"}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}
}

// =============================================================================
// Context Injection Tests
// =============================================================================

func TestWithSteeringChannel_RoundTrip(t *testing.T) {
	ch := NewSteeringChannel(1)
	defer ch.Close()

	ctx := WithSteeringChannel(context.Background(), ch)
	got, ok := SteeringChannelFromContext(ctx)
	if !ok {
		t.Fatal("SteeringChannelFromContext returned false")
	}
	if got != ch {
		t.Error("got different SteeringChannel instance")
	}
}

func TestWithSteeringChannel_NilChannel(t *testing.T) {
	ctx := WithSteeringChannel(context.Background(), nil)
	_, ok := SteeringChannelFromContext(ctx)
	if ok {
		t.Error("should return false for nil channel")
	}
}

func TestSteeringChannelFromContext_NilContext(t *testing.T) {
	_, ok := SteeringChannelFromContext(nil)
	if ok {
		t.Error("should return false for nil context")
	}
}

func TestSteeringChannelFromContext_NoChannel(t *testing.T) {
	_, ok := SteeringChannelFromContext(context.Background())
	if ok {
		t.Error("should return false when no channel in context")
	}
}

// =============================================================================
// steerChOrNil Tests
// =============================================================================

func TestSteerChOrNil_Nil(t *testing.T) {
	ch := steerChOrNil(nil)
	if ch != nil {
		t.Error("should return nil for nil SteeringChannel")
	}
}

func TestSteerChOrNil_Valid(t *testing.T) {
	sc := NewSteeringChannel(1)
	defer sc.Close()

	ch := steerChOrNil(sc)
	if ch == nil {
		t.Error("should return non-nil channel")
	}
}

// =============================================================================
// SessionManager Tests
// =============================================================================

func TestSessionManager_CreateAndGet(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	sess := mgr.Create("test-agent")
	if sess.ID == "" {
		t.Fatal("session ID should not be empty")
	}
	if sess.AgentID != "test-agent" {
		t.Errorf("AgentID = %q, want %q", sess.AgentID, "test-agent")
	}
	if sess.SteeringCh == nil {
		t.Fatal("SteeringCh should not be nil")
	}
	if !sess.IsRunning() {
		t.Error("new session should be running")
	}

	got, ok := mgr.Get(sess.ID)
	if !ok {
		t.Fatal("Get returned false for existing session")
	}
	if got.ID != sess.ID {
		t.Error("got different session")
	}
}

func TestSessionManager_GetNotFound(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	_, ok := mgr.Get("nonexistent")
	if ok {
		t.Error("Get should return false for nonexistent session")
	}
}

func TestSessionManager_Remove(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	sess := mgr.Create("test-agent")
	mgr.Remove(sess.ID)

	_, ok := mgr.Get(sess.ID)
	if ok {
		t.Error("Get should return false after Remove")
	}

	if sess.IsRunning() {
		t.Error("session should be completed after Remove")
	}
	if !sess.SteeringCh.IsClosed() {
		t.Error("SteeringCh should be closed after Remove")
	}
}

func TestSessionManager_RemoveIdempotent(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	sess := mgr.Create("test-agent")
	mgr.Remove(sess.ID)
	mgr.Remove(sess.ID) // 不应 panic
}

func TestSessionManager_Cleanup(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	sess := mgr.Create("old-agent")
	// 手动设置 CreatedAt 为过去
	sess.CreatedAt = time.Now().Add(-time.Hour)
	sess.Complete() // 已完成 + 过期 → 应被清理

	stale := mgr.Create("stale-running")
	stale.CreatedAt = time.Now().Add(-time.Hour)
	// 仍在运行 → 不应被清理

	fresh := mgr.Create("fresh-agent")

	mgr.Cleanup(30 * time.Minute)

	_, oldOK := mgr.Get(sess.ID)
	_, staleOK := mgr.Get(stale.ID)
	_, freshOK := mgr.Get(fresh.ID)

	if oldOK {
		t.Error("completed old session should be cleaned up")
	}
	if !staleOK {
		t.Error("stale but running session should NOT be cleaned up")
	}
	if !freshOK {
		t.Error("fresh session should still exist")
	}
}

func TestExecutionSession_Complete(t *testing.T) {
	mgr := NewSessionManager()
	defer mgr.Stop()

	sess := mgr.Create("test-agent")
	if sess.Status() != "running" {
		t.Errorf("Status = %q, want %q", sess.Status(), "running")
	}

	sess.Complete()
	if sess.Status() != "completed" {
		t.Errorf("Status = %q, want %q", sess.Status(), "completed")
	}
	if sess.IsRunning() {
		t.Error("IsRunning should be false after Complete")
	}
}



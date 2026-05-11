package hitl

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// fakeTxStore 是一个仅供测试的 InterruptStore，附带 WithTransaction 能力。
// 它把每次 WithTransaction 调用记录下来，并把 fn 收到的 store 替换成包装了
// "tx-bound" 标记的子 store，以验证 fn 内的 ops 走的是 tx 路径。
type fakeTxStore struct {
	mu        sync.Mutex
	beginCnt  int
	commitCnt int
	abortCnt  int
	saved     []*Interrupt
	updated   []*Interrupt
}

func (s *fakeTxStore) Save(ctx context.Context, it *Interrupt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, it)
	return nil
}
func (s *fakeTxStore) Load(ctx context.Context, id string) (*Interrupt, error) {
	return nil, errors.New("not implemented")
}
func (s *fakeTxStore) List(ctx context.Context, _ string, _ InterruptStatus) ([]*Interrupt, error) {
	return nil, nil
}
func (s *fakeTxStore) Update(ctx context.Context, it *Interrupt) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updated = append(s.updated, it)
	return nil
}

// WithTransaction 模拟一个真正的事务边界：begin → fn → commit/abort。
// 注意 fn 收到的 store 仍然是 s 自己（in-memory 没有真实分支），但 begin/commit
// 的计数让我们能验证 RunInTransaction 走对了分支。
func (s *fakeTxStore) WithTransaction(ctx context.Context, fn func(InterruptStore) error) error {
	s.mu.Lock()
	s.beginCnt++
	s.mu.Unlock()

	if err := fn(s); err != nil {
		s.mu.Lock()
		s.abortCnt++
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.commitCnt++
	s.mu.Unlock()
	return nil
}

var _ InterruptStore = (*fakeTxStore)(nil)
var _ TxInterruptStore = (*fakeTxStore)(nil)

// TestRunInTransaction_FallThroughWhenNotTx 验证 GitHub Issue #18：
// 当 store 不实现 TxInterruptStore 时，RunInTransaction 应直接调用 fn(store)
// 而不要求实现额外能力（向后兼容现有 InMemoryInterruptStore）。
func TestRunInTransaction_FallThroughWhenNotTx(t *testing.T) {
	store := NewInMemoryInterruptStore()

	called := false
	err := RunInTransaction(context.Background(), store, func(s InterruptStore) error {
		called = true
		if s != store {
			return errors.New("fn should receive the original store when no tx capability")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTransaction: %v", err)
	}
	if !called {
		t.Fatal("fn was not invoked")
	}
}

// TestRunInTransaction_UsesTxWhenSupported 验证当 store 实现 TxInterruptStore 时，
// RunInTransaction 必须通过 WithTransaction 调用 fn。
func TestRunInTransaction_UsesTxWhenSupported(t *testing.T) {
	tx := &fakeTxStore{}

	err := RunInTransaction(context.Background(), tx, func(s InterruptStore) error {
		return s.Update(context.Background(), &Interrupt{ID: "i1"})
	})
	if err != nil {
		t.Fatalf("RunInTransaction: %v", err)
	}
	if tx.beginCnt != 1 {
		t.Errorf("begin: want 1, got %d", tx.beginCnt)
	}
	if tx.commitCnt != 1 {
		t.Errorf("commit: want 1, got %d", tx.commitCnt)
	}
	if tx.abortCnt != 0 {
		t.Errorf("abort: want 0, got %d", tx.abortCnt)
	}
	if len(tx.updated) != 1 {
		t.Errorf("updated: want 1, got %d", len(tx.updated))
	}
}

// TestRunInTransaction_PropagatesError 验证 fn 返回错误时事务被 abort，
// RunInTransaction 把错误透传给调用者。
func TestRunInTransaction_PropagatesError(t *testing.T) {
	tx := &fakeTxStore{}
	want := errors.New("boom")

	got := RunInTransaction(context.Background(), tx, func(s InterruptStore) error {
		return want
	})
	if !errors.Is(got, want) {
		t.Errorf("err: want %v, got %v", want, got)
	}
	if tx.beginCnt != 1 {
		t.Errorf("begin: want 1, got %d", tx.beginCnt)
	}
	if tx.commitCnt != 0 {
		t.Errorf("commit: want 0 on err, got %d", tx.commitCnt)
	}
	if tx.abortCnt != 1 {
		t.Errorf("abort: want 1, got %d", tx.abortCnt)
	}
}

// TestResolveInterrupt_UsesTransactionWhenSupported 验证 ResolveInterrupt 在 store
// 支持事务时使用 WithTransaction 包裹 Update（让事务边界保护"状态转换 + 持久化"）。
// 这是 issue #18 的核心收益：避免并发场景下 in-memory 状态与持久化状态分裂。
func TestResolveInterrupt_UsesTransactionWhenSupported(t *testing.T) {
	tx := &fakeTxStore{}
	mgr := NewInterruptManager(tx, nil)

	// 直接构造 pending（绕过 CreateInterrupt 的等待逻辑）
	interrupt := &Interrupt{ID: "int_test", Type: InterruptTypeApproval, Status: InterruptStatusPending}
	pending := &pendingInterrupt{
		interrupt:  interrupt,
		responseCh: make(chan *Response, 1),
		cancelFn:   func() {},
		timeoutCtx: context.Background(),
	}
	mgr.mu.Lock()
	mgr.pending[interrupt.ID] = pending
	mgr.mu.Unlock()

	err := mgr.ResolveInterrupt(context.Background(), interrupt.ID, &Response{Approved: true})
	if err != nil {
		t.Fatalf("ResolveInterrupt: %v", err)
	}

	if tx.beginCnt != 1 {
		t.Errorf("ResolveInterrupt should open exactly 1 tx, got begin=%d", tx.beginCnt)
	}
	if tx.commitCnt != 1 {
		t.Errorf("ResolveInterrupt should commit, got commit=%d", tx.commitCnt)
	}
	if len(tx.updated) != 1 {
		t.Errorf("Update inside tx: want 1, got %d", len(tx.updated))
	}
	if tx.updated[0].Status != InterruptStatusResolved {
		t.Errorf("status inside tx: want resolved, got %s", tx.updated[0].Status)
	}
}

// TestCancelInterrupt_UsesTransactionWhenSupported 验证 CancelInterrupt 在 store
// 支持事务时同样使用 WithTransaction 包裹 Update，避免取消路径把 pending 删除后持久化失败。
func TestCancelInterrupt_UsesTransactionWhenSupported(t *testing.T) {
	tx := &fakeTxStore{}
	mgr := NewInterruptManager(tx, nil)

	interrupt := &Interrupt{ID: "int_cancel", Type: InterruptTypeApproval, Status: InterruptStatusPending}
	pending := &pendingInterrupt{
		interrupt:  interrupt,
		responseCh: make(chan *Response, 1),
		cancelFn:   func() {},
		timeoutCtx: context.Background(),
	}
	mgr.mu.Lock()
	mgr.pending[interrupt.ID] = pending
	mgr.mu.Unlock()

	err := mgr.CancelInterrupt(context.Background(), interrupt.ID)
	if err != nil {
		t.Fatalf("CancelInterrupt: %v", err)
	}

	if tx.beginCnt != 1 {
		t.Errorf("CancelInterrupt should open exactly 1 tx, got begin=%d", tx.beginCnt)
	}
	if tx.commitCnt != 1 {
		t.Errorf("CancelInterrupt should commit, got commit=%d", tx.commitCnt)
	}
	if len(tx.updated) != 1 {
		t.Errorf("Update inside tx: want 1, got %d", len(tx.updated))
	}
	if tx.updated[0].Status != InterruptStatusCanceled {
		t.Errorf("status inside tx: want canceled, got %s", tx.updated[0].Status)
	}
}

// TestHandleTimeout_UsesTransactionWhenSupported 验证 timeout 路径也走事务包装，
// 保证后台超时回调的状态落库与内存态切换保持一致。
func TestHandleTimeout_UsesTransactionWhenSupported(t *testing.T) {
	tx := &fakeTxStore{}
	mgr := NewInterruptManager(tx, nil)

	interrupt := &Interrupt{ID: "int_timeout", Type: InterruptTypeApproval, Status: InterruptStatusPending}
	mgr.mu.Lock()
	mgr.pending[interrupt.ID] = &pendingInterrupt{
		interrupt:  interrupt,
		responseCh: make(chan *Response, 1),
		cancelFn:   func() {},
		timeoutCtx: context.Background(),
	}
	mgr.mu.Unlock()

	mgr.handleTimeout(context.Background(), interrupt)

	if tx.beginCnt != 1 {
		t.Errorf("handleTimeout should open exactly 1 tx, got begin=%d", tx.beginCnt)
	}
	if tx.commitCnt != 1 {
		t.Errorf("handleTimeout should commit, got commit=%d", tx.commitCnt)
	}
	if len(tx.updated) != 1 {
		t.Errorf("Update inside tx: want 1, got %d", len(tx.updated))
	}
	if tx.updated[0].Status != InterruptStatusTimeout {
		t.Errorf("status inside tx: want timeout, got %s", tx.updated[0].Status)
	}
}

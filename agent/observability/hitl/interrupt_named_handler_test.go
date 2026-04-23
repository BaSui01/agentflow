package hitl

import (
	"context"
	"testing"
)

func TestRegisterNamedHandler_IsIdempotent(t *testing.T) {
	t.Parallel()

	manager := NewInterruptManager(NewInMemoryInterruptStore(), nil)

	first := manager.RegisterNamedHandler(InterruptTypeApproval, "workflow_auto_approve", func(_ctx context.Context, _interrupt *Interrupt) error {
		return nil
	})
	second := manager.RegisterNamedHandler(InterruptTypeApproval, "workflow_auto_approve", func(_ctx context.Context, _interrupt *Interrupt) error {
		return nil
	})

	if !first {
		t.Fatalf("expected first named handler registration to succeed")
	}
	if second {
		t.Fatalf("expected duplicate named handler registration to be ignored")
	}
	if got := len(manager.handlers[InterruptTypeApproval]); got != 1 {
		t.Fatalf("expected exactly one approval handler, got %d", got)
	}
}

package workflow

import (
	"sync"
	"testing"
)

// TestChannelImplementsChannelReader verifies that Channel[T] satisfies the ChannelReader interface.
func TestChannelImplementsChannelReader(t *testing.T) {
	ch := NewChannel[string]("test", "hello")
	var _ ChannelReader = ch // compile-time check
}

// TestSnapshotReturnsValues ensures Snapshot captures values from all registered channels.
func TestSnapshotReturnsValues(t *testing.T) {
	sg := NewStateGraph()

	strCh := NewChannel[string]("name", "alice")
	intCh := NewChannel[int]("count", 42)
	RegisterChannel(sg, strCh)
	RegisterChannel(sg, intCh)

	snap := sg.Snapshot()

	// Values should not be empty (this was the original bug).
	if len(snap.Values) != 2 {
		t.Fatalf("expected 2 values in snapshot, got %d", len(snap.Values))
	}

	if v, ok := snap.Values["name"]; !ok || v != "alice" {
		t.Errorf("expected name=alice, got %v", v)
	}
	if v, ok := snap.Values["count"]; !ok || v != 42 {
		t.Errorf("expected count=42, got %v", v)
	}
}

// TestSnapshotReturnsVersions ensures Snapshot captures version numbers.
func TestSnapshotReturnsVersions(t *testing.T) {
	sg := NewStateGraph()

	ch := NewChannel[string]("status", "init")
	RegisterChannel(sg, ch)

	ch.Update("running")
	ch.Update("done")

	snap := sg.Snapshot()

	if v, ok := snap.Versions["status"]; !ok || v != 2 {
		t.Errorf("expected version=2, got %v", v)
	}
	if v, ok := snap.Values["status"]; !ok || v != "done" {
		t.Errorf("expected status=done, got %v", v)
	}
}

// TestSnapshotEmptyGraph ensures Snapshot on an empty graph returns empty maps.
func TestSnapshotEmptyGraph(t *testing.T) {
	sg := NewStateGraph()
	snap := sg.Snapshot()

	if len(snap.Values) != 0 {
		t.Errorf("expected 0 values, got %d", len(snap.Values))
	}
	if len(snap.Versions) != 0 {
		t.Errorf("expected 0 versions, got %d", len(snap.Versions))
	}
}

// TestSnapshotConcurrentAccess verifies Snapshot is safe under concurrent updates.
func TestSnapshotConcurrentAccess(t *testing.T) {
	sg := NewStateGraph()
	ch := NewChannel[int]("counter", 0, WithReducer(SumReducer[int]()))
	RegisterChannel(sg, ch)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Update(1)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := sg.Snapshot()
			// Value must always be present and non-negative.
			v, ok := snap.Values["counter"]
			if !ok {
				t.Errorf("counter missing from snapshot")
			}
			if v.(int) < 0 {
				t.Errorf("unexpected negative counter: %v", v)
			}
		}()
	}
	wg.Wait()
}

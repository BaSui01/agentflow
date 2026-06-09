package runtime

import "testing"

func TestSessionManagerStopClosesAndRejectsNewSessions(t *testing.T) {
	m := NewSessionManager()
	sess := m.Create("agent-1")
	if sess == nil {
		t.Fatal("expected initial session")
	}

	m.Stop()

	if sess.IsRunning() {
		t.Fatal("expected Stop to complete existing session")
	}
	if _, ok := m.Get(sess.ID); ok {
		t.Fatal("expected Stop to remove existing session")
	}
	if created := m.Create("agent-1"); created != nil {
		t.Fatalf("expected Create after Stop to be rejected, got %#v", created)
	}
}

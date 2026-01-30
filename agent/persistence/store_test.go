package persistence

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestMemoryMessageStore tests the in-memory message store
func TestMemoryMessageStore(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false // Disable auto cleanup for tests
	store := NewMemoryMessageStore(config)
	defer store.Close()

	ctx := context.Background()

	t.Run("Ping", func(t *testing.T) {
		if err := store.Ping(ctx); err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	t.Run("SaveAndGetMessage", func(t *testing.T) {
		msg := &Message{
			ID:      "test-msg-1",
			Topic:   "test-topic",
			FromID:  "agent-1",
			ToID:    "agent-2",
			Type:    "proposal",
			Content: "Hello, World!",
		}

		if err := store.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}

		retrieved, err := store.GetMessage(ctx, "test-msg-1")
		if err != nil {
			t.Fatalf("GetMessage failed: %v", err)
		}

		if retrieved.Content != msg.Content {
			t.Errorf("Content mismatch: got %s, want %s", retrieved.Content, msg.Content)
		}
	})

	t.Run("SaveMessages", func(t *testing.T) {
		msgs := []*Message{
			{ID: "batch-1", Topic: "batch-topic", Content: "Message 1"},
			{ID: "batch-2", Topic: "batch-topic", Content: "Message 2"},
			{ID: "batch-3", Topic: "batch-topic", Content: "Message 3"},
		}

		if err := store.SaveMessages(ctx, msgs); err != nil {
			t.Fatalf("SaveMessages failed: %v", err)
		}

		for _, msg := range msgs {
			retrieved, err := store.GetMessage(ctx, msg.ID)
			if err != nil {
				t.Errorf("GetMessage failed for %s: %v", msg.ID, err)
			}
			if retrieved.Content != msg.Content {
				t.Errorf("Content mismatch for %s", msg.ID)
			}
		}
	})

	t.Run("GetMessages", func(t *testing.T) {
		// Get messages from batch-topic
		msgs, cursor, err := store.GetMessages(ctx, "batch-topic", "", 10)
		if err != nil {
			t.Fatalf("GetMessages failed: %v", err)
		}

		if len(msgs) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(msgs))
		}

		// Test pagination
		msgs, _, err = store.GetMessages(ctx, "batch-topic", "", 2)
		if err != nil {
			t.Fatalf("GetMessages with limit failed: %v", err)
		}

		if len(msgs) != 2 {
			t.Errorf("Expected 2 messages with limit, got %d", len(msgs))
		}

		_ = cursor // cursor is used for pagination
	})

	t.Run("AckMessage", func(t *testing.T) {
		msg := &Message{
			ID:      "ack-test",
			Topic:   "ack-topic",
			Content: "To be acked",
		}
		store.SaveMessage(ctx, msg)

		if err := store.AckMessage(ctx, "ack-test"); err != nil {
			t.Fatalf("AckMessage failed: %v", err)
		}

		retrieved, _ := store.GetMessage(ctx, "ack-test")
		if retrieved.AckedAt == nil {
			t.Error("Message should be acked")
		}
	})

	t.Run("GetUnackedMessages", func(t *testing.T) {
		// Create an old unacked message
		oldMsg := &Message{
			ID:        "old-unacked",
			Topic:     "unacked-topic",
			Content:   "Old message",
			CreatedAt: time.Now().Add(-10 * time.Minute),
		}
		store.SaveMessage(ctx, oldMsg)

		// Create a new unacked message
		newMsg := &Message{
			ID:      "new-unacked",
			Topic:   "unacked-topic",
			Content: "New message",
		}
		store.SaveMessage(ctx, newMsg)

		// Get messages older than 5 minutes
		msgs, err := store.GetUnackedMessages(ctx, "unacked-topic", 5*time.Minute)
		if err != nil {
			t.Fatalf("GetUnackedMessages failed: %v", err)
		}

		if len(msgs) != 1 {
			t.Errorf("Expected 1 old unacked message, got %d", len(msgs))
		}
	})

	t.Run("DeleteMessage", func(t *testing.T) {
		msg := &Message{
			ID:      "to-delete",
			Topic:   "delete-topic",
			Content: "Delete me",
		}
		store.SaveMessage(ctx, msg)

		if err := store.DeleteMessage(ctx, "to-delete"); err != nil {
			t.Fatalf("DeleteMessage failed: %v", err)
		}

		_, err := store.GetMessage(ctx, "to-delete")
		if err != ErrNotFound {
			t.Error("Message should be deleted")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats, err := store.Stats(ctx)
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if stats.TotalMessages == 0 {
			t.Error("Expected some messages in stats")
		}
	})
}

// TestMemoryTaskStore tests the in-memory task store
func TestMemoryTaskStore(t *testing.T) {
	config := DefaultStoreConfig()
	config.Cleanup.Enabled = false
	store := NewMemoryTaskStore(config)
	defer store.Close()

	ctx := context.Background()

	t.Run("Ping", func(t *testing.T) {
		if err := store.Ping(ctx); err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	})

	t.Run("SaveAndGetTask", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "test-task-1",
			AgentID: "agent-1",
			Type:    "test",
			Status:  TaskStatusPending,
			Input:   map[string]interface{}{"key": "value"},
		}

		if err := store.SaveTask(ctx, task); err != nil {
			t.Fatalf("SaveTask failed: %v", err)
		}

		retrieved, err := store.GetTask(ctx, "test-task-1")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}

		if retrieved.AgentID != task.AgentID {
			t.Errorf("AgentID mismatch: got %s, want %s", retrieved.AgentID, task.AgentID)
		}
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "status-test",
			AgentID: "agent-1",
			Status:  TaskStatusPending,
		}
		store.SaveTask(ctx, task)

		result := map[string]string{"output": "success"}
		if err := store.UpdateStatus(ctx, "status-test", TaskStatusCompleted, result, ""); err != nil {
			t.Fatalf("UpdateStatus failed: %v", err)
		}

		retrieved, _ := store.GetTask(ctx, "status-test")
		if retrieved.Status != TaskStatusCompleted {
			t.Errorf("Status should be completed, got %s", retrieved.Status)
		}
		if retrieved.CompletedAt == nil {
			t.Error("CompletedAt should be set")
		}
	})

	t.Run("UpdateProgress", func(t *testing.T) {
		task := &AsyncTask{
			ID:       "progress-test",
			AgentID:  "agent-1",
			Status:   TaskStatusRunning,
			Progress: 0,
		}
		store.SaveTask(ctx, task)

		if err := store.UpdateProgress(ctx, "progress-test", 50.0); err != nil {
			t.Fatalf("UpdateProgress failed: %v", err)
		}

		retrieved, _ := store.GetTask(ctx, "progress-test")
		if retrieved.Progress != 50.0 {
			t.Errorf("Progress should be 50, got %f", retrieved.Progress)
		}
	})

	t.Run("ListTasks", func(t *testing.T) {
		// Create tasks with different statuses
		tasks := []*AsyncTask{
			{ID: "list-1", AgentID: "agent-1", Status: TaskStatusPending},
			{ID: "list-2", AgentID: "agent-1", Status: TaskStatusRunning},
			{ID: "list-3", AgentID: "agent-2", Status: TaskStatusCompleted},
		}
		for _, task := range tasks {
			store.SaveTask(ctx, task)
		}

		// Filter by status
		filter := TaskFilter{Status: []TaskStatus{TaskStatusPending}}
		result, err := store.ListTasks(ctx, filter)
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}

		pendingCount := 0
		for _, task := range result {
			if task.Status == TaskStatusPending {
				pendingCount++
			}
		}
		if pendingCount == 0 {
			t.Error("Expected at least one pending task")
		}

		// Filter by agent
		filter = TaskFilter{AgentID: "agent-2"}
		result, err = store.ListTasks(ctx, filter)
		if err != nil {
			t.Fatalf("ListTasks by agent failed: %v", err)
		}

		for _, task := range result {
			if task.AgentID != "agent-2" {
				t.Errorf("Expected agent-2, got %s", task.AgentID)
			}
		}
	})

	t.Run("GetRecoverableTasks", func(t *testing.T) {
		// Create recoverable tasks
		tasks := []*AsyncTask{
			{ID: "recover-1", AgentID: "agent-1", Status: TaskStatusPending, Priority: 1},
			{ID: "recover-2", AgentID: "agent-1", Status: TaskStatusRunning, Priority: 2},
			{ID: "recover-3", AgentID: "agent-1", Status: TaskStatusCompleted},
		}
		for _, task := range tasks {
			store.SaveTask(ctx, task)
		}

		result, err := store.GetRecoverableTasks(ctx)
		if err != nil {
			t.Fatalf("GetRecoverableTasks failed: %v", err)
		}

		// Should only get pending and running tasks
		for _, task := range result {
			if task.Status != TaskStatusPending && task.Status != TaskStatusRunning {
				t.Errorf("Got non-recoverable task: %s", task.Status)
			}
		}

		// Should be sorted by priority (higher first)
		if len(result) >= 2 && result[0].Priority < result[1].Priority {
			t.Error("Tasks should be sorted by priority descending")
		}
	})

	t.Run("DeleteTask", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "to-delete-task",
			AgentID: "agent-1",
			Status:  TaskStatusPending,
		}
		store.SaveTask(ctx, task)

		if err := store.DeleteTask(ctx, "to-delete-task"); err != nil {
			t.Fatalf("DeleteTask failed: %v", err)
		}

		_, err := store.GetTask(ctx, "to-delete-task")
		if err != ErrNotFound {
			t.Error("Task should be deleted")
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Create old completed task
		oldTask := &AsyncTask{
			ID:        "old-completed",
			AgentID:   "agent-1",
			Status:    TaskStatusCompleted,
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-48 * time.Hour),
		}
		now := time.Now().Add(-48 * time.Hour)
		oldTask.CompletedAt = &now
		store.SaveTask(ctx, oldTask)

		// Cleanup tasks older than 24 hours
		count, err := store.Cleanup(ctx, 24*time.Hour)
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		if count == 0 {
			t.Error("Expected at least one task to be cleaned up")
		}
	})

	t.Run("Stats", func(t *testing.T) {
		stats, err := store.Stats(ctx)
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if stats.TotalTasks == 0 {
			t.Error("Expected some tasks in stats")
		}
	})
}

// TestFileMessageStore tests the file-based message store
func TestFileMessageStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "persistence-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := DefaultStoreConfig()
	config.BaseDir = tmpDir
	config.Cleanup.Enabled = false

	store, err := NewFileMessageStore(config)
	if err != nil {
		t.Fatalf("Failed to create file message store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("SaveAndGetMessage", func(t *testing.T) {
		msg := &Message{
			ID:      "file-msg-1",
			Topic:   "file-topic",
			Content: "File message",
		}

		if err := store.SaveMessage(ctx, msg); err != nil {
			t.Fatalf("SaveMessage failed: %v", err)
		}

		retrieved, err := store.GetMessage(ctx, "file-msg-1")
		if err != nil {
			t.Fatalf("GetMessage failed: %v", err)
		}

		if retrieved.Content != msg.Content {
			t.Errorf("Content mismatch")
		}
	})

	t.Run("PersistenceAcrossRestart", func(t *testing.T) {
		msg := &Message{
			ID:      "persist-msg",
			Topic:   "persist-topic",
			Content: "Persistent message",
		}
		store.SaveMessage(ctx, msg)

		// Close and reopen store
		store.Close()

		store2, err := NewFileMessageStore(config)
		if err != nil {
			t.Fatalf("Failed to reopen store: %v", err)
		}
		defer store2.Close()

		retrieved, err := store2.GetMessage(ctx, "persist-msg")
		if err != nil {
			t.Fatalf("Message should persist: %v", err)
		}

		if retrieved.Content != msg.Content {
			t.Error("Content should match after restart")
		}
	})
}

// TestFileTaskStore tests the file-based task store
func TestFileTaskStore(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "persistence-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := DefaultStoreConfig()
	config.BaseDir = tmpDir
	config.Cleanup.Enabled = false

	store, err := NewFileTaskStore(config)
	if err != nil {
		t.Fatalf("Failed to create file task store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("SaveAndGetTask", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "file-task-1",
			AgentID: "agent-1",
			Status:  TaskStatusPending,
		}

		if err := store.SaveTask(ctx, task); err != nil {
			t.Fatalf("SaveTask failed: %v", err)
		}

		retrieved, err := store.GetTask(ctx, "file-task-1")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}

		if retrieved.AgentID != task.AgentID {
			t.Errorf("AgentID mismatch")
		}
	})

	t.Run("PersistenceAcrossRestart", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "persist-task",
			AgentID: "agent-1",
			Status:  TaskStatusRunning,
		}
		store.SaveTask(ctx, task)

		// Close and reopen store
		store.Close()

		store2, err := NewFileTaskStore(config)
		if err != nil {
			t.Fatalf("Failed to reopen store: %v", err)
		}
		defer store2.Close()

		retrieved, err := store2.GetTask(ctx, "persist-task")
		if err != nil {
			t.Fatalf("Task should persist: %v", err)
		}

		if retrieved.Status != TaskStatusRunning {
			t.Error("Status should match after restart")
		}
	})

	t.Run("RecoverableTasksAfterRestart", func(t *testing.T) {
		// Create recoverable tasks
		tasks := []*AsyncTask{
			{ID: "recover-file-1", AgentID: "agent-1", Status: TaskStatusPending},
			{ID: "recover-file-2", AgentID: "agent-1", Status: TaskStatusRunning},
		}
		for _, task := range tasks {
			store.SaveTask(ctx, task)
		}

		// Close and reopen store
		store.Close()

		store2, err := NewFileTaskStore(config)
		if err != nil {
			t.Fatalf("Failed to reopen store: %v", err)
		}
		defer store2.Close()

		result, err := store2.GetRecoverableTasks(ctx)
		if err != nil {
			t.Fatalf("GetRecoverableTasks failed: %v", err)
		}

		if len(result) < 2 {
			t.Errorf("Expected at least 2 recoverable tasks, got %d", len(result))
		}
	})
}

// TestRetryConfig tests the retry configuration
func TestRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()

	t.Run("CalculateBackoff", func(t *testing.T) {
		// First retry: 1s
		backoff := config.CalculateBackoff(0)
		if backoff != 1*time.Second {
			t.Errorf("Expected 1s, got %v", backoff)
		}

		// Second retry: 2s
		backoff = config.CalculateBackoff(1)
		if backoff != 2*time.Second {
			t.Errorf("Expected 2s, got %v", backoff)
		}

		// Third retry: 4s
		backoff = config.CalculateBackoff(2)
		if backoff != 4*time.Second {
			t.Errorf("Expected 4s, got %v", backoff)
		}
	})

	t.Run("MaxBackoff", func(t *testing.T) {
		// Should not exceed max backoff
		backoff := config.CalculateBackoff(100)
		if backoff > config.MaxBackoff {
			t.Errorf("Backoff %v exceeds max %v", backoff, config.MaxBackoff)
		}
	})
}

// TestTaskStatus tests task status methods
func TestTaskStatus(t *testing.T) {
	t.Run("IsTerminal", func(t *testing.T) {
		terminalStatuses := []TaskStatus{
			TaskStatusCompleted,
			TaskStatusFailed,
			TaskStatusCancelled,
			TaskStatusTimeout,
		}

		for _, status := range terminalStatuses {
			if !status.IsTerminal() {
				t.Errorf("%s should be terminal", status)
			}
		}

		nonTerminalStatuses := []TaskStatus{
			TaskStatusPending,
			TaskStatusRunning,
		}

		for _, status := range nonTerminalStatuses {
			if status.IsTerminal() {
				t.Errorf("%s should not be terminal", status)
			}
		}
	})

	t.Run("IsRecoverable", func(t *testing.T) {
		recoverableStatuses := []TaskStatus{
			TaskStatusPending,
			TaskStatusRunning,
		}

		for _, status := range recoverableStatuses {
			if !status.IsRecoverable() {
				t.Errorf("%s should be recoverable", status)
			}
		}
	})
}

// TestMessage tests message methods
func TestMessage(t *testing.T) {
	t.Run("IsExpired", func(t *testing.T) {
		// Not expired (no expiry set)
		msg := &Message{ID: "test"}
		if msg.IsExpired() {
			t.Error("Message without expiry should not be expired")
		}

		// Not expired (future expiry)
		future := time.Now().Add(1 * time.Hour)
		msg.ExpiresAt = &future
		if msg.IsExpired() {
			t.Error("Message with future expiry should not be expired")
		}

		// Expired
		past := time.Now().Add(-1 * time.Hour)
		msg.ExpiresAt = &past
		if !msg.IsExpired() {
			t.Error("Message with past expiry should be expired")
		}
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		config := DefaultRetryConfig()

		// Should retry (not acked, not expired, under max retries)
		msg := &Message{ID: "test", RetryCount: 0}
		if !msg.ShouldRetry(config) {
			t.Error("Message should be retried")
		}

		// Should not retry (acked)
		now := time.Now()
		msg.AckedAt = &now
		if msg.ShouldRetry(config) {
			t.Error("Acked message should not be retried")
		}

		// Should not retry (max retries exceeded)
		msg.AckedAt = nil
		msg.RetryCount = config.MaxRetries
		if msg.ShouldRetry(config) {
			t.Error("Message at max retries should not be retried")
		}
	})
}

// TestAsyncTask tests async task methods
func TestAsyncTask(t *testing.T) {
	t.Run("Duration", func(t *testing.T) {
		task := &AsyncTask{ID: "test"}

		// No start time
		if task.Duration() != 0 {
			t.Error("Duration should be 0 without start time")
		}

		// Running task
		start := time.Now().Add(-5 * time.Minute)
		task.StartedAt = &start
		duration := task.Duration()
		if duration < 5*time.Minute {
			t.Errorf("Duration should be at least 5 minutes, got %v", duration)
		}

		// Completed task
		end := start.Add(2 * time.Minute)
		task.CompletedAt = &end
		if task.Duration() != 2*time.Minute {
			t.Errorf("Duration should be 2 minutes, got %v", task.Duration())
		}
	})

	t.Run("IsTimedOut", func(t *testing.T) {
		task := &AsyncTask{
			ID:      "test",
			Timeout: 1 * time.Minute,
		}

		// Not started
		if task.IsTimedOut() {
			t.Error("Task without start time should not be timed out")
		}

		// Not timed out
		recent := time.Now().Add(-30 * time.Second)
		task.StartedAt = &recent
		if task.IsTimedOut() {
			t.Error("Recent task should not be timed out")
		}

		// Timed out
		old := time.Now().Add(-2 * time.Minute)
		task.StartedAt = &old
		if !task.IsTimedOut() {
			t.Error("Old task should be timed out")
		}
	})

	t.Run("ShouldRetry", func(t *testing.T) {
		task := &AsyncTask{
			ID:         "test",
			Status:     TaskStatusFailed,
			RetryCount: 0,
			MaxRetries: 3,
		}

		if !task.ShouldRetry() {
			t.Error("Failed task under max retries should retry")
		}

		task.RetryCount = 3
		if task.ShouldRetry() {
			t.Error("Task at max retries should not retry")
		}

		task.Status = TaskStatusCompleted
		task.RetryCount = 0
		if task.ShouldRetry() {
			t.Error("Completed task should not retry")
		}
	})
}

// TestFactory tests the factory functions
func TestFactory(t *testing.T) {
	t.Run("NewMessageStore_Memory", func(t *testing.T) {
		config := DefaultStoreConfig()
		config.Type = StoreTypeMemory

		store, err := NewMessageStore(config)
		if err != nil {
			t.Fatalf("Failed to create memory message store: %v", err)
		}
		defer store.Close()

		if _, ok := store.(*MemoryMessageStore); !ok {
			t.Error("Expected MemoryMessageStore")
		}
	})

	t.Run("NewTaskStore_Memory", func(t *testing.T) {
		config := DefaultStoreConfig()
		config.Type = StoreTypeMemory

		store, err := NewTaskStore(config)
		if err != nil {
			t.Fatalf("Failed to create memory task store: %v", err)
		}
		defer store.Close()

		if _, ok := store.(*MemoryTaskStore); !ok {
			t.Error("Expected MemoryTaskStore")
		}
	})

	t.Run("NewMessageStore_File", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "factory-test-*")
		defer os.RemoveAll(tmpDir)

		config := DefaultStoreConfig()
		config.Type = StoreTypeFile
		config.BaseDir = tmpDir

		store, err := NewMessageStore(config)
		if err != nil {
			t.Fatalf("Failed to create file message store: %v", err)
		}
		defer store.Close()

		if _, ok := store.(*FileMessageStore); !ok {
			t.Error("Expected FileMessageStore")
		}
	})

	t.Run("NewTaskStore_File", func(t *testing.T) {
		tmpDir, _ := os.MkdirTemp("", "factory-test-*")
		defer os.RemoveAll(tmpDir)

		config := DefaultStoreConfig()
		config.Type = StoreTypeFile
		config.BaseDir = tmpDir

		store, err := NewTaskStore(config)
		if err != nil {
			t.Fatalf("Failed to create file task store: %v", err)
		}
		defer store.Close()

		if _, ok := store.(*FileTaskStore); !ok {
			t.Error("Expected FileTaskStore")
		}
	})

	t.Run("InvalidType", func(t *testing.T) {
		config := DefaultStoreConfig()
		config.Type = "invalid"

		_, err := NewMessageStore(config)
		if err == nil {
			t.Error("Expected error for invalid type")
		}

		_, err = NewTaskStore(config)
		if err == nil {
			t.Error("Expected error for invalid type")
		}
	})
}

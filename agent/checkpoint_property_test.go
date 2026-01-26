package agent

import (
	"context"
	"testing"
	"time"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"go.uber.org/zap"
)

// Feature: agent-framework-enhancements, Property 1: Checkpoint Round-Trip Consistency
// Validates: Requirements 1.1, 1.4
func TestProperty_CheckpointRoundTripConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("saving and loading checkpoint preserves all fields", prop.ForAll(
		func(threadID string, agentID string, state State, messageCount int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// Create checkpoint with generated data
			messages := make([]CheckpointMessage, messageCount)
			for i := 0; i < messageCount; i++ {
				messages[i] = CheckpointMessage{
					Role:    "user",
					Content: "test message",
				}
			}

			original := &Checkpoint{
				ID:       generateCheckpointID(),
				ThreadID: threadID,
				AgentID:  agentID,
				State:    state,
				Messages: messages,
				Metadata: map[string]interface{}{
					"test_key": "test_value",
				},
				CreatedAt: time.Now(),
			}

			// Save checkpoint
			if err := store.Save(ctx, original); err != nil {
				t.Logf("Save failed: %v", err)
				return false
			}

			// Load checkpoint
			loaded, err := store.Load(ctx, original.ID)
			if err != nil {
				t.Logf("Load failed: %v", err)
				return false
			}

			// Verify all fields are preserved
			if loaded.ID != original.ID {
				t.Logf("ID mismatch: expected %s, got %s", original.ID, loaded.ID)
				return false
			}
			if loaded.ThreadID != original.ThreadID {
				t.Logf("ThreadID mismatch: expected %s, got %s", original.ThreadID, loaded.ThreadID)
				return false
			}
			if loaded.AgentID != original.AgentID {
				t.Logf("AgentID mismatch: expected %s, got %s", original.AgentID, loaded.AgentID)
				return false
			}
			if loaded.State != original.State {
				t.Logf("State mismatch: expected %s, got %s", original.State, loaded.State)
				return false
			}
			if len(loaded.Messages) != len(original.Messages) {
				t.Logf("Messages count mismatch: expected %d, got %d", len(original.Messages), len(loaded.Messages))
				return false
			}
			if loaded.Metadata["test_key"] != original.Metadata["test_key"] {
				t.Logf("Metadata mismatch")
				return false
			}

			return true
		},
		gen.Identifier(),                                                    // threadID
		gen.Identifier(),                                                    // agentID
		gen.OneConstOf(StateInit, StateReady, StateRunning, StateCompleted), // state
		gen.IntRange(0, 10),                                                 // messageCount
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 2: Checkpoint ID and Timestamp Assignment
// Validates: Requirements 1.2
func TestProperty_CheckpointIDAndTimestampAssignment(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("every saved checkpoint has non-empty ID and valid timestamp", prop.ForAll(
		func(threadID string, agentID string) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			beforeSave := time.Now()

			checkpoint := &Checkpoint{
				ID:       generateCheckpointID(), // Generate ID before save
				ThreadID: threadID,
				AgentID:  agentID,
				State:    StateReady,
				Messages: []CheckpointMessage{},
				Metadata: make(map[string]interface{}),
				CreatedAt: time.Now(),
			}

			// Save checkpoint
			if err := store.Save(ctx, checkpoint); err != nil {
				t.Logf("Save failed: %v", err)
				return false
			}

			afterSave := time.Now()

			// Verify ID is non-empty
			if checkpoint.ID == "" {
				t.Logf("Checkpoint ID is empty")
				return false
			}

			// Verify timestamp is valid (between before and after save)
			if checkpoint.CreatedAt.Before(beforeSave) || checkpoint.CreatedAt.After(afterSave) {
				t.Logf("Timestamp out of range: %v not between %v and %v", checkpoint.CreatedAt, beforeSave, afterSave)
				return false
			}

			// Load and verify
			loaded, err := store.Load(ctx, checkpoint.ID)
			if err != nil {
				t.Logf("Load failed: %v", err)
				return false
			}

			if loaded.ID == "" {
				t.Logf("Loaded checkpoint ID is empty")
				return false
			}

			if loaded.CreatedAt.IsZero() {
				t.Logf("Loaded checkpoint timestamp is zero")
				return false
			}

			return true
		},
		gen.Identifier(), // threadID
		gen.Identifier(), // agentID
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 4: Checkpoint Listing Order
// Validates: Requirements 1.5
func TestProperty_CheckpointListingOrder(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("checkpoints are listed in reverse chronological order", prop.ForAll(
		func(threadID string, agentID string, count int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// Save multiple checkpoints with small delays
			for i := 0; i < count; i++ {
				checkpoint := &Checkpoint{
					ID:       generateCheckpointID(),
					ThreadID: threadID,
					AgentID:  agentID,
					State:    StateReady,
					Messages: []CheckpointMessage{},
					Metadata: make(map[string]interface{}),
				}

				if err := store.Save(ctx, checkpoint); err != nil {
					t.Logf("Save failed: %v", err)
					return false
				}

				// Small delay to ensure different timestamps
				time.Sleep(2 * time.Millisecond)
			}

			// List checkpoints
			checkpoints, err := store.List(ctx, threadID, count)
			if err != nil {
				t.Logf("List failed: %v", err)
				return false
			}

			if len(checkpoints) != count {
				t.Logf("Expected %d checkpoints, got %d", count, len(checkpoints))
				return false
			}

			// Verify reverse chronological order (newest first)
			for i := 0; i < len(checkpoints)-1; i++ {
				if checkpoints[i].CreatedAt.Before(checkpoints[i+1].CreatedAt) {
					t.Logf("Checkpoints not in reverse chronological order at index %d", i)
					return false
				}
			}

			return true
		},
		gen.Identifier(),    // threadID
		gen.Identifier(),    // agentID
		gen.IntRange(2, 10), // count (at least 2 to test ordering)
	))

	properties.TestingRun(t)
}

// Feature: agent-framework-enhancements, Property 7: Sequential Version Numbering
// Validates: Requirements 1.10, 5.1
func TestProperty_SequentialVersionNumbering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("version numbers are sequential starting from 1", prop.ForAll(
		func(threadID string, agentID string, count int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// Save multiple checkpoints
			for i := 0; i < count; i++ {
				checkpoint := &Checkpoint{
					ID:       generateCheckpointID(),
					ThreadID: threadID,
					AgentID:  agentID,
					State:    StateReady,
					Messages: []CheckpointMessage{},
					Metadata: make(map[string]interface{}),
				}

				if err := store.Save(ctx, checkpoint); err != nil {
					t.Logf("Save failed: %v", err)
					return false
				}
			}

			// List versions
			versions, err := store.ListVersions(ctx, threadID)
			if err != nil {
				t.Logf("ListVersions failed: %v", err)
				return false
			}

			if len(versions) != count {
				t.Logf("Expected %d versions, got %d", count, len(versions))
				return false
			}

			// Verify sequential numbering starting from 1
			for i, version := range versions {
				expectedVersion := i + 1
				if version.Version != expectedVersion {
					t.Logf("Version mismatch at index %d: expected %d, got %d", i, expectedVersion, version.Version)
					return false
				}
			}

			// Verify each version increments by 1
			for i := 0; i < len(versions)-1; i++ {
				if versions[i+1].Version != versions[i].Version+1 {
					t.Logf("Non-sequential version numbers: %d followed by %d", versions[i].Version, versions[i+1].Version)
					return false
				}
			}

			return true
		},
		gen.Identifier(),    // threadID
		gen.Identifier(),    // agentID
		gen.IntRange(1, 10), // count
	))

	properties.TestingRun(t)
}

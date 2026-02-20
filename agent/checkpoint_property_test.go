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

// 特征:代理-框架增强,财产1:检查站圆通-Trip一致性
// 审定:要求1.1、1.4
func TestProperty_CheckpointRoundTripConsistency(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("saving and loading checkpoint preserves all fields", prop.ForAll(
		func(threadID string, agentID string, state State, messageCount int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// 使用生成的数据创建检查点
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
				Metadata: map[string]any{
					"test_key": "test_value",
				},
				CreatedAt: time.Now(),
			}

			// 保存检查点
			if err := store.Save(ctx, original); err != nil {
				t.Logf("Save failed: %v", err)
				return false
			}

			// 装入检查站
			loaded, err := store.Load(ctx, original.ID)
			if err != nil {
				t.Logf("Load failed: %v", err)
				return false
			}

			// 验证所有字段保存
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

// 特性:代理-框架强化,财产2:检查站ID和时间戳
// 审定:要求1.2
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
				Metadata: make(map[string]any),
				CreatedAt: time.Now(),
			}

			// 保存检查点
			if err := store.Save(ctx, checkpoint); err != nil {
				t.Logf("Save failed: %v", err)
				return false
			}

			afterSave := time.Now()

			// 校验身份是非空的
			if checkpoint.ID == "" {
				t.Logf("Checkpoint ID is empty")
				return false
			}

			// 验证时间戳是有效的( 在保存前后之间)
			if checkpoint.CreatedAt.Before(beforeSave) || checkpoint.CreatedAt.After(afterSave) {
				t.Logf("Timestamp out of range: %v not between %v and %v", checkpoint.CreatedAt, beforeSave, afterSave)
				return false
			}

			// 装入并验证
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

// 特征:代理框架增强,财产4:检查站列名令
// 验证:要求 1.5
func TestProperty_CheckpointListingOrder(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("checkpoints are listed in reverse chronological order", prop.ForAll(
		func(threadID string, agentID string, count int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// 节省多处检查站,稍有延误
			for i := 0; i < count; i++ {
				checkpoint := &Checkpoint{
					ID:       generateCheckpointID(),
					ThreadID: threadID,
					AgentID:  agentID,
					State:    StateReady,
					Messages: []CheckpointMessage{},
					Metadata: make(map[string]any),
				}

				if err := store.Save(ctx, checkpoint); err != nil {
					t.Logf("Save failed: %v", err)
					return false
				}

				// 确保不同时间戳的少量延迟
				time.Sleep(2 * time.Millisecond)
			}

			// 列出检查站名单
			checkpoints, err := store.List(ctx, threadID, count)
			if err != nil {
				t.Logf("List failed: %v", err)
				return false
			}

			if len(checkpoints) != count {
				t.Logf("Expected %d checkpoints, got %d", count, len(checkpoints))
				return false
			}

			// 校验反向时间顺序( 最新第一)
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

// 特征:代理框架增强, 属性 7: 顺序版本编号
// 审定:要求 1.10, 5.1
func TestProperty_SequentialVersionNumbering(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("version numbers are sequential starting from 1", prop.ForAll(
		func(threadID string, agentID string, count int) bool {
			ctx := context.Background()
			logger, _ := zap.NewDevelopment()
			store, _ := NewFileCheckpointStore(t.TempDir(), logger)

			// 保存多个检查站
			for i := 0; i < count; i++ {
				checkpoint := &Checkpoint{
					ID:       generateCheckpointID(),
					ThreadID: threadID,
					AgentID:  agentID,
					State:    StateReady,
					Messages: []CheckpointMessage{},
					Metadata: make(map[string]any),
				}

				if err := store.Save(ctx, checkpoint); err != nil {
					t.Logf("Save failed: %v", err)
					return false
				}
			}

			// 列表版本
			versions, err := store.ListVersions(ctx, threadID)
			if err != nil {
				t.Logf("ListVersions failed: %v", err)
				return false
			}

			if len(versions) != count {
				t.Logf("Expected %d versions, got %d", count, len(versions))
				return false
			}

			// 从 1 开始验证相继编号
			for i, version := range versions {
				expectedVersion := i + 1
				if version.Version != expectedVersion {
					t.Logf("Version mismatch at index %d: expected %d, got %d", i, expectedVersion, version.Version)
					return false
				}
			}

			// 以 1 验证每个版本的增量
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

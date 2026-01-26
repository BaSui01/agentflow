package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFileCheckpointStore_SaveAndLoad(t *testing.T) {
	// 创建临时目录
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 创建检查点
	checkpoint := &Checkpoint{
		ID:       "test-checkpoint-1",
		ThreadID: "thread-1",
		AgentID:  "agent-1",
		State:    StateReady,
		Messages: []CheckpointMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		Metadata:  map[string]interface{}{"key": "value"},
		CreatedAt: time.Now(),
	}

	// 保存检查点
	err = store.Save(ctx, checkpoint)
	require.NoError(t, err)

	// 加载检查点
	loaded, err := store.Load(ctx, checkpoint.ID)
	require.NoError(t, err)
	assert.Equal(t, checkpoint.ID, loaded.ID)
	assert.Equal(t, checkpoint.ThreadID, loaded.ThreadID)
	assert.Equal(t, checkpoint.AgentID, loaded.AgentID)
	assert.Equal(t, checkpoint.State, loaded.State)
	assert.Equal(t, 1, loaded.Version) // 自动分配版本号
}

func TestFileCheckpointStore_LoadLatest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 保存多个检查点
	for i := 1; i <= 3; i++ {
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			State:     StateReady,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond) // 确保时间戳不同
	}

	// 加载最新检查点
	latest, err := store.LoadLatest(ctx, "thread-1")
	require.NoError(t, err)
	assert.Equal(t, 3, latest.Version) // 最新版本应该是3
}

func TestFileCheckpointStore_List(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 保存多个检查点
	for i := 1; i <= 5; i++ {
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			State:     StateReady,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// 列出所有检查点
	checkpoints, err := store.List(ctx, "thread-1", 10)
	require.NoError(t, err)
	assert.Equal(t, 5, len(checkpoints))

	// 验证按时间降序排序
	for i := 0; i < len(checkpoints)-1; i++ {
		assert.True(t, checkpoints[i].CreatedAt.After(checkpoints[i+1].CreatedAt))
	}

	// 测试限制数量
	limited, err := store.List(ctx, "thread-1", 3)
	require.NoError(t, err)
	assert.Equal(t, 3, len(limited))
}

func TestFileCheckpointStore_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	checkpoint := &Checkpoint{
		ID:        "test-checkpoint-1",
		ThreadID:  "thread-1",
		AgentID:   "agent-1",
		State:     StateReady,
		CreatedAt: time.Now(),
	}

	err = store.Save(ctx, checkpoint)
	require.NoError(t, err)

	// 删除检查点
	err = store.Delete(ctx, checkpoint.ID)
	require.NoError(t, err)

	// 验证已删除
	_, err = store.Load(ctx, checkpoint.ID)
	assert.Error(t, err)
}

func TestFileCheckpointStore_DeleteThread(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 保存多个检查点
	for i := 1; i <= 3; i++ {
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			State:     StateReady,
			CreatedAt: time.Now(),
		}
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err)
	}

	// 删除整个线程
	err = store.DeleteThread(ctx, "thread-1")
	require.NoError(t, err)

	// 验证线程已删除
	checkpoints, err := store.List(ctx, "thread-1", 10)
	require.NoError(t, err)
	assert.Equal(t, 0, len(checkpoints))
}

func TestFileCheckpointStore_Versioning(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 保存多个版本
	for i := 1; i <= 3; i++ {
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			State:     StateReady,
			CreatedAt: time.Now(),
		}
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// 列出版本
	versions, err := store.ListVersions(ctx, "thread-1")
	require.NoError(t, err)
	assert.Equal(t, 3, len(versions))

	// 验证版本号递增
	for i, v := range versions {
		assert.Equal(t, i+1, v.Version)
	}

	// 加载特定版本
	v2, err := store.LoadVersion(ctx, "thread-1", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, v2.Version)
}

func TestFileCheckpointStore_Rollback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 保存多个版本
	for i := 1; i <= 3; i++ {
		checkpoint := &Checkpoint{
			ID:        generateCheckpointID(),
			ThreadID:  "thread-1",
			AgentID:   "agent-1",
			State:     StateReady,
			Metadata:  map[string]interface{}{"version": i},
			CreatedAt: time.Now(),
		}
		err = store.Save(ctx, checkpoint)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// 回滚到版本2
	err = store.Rollback(ctx, "thread-1", 2)
	require.NoError(t, err)

	// 验证创建了新版本
	versions, err := store.ListVersions(ctx, "thread-1")
	require.NoError(t, err)
	assert.Equal(t, 4, len(versions)) // 原来3个 + 回滚创建的1个

	// 验证最新版本是回滚版本
	latest, err := store.LoadLatest(ctx, "thread-1")
	require.NoError(t, err)
	assert.Equal(t, 4, latest.Version)
	// JSON 反序列化会将数字转为 float64
	rollbackVersion, ok := latest.Metadata["rollback_from_version"].(float64)
	require.True(t, ok, "rollback_from_version should be float64")
	assert.Equal(t, float64(2), rollbackVersion)
}

func TestFileCheckpointStore_ConcurrentAccess(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// 并发保存检查点
	const numGoroutines = 10
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			// 在 goroutine 内部生成唯一 ID,避免竞态条件
			time.Sleep(time.Millisecond) // 确保时间戳不同
			checkpoint := &Checkpoint{
				ID:        generateCheckpointID(),
				ThreadID:  "thread-1",
				AgentID:   "agent-1",
				State:     StateReady,
				CreatedAt: time.Now(),
			}
			err := store.Save(ctx, checkpoint)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证所有检查点都已保存
	checkpoints, err := store.List(ctx, "thread-1", 100)
	require.NoError(t, err)
	assert.Equal(t, numGoroutines, len(checkpoints))
}

func TestFileCheckpointStore_DirectoryStructure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "checkpoint-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	logger, _ := zap.NewDevelopment()
	store, err := NewFileCheckpointStore(tmpDir, logger)
	require.NoError(t, err)

	ctx := context.Background()

	checkpoint := &Checkpoint{
		ID:        "test-checkpoint-1",
		ThreadID:  "thread-1",
		AgentID:   "agent-1",
		State:     StateReady,
		CreatedAt: time.Now(),
	}

	err = store.Save(ctx, checkpoint)
	require.NoError(t, err)

	// 验证目录结构
	threadDir := filepath.Join(tmpDir, "threads", "thread-1")
	assert.DirExists(t, threadDir)

	checkpointsDir := filepath.Join(threadDir, "checkpoints")
	assert.DirExists(t, checkpointsDir)

	checkpointFile := filepath.Join(checkpointsDir, "test-checkpoint-1.json")
	assert.FileExists(t, checkpointFile)

	versionsFile := filepath.Join(threadDir, "versions.json")
	assert.FileExists(t, versionsFile)

	latestFile := filepath.Join(threadDir, "latest.txt")
	assert.FileExists(t, latestFile)
}

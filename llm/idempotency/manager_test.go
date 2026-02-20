package idempotency

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// NewMemoryManager
// ---------------------------------------------------------------------------

func TestNewMemoryManager(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	require.NotNil(t, m)

	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	assert.NotNil(t, mm.cache)
	assert.Equal(t, 5*time.Minute, mm.cleanupInterval)
}

func TestNewMemoryManagerWithCleanup(t *testing.T) {
	m := NewMemoryManagerWithCleanup(zap.NewNop(), 100*time.Millisecond)
	require.NotNil(t, m)

	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	assert.Equal(t, 100*time.Millisecond, mm.cleanupInterval)
}

// ---------------------------------------------------------------------------
// GenerateKey
// ---------------------------------------------------------------------------

func TestMemoryManager_GenerateKey(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })
	tests := []struct {
		name    string
		inputs  []any
		wantErr bool
	}{
		{
			name:    "single string input",
			inputs:  []any{"hello"},
			wantErr: false,
		},
		{
			name:    "multiple inputs",
			inputs:  []any{"model", "prompt", 42},
			wantErr: false,
		},
		{
			name:    "empty inputs returns error",
			inputs:  []any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := m.GenerateKey(tt.inputs...)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, key)
			assert.Len(t, key, 64) // SHA256 hex = 64 chars
		})
	}
}

func TestMemoryManager_GenerateKey_Consistency(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	key1, err := m.GenerateKey("same", "input")
	require.NoError(t, err)

	key2, err := m.GenerateKey("same", "input")
	require.NoError(t, err)

	assert.Equal(t, key1, key2, "same inputs should produce same key")
}

func TestMemoryManager_GenerateKey_Uniqueness(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	key1, _ := m.GenerateKey("input-a")
	key2, _ := m.GenerateKey("input-b")
	assert.NotEqual(t, key1, key2, "different inputs should produce different keys")
}

// ---------------------------------------------------------------------------
// Set + Get
// ---------------------------------------------------------------------------

func TestMemoryManager_SetAndGet(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	err := m.Set(ctx, "k1", map[string]string{"result": "ok"}, time.Hour)
	require.NoError(t, err)

	data, found, err := m.Get(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, found)

	var result map[string]string
	require.NoError(t, json.Unmarshal(data, &result))
	assert.Equal(t, "ok", result["result"])
}

func TestMemoryManager_Get_NotFound(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	data, found, err := m.Get(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, data)
}

func TestMemoryManager_Set_DefaultTTL(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	// TTL <= 0 should default to 1 hour
	err := m.Set(context.Background(), "k1", "val", 0)
	require.NoError(t, err)

	mm.mu.RLock()
	entry := mm.cache["k1"]
	mm.mu.RUnlock()

	require.NotNil(t, entry)
	// Should expire roughly 1 hour from now
	assert.WithinDuration(t, time.Now().Add(time.Hour), entry.ExpiresAt, 5*time.Second)
}

// ---------------------------------------------------------------------------
// Exists
// ---------------------------------------------------------------------------

func TestMemoryManager_Exists(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	exists, err := m.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, exists)

	_ = m.Set(ctx, "k1", "val", time.Hour)

	exists, err = m.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.True(t, exists)
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestMemoryManager_Delete(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	_ = m.Set(ctx, "k1", "val", time.Hour)

	err := m.Delete(ctx, "k1")
	require.NoError(t, err)

	_, found, err := m.Get(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestMemoryManager_Delete_NonExistent(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	// Deleting a non-existent key should not error
	err := m.Delete(context.Background(), "nope")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// TTL expiration
// ---------------------------------------------------------------------------

func TestMemoryManager_TTLExpiration_Get(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	// Set with very short TTL
	err := m.Set(ctx, "k1", "val", 50*time.Millisecond)
	require.NoError(t, err)

	// Immediately available
	_, found, _ := m.Get(ctx, "k1")
	assert.True(t, found)

	// Wait for expiration
	time.Sleep(80 * time.Millisecond)

	_, found, err = m.Get(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, found, "entry should be expired")
}

func TestMemoryManager_TTLExpiration_Exists(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	_ = m.Set(ctx, "k1", "val", 50*time.Millisecond)
	time.Sleep(80 * time.Millisecond)

	exists, err := m.Exists(ctx, "k1")
	require.NoError(t, err)
	assert.False(t, exists, "expired entry should not exist")
}

// ---------------------------------------------------------------------------
// Background cleanup
// ---------------------------------------------------------------------------

func TestMemoryManager_BackgroundCleanup(t *testing.T) {
	m := NewMemoryManagerWithCleanup(zap.NewNop(), 50*time.Millisecond)
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	_ = m.Set(ctx, "k1", "val", 30*time.Millisecond)

	// Wait for TTL + cleanup interval
	time.Sleep(120 * time.Millisecond)

	mm.mu.RLock()
	_, exists := mm.cache["k1"]
	mm.mu.RUnlock()
	assert.False(t, exists, "cleanup should have removed expired entry")
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestMemoryManager_ConcurrentSafety(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := "key-" + string(rune('a'+i%26))
			_ = m.Set(ctx, key, i, time.Hour)
			_, _, _ = m.Get(ctx, key)
			_, _ = m.Exists(ctx, key)
		}(i)
	}

	wg.Wait()
	// No panic or race = pass
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func TestMemoryManager_ImplementsManager(t *testing.T) {
	var _ Manager = (*memoryManager)(nil)
}

// ---------------------------------------------------------------------------
// GetTyped / SetTyped (generic wrappers)
// ---------------------------------------------------------------------------

func TestGetTyped_Success(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	type payload struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	}

	// Store via SetTyped
	err := SetTyped[payload](m, ctx, "k1", payload{Name: "test", Score: 99}, time.Hour)
	require.NoError(t, err)

	// Retrieve via GetTyped
	val, found, err := GetTyped[payload](m, ctx, "k1")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "test", val.Name)
	assert.Equal(t, 99, val.Score)
}

func TestGetTyped_NotFound(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	val, found, err := GetTyped[string](m, context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, "", val)
}

func TestGetTyped_UnmarshalError(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	// Store a string value
	err := m.Set(ctx, "k1", "hello", time.Hour)
	require.NoError(t, err)

	// Try to unmarshal as struct — should fail
	type complex struct {
		Field int `json:"field"`
	}
	_, found, err := GetTyped[complex](m, ctx, "k1")
	assert.Error(t, err)
	assert.False(t, found)
	assert.Contains(t, err.Error(), "unmarshal cached result")
}

func TestSetTyped_PrimitiveTypes(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	// int
	require.NoError(t, SetTyped[int](m, ctx, "int-key", 42, time.Hour))
	intVal, found, err := GetTyped[int](m, ctx, "int-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 42, intVal)

	// string
	require.NoError(t, SetTyped[string](m, ctx, "str-key", "hello", time.Hour))
	strVal, found, err := GetTyped[string](m, ctx, "str-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "hello", strVal)

	// []int
	require.NoError(t, SetTyped[[]int](m, ctx, "slice-key", []int{1, 2, 3}, time.Hour))
	sliceVal, found, err := GetTyped[[]int](m, ctx, "slice-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, []int{1, 2, 3}, sliceVal)
}

func TestGetTyped_MapType(t *testing.T) {
	m := NewMemoryManager(zap.NewNop())
	mm := m.(*memoryManager)
	t.Cleanup(func() { mm.Close() })

	ctx := context.Background()

	data := map[string]int{"a": 1, "b": 2}
	require.NoError(t, SetTyped[map[string]int](m, ctx, "map-key", data, time.Hour))

	val, found, err := GetTyped[map[string]int](m, ctx, "map-key")
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, 1, val["a"])
	assert.Equal(t, 2, val["b"])
}

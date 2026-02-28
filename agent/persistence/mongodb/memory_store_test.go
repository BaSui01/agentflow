//go:build integration

package mongodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/config"
	mongoclient "github.com/BaSui01/agentflow/pkg/mongodb"
	"go.uber.org/zap"
)

// testMongoClient creates a MongoDB client for integration tests.
// It reads the connection URI from MONGO_URI env var (default: mongodb://localhost:27017).
func testMongoClient(t *testing.T) *mongoclient.Client {
	t.Helper()

	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	cfg := config.MongoDBConfig{
		URI:      uri,
		Database: fmt.Sprintf("agentflow_test_%d", time.Now().UnixNano()),
	}

	client, err := mongoclient.NewClient(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("failed to create mongo client: %v", err)
	}

	t.Cleanup(func() {
		// Drop the test database on cleanup.
		_ = client.Database().Drop(context.Background())
		_ = client.Close(context.Background())
	})

	return client
}

func TestMongoMemoryStore_SaveAndLoad(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewMemoryStore(ctx, client)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	// Save a value.
	err = store.Save(ctx, "key1", "value1", 0)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load it back.
	val, err := store.Load(ctx, "key1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if val != "value1" {
		t.Errorf("Load got %v, want %q", val, "value1")
	}
}

func TestMongoMemoryStore_Delete(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewMemoryStore(ctx, client)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	_ = store.Save(ctx, "del-key", "del-value", 0)

	if err := store.Delete(ctx, "del-key"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Load(ctx, "del-key")
	if err == nil {
		t.Error("expected error loading deleted key, got nil")
	}
}

func TestMongoMemoryStore_List(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewMemoryStore(ctx, client)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	_ = store.Save(ctx, "list:a:1", map[string]any{"key": "list:a:1"}, 0)
	_ = store.Save(ctx, "list:a:2", map[string]any{"key": "list:a:2"}, 0)
	_ = store.Save(ctx, "list:b:1", map[string]any{"key": "list:b:1"}, 0)

	items, err := store.List(ctx, "list:a:*", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("List got %d items, want 2", len(items))
	}
}

func TestMongoMemoryStore_Clear(t *testing.T) {
	client := testMongoClient(t)
	ctx := context.Background()

	store, err := NewMemoryStore(ctx, client)
	if err != nil {
		t.Fatalf("NewMemoryStore: %v", err)
	}

	_ = store.Save(ctx, "clear1", "v1", 0)
	_ = store.Save(ctx, "clear2", "v2", 0)

	if err := store.Clear(ctx); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	items, err := store.List(ctx, "*", 100)
	if err != nil {
		t.Fatalf("List after Clear: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items after Clear, got %d", len(items))
	}
}
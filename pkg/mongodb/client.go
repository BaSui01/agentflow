package mongodb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"

	"github.com/BaSui01/agentflow/config"
)

// Client wraps the MongoDB driver client with health checks and graceful shutdown.
// Mirrors the patterns in pkg/database/pool.go.
type Client struct {
	client   *mongo.Client
	database *mongo.Database
	cfg      config.MongoDBConfig
	logger   *zap.Logger
	mu       sync.RWMutex
	closed   bool
}

// NewClient creates a new MongoDB client, pings the server, and starts
// a background health-check goroutine.
func NewClient(cfg config.MongoDBConfig, logger *zap.Logger) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	logger = logger.With(zap.String("component", "mongodb"))

	opts, err := BuildClientOptions(cfg)
	if err != nil {
		return nil, fmt.Errorf("build mongo options: %w", err)
	}

	driver, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	dbName := cfg.Database
	if dbName == "" {
		dbName = "agentflow"
	}

	c := &Client{
		client:   driver,
		database: driver.Database(dbName),
		cfg:      cfg,
		logger:   logger,
	}

	// Initial ping to verify connectivity.
	pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx); err != nil {
		// Best-effort disconnect on failed initial ping.
		_ = driver.Disconnect(context.Background())
		return nil, fmt.Errorf("mongo initial ping: %w", err)
	}

	// Start background health check.
	interval := cfg.HealthCheckInterval
	if interval <= 0 {
		interval = DefaultHealthCheckInterval
	}
	go c.healthCheckLoop(interval)

	logger.Info("mongodb client initialized",
		zap.String("database", dbName),
		zap.Int("max_pool_size", cfg.MaxPoolSize),
	)

	return c, nil
}

// Database returns the default *mongo.Database.
func (c *Client) Database() *mongo.Database {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.database
}

// Collection is a convenience shortcut for Database().Collection(name).
func (c *Client) Collection(name string) *mongo.Collection {
	return c.Database().Collection(name)
}

// Ping checks MongoDB connectivity.
func (c *Client) Ping(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return fmt.Errorf("mongodb client is closed")
	}
	return c.client.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
}

// Close disconnects the MongoDB client gracefully.
func (c *Client) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.logger.Info("closing mongodb client")
	return c.client.Disconnect(ctx)
}

// EnsureIndexes creates indexes on the given collection. Intended to be
// called once during application startup.
func (c *Client) EnsureIndexes(ctx context.Context, collName string, models []mongo.IndexModel) error {
	coll := c.Collection(collName)
	_, err := coll.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("ensure indexes on %s: %w", collName, err)
	}
	return nil
}

// healthCheckLoop periodically pings MongoDB.
func (c *Client) healthCheckLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.RLock()
		if c.closed {
			c.mu.RUnlock()
			return
		}
		c.mu.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.Ping(ctx); err != nil {
			c.logger.Error("mongodb health check failed", zap.Error(err))
		} else {
			c.logger.Debug("mongodb health check passed")
		}
		cancel()
	}
}

// NewClientFromOptions creates a Client from pre-built driver options.
// Useful for testing with custom options.
func NewClientFromOptions(opts *options.ClientOptions, dbName string, logger *zap.Logger) (*Client, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	logger = logger.With(zap.String("component", "mongodb"))

	driver, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("mongo connect: %w", err)
	}

	if dbName == "" {
		dbName = "agentflow"
	}

	c := &Client{
		client:   driver,
		database: driver.Database(dbName),
		logger:   logger,
	}

	return c, nil
}

package bootstrap

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/storage"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// BuildMultimodalRedisReferenceStore creates the redis client and reference store for multimodal runtime.
func BuildMultimodalRedisReferenceStore(
	cfg *config.Config,
	keyPrefix string,
	ttl time.Duration,
	logger *zap.Logger,
) (*redis.Client, storage.ReferenceStore, error) {
	addr := strings.TrimSpace(cfg.Redis.Addr)
	if addr == "" {
		return nil, nil, fmt.Errorf("redis address is required when multimodal reference_store_backend=redis")
	}

	var (
		opts *redis.Options
		err  error
	)

	if strings.HasPrefix(addr, "redis://") || strings.HasPrefix(addr, "rediss://") {
		parsed, parseErr := url.Parse(addr)
		if parseErr != nil {
			return nil, nil, fmt.Errorf("invalid redis url: %w", parseErr)
		}
		scheme := strings.ToLower(parsed.Scheme)
		host := parsed.Hostname()
		if scheme == "redis" && !IsLoopbackHost(host) {
			return nil, nil, fmt.Errorf("insecure redis:// is only allowed for loopback hosts, use rediss:// for %q", host)
		}

		opts, err = redis.ParseURL(addr)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid redis url: %w", err)
		}
		if cfg.Redis.Password != "" && opts.Password == "" {
			opts.Password = cfg.Redis.Password
		}
		if cfg.Redis.DB != 0 && opts.DB == 0 {
			opts.DB = cfg.Redis.DB
		}
		if cfg.Redis.PoolSize > 0 {
			opts.PoolSize = cfg.Redis.PoolSize
		}
		if cfg.Redis.MinIdleConns > 0 {
			opts.MinIdleConns = cfg.Redis.MinIdleConns
		}
		if scheme == "rediss" && opts.TLSConfig == nil {
			opts.TLSConfig = tlsutil.DefaultTLSConfig()
		}
		if scheme == "redis" && IsLoopbackHost(host) {
			logger.Warn("using insecure redis:// for loopback host in multimodal reference store",
				zap.String("host", host))
		}
	} else {
		host := hostFromAddr(addr)
		if !IsLoopbackHost(host) {
			return nil, nil, fmt.Errorf("non-loopback redis address %q requires rediss:// scheme", host)
		}

		opts = &redis.Options{
			Addr:         addr,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
		}
		logger.Warn("using insecure plaintext redis connection for loopback host in multimodal reference store",
			zap.String("host", host))
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return client, storage.NewRedisReferenceStore(client, keyPrefix, ttl, logger), nil
}

func hostFromAddr(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(addr)
}

// IsLoopbackHost reports whether host resolves to loopback host/ip form.
func IsLoopbackHost(host string) bool {
	h := strings.TrimSpace(strings.Trim(host, "[]"))
	if h == "" {
		return false
	}
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// Package mongodb provides MongoDB client management for agentflow.
//
// Supports connection pooling, health checks, and graceful shutdown,
// mirroring the patterns in pkg/database/pool.go.
package mongodb

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/pkg/tlsutil"
)

// BuildClientOptions converts a config.MongoDBConfig into mongo driver options.
func BuildClientOptions(cfg config.MongoDBConfig) (*options.ClientOptions, error) {
	opts := options.Client()

	// URI takes precedence; otherwise build from host/port.
	uri := cfg.URI
	if uri == "" {
		uri = buildURI(cfg)
	}
	opts.ApplyURI(uri)

	// Pool sizing.
	if cfg.MaxPoolSize > 0 {
		opts.SetMaxPoolSize(uint64(cfg.MaxPoolSize))
	}
	if cfg.MinPoolSize > 0 {
		opts.SetMinPoolSize(uint64(cfg.MinPoolSize))
	}

	// Timeouts.
	if cfg.ConnectTimeout > 0 {
		opts.SetConnectTimeout(cfg.ConnectTimeout)
	}
	if cfg.Timeout > 0 {
		opts.SetTimeout(cfg.Timeout)
	}

	// TLS hardening (§32).
	if cfg.TLSEnabled {
		tlsCfg, err := buildTLSConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("mongodb tls config: %w", err)
		}
		opts.SetTLSConfig(tlsCfg)
	}

	return opts, nil
}

// buildURI constructs a mongodb:// URI from discrete config fields.
func buildURI(cfg config.MongoDBConfig) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := cfg.Port
	if port == 0 {
		port = 27017
	}

	var userInfo string
	if cfg.User != "" {
		userInfo = cfg.User
		if cfg.Password != "" {
			userInfo += ":" + cfg.Password
		}
		userInfo += "@"
	}

	uri := fmt.Sprintf("mongodb://%s%s:%d", userInfo, host, port)

	params := ""
	if cfg.AuthSource != "" {
		params += "authSource=" + cfg.AuthSource
	}
	if cfg.ReplicaSet != "" {
		if params != "" {
			params += "&"
		}
		params += "replicaSet=" + cfg.ReplicaSet
	}
	if params != "" {
		uri += "/?" + params
	}

	return uri
}

// buildTLSConfig creates a *tls.Config following §32 TLS hardening.
func buildTLSConfig(cfg config.MongoDBConfig) (*tls.Config, error) {
	// Start from the project-wide hardened defaults.
	tlsCfg := tlsutil.DefaultTLSConfig()

	// Load CA certificate if provided.
	if cfg.TLSCAFile != "" {
		caCert, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate")
		}
		tlsCfg.RootCAs = pool
	}

	// Load client certificate if provided.
	if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
		cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("load client cert: %w", err)
		}
		tlsCfg.Certificates = []tls.Certificate{cert}
	}

	return tlsCfg, nil
}

// DefaultHealthCheckInterval is used when the config value is zero.
const DefaultHealthCheckInterval = 30 * time.Second

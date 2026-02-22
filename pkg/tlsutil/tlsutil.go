// Package tlsutil provides centralized TLS configuration for all HTTP clients,
// servers, and Redis connections in agentflow.
// 安全加固：TLS 1.2+，仅 AEAD 密码套件。
package tlsutil

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// DefaultTLSConfig returns a hardened TLS configuration.
// MinVersion TLS 1.2, AEAD-only cipher suites.
func DefaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}
}

// SecureTransport returns an http.Transport with TLS hardening.
func SecureTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: DefaultTLSConfig(),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

// SecureHTTPClient returns an http.Client with TLS hardening.
// Drop-in replacement for &http.Client{Timeout: timeout}.
func SecureHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: SecureTransport(),
	}
}

package tlsutil

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestDefaultTLSConfig(t *testing.T) {
	cfg := DefaultTLSConfig()
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", cfg.MinVersion, tls.VersionTLS12)
	}
	if len(cfg.CipherSuites) == 0 {
		t.Error("CipherSuites should not be empty")
	}
	// Verify all cipher suites are AEAD
	for _, cs := range cfg.CipherSuites {
		switch cs {
		case tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305:
			// OK — AEAD cipher suite
		default:
			t.Errorf("unexpected non-AEAD cipher suite: %d", cs)
		}
	}
}

func TestSecureTransport(t *testing.T) {
	tr := SecureTransport()
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig should not be nil")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("Transport TLS MinVersion = %d, want %d",
			tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 should be true")
	}
}

func TestSecureHTTPClient(t *testing.T) {
	timeout := 15 * time.Second
	client := SecureHTTPClient(timeout)
	if client.Timeout != timeout {
		t.Errorf("Timeout = %v, want %v", client.Timeout, timeout)
	}
	if client.Transport == nil {
		t.Fatal("Transport should not be nil")
	}
}

package httpclient

import (
	"crypto/tls"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewFactory_Defaults(t *testing.T) {
	f := NewFactory()
	client := f.Client()
	if client == nil {
		t.Fatal("expected non-nil default client")
	}
	if client.Timeout != 30*time.Second {
		t.Fatalf("expected 30s timeout, got %v", client.Timeout)
	}
}

func TestNewFactory_WithOptions(t *testing.T) {
	f := NewFactory(
		WithTimeout(10*time.Second),
		WithMaxIdleConns(50),
	)
	client := f.Client()
	if client.Timeout != 10*time.Second {
		t.Fatalf("expected 10s timeout, got %v", client.Timeout)
	}
	tr := client.Transport.(*http.Transport)
	if tr.MaxIdleConns != 50 {
		t.Fatalf("expected 50 max idle conns, got %d", tr.MaxIdleConns)
	}
}

func TestNewFactory_WithTLSConfig(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	f := NewFactory(WithTLSConfig(tlsCfg))
	tr := f.Client().Transport.(*http.Transport)
	if tr.TLSClientConfig != tlsCfg {
		t.Fatal("expected TLS config to be set on default transport")
	}
}

func TestClientForHost_HostIsolation(t *testing.T) {
	host1TLS := &tls.Config{MinVersion: tls.VersionTLS12}
	host2TLS := &tls.Config{MinVersion: tls.VersionTLS13}

	f := NewFactory(
		WithHostTLS("api.example.com", host1TLS),
		WithHostTLS("api.other.com", host2TLS),
	)

	c1 := f.ClientForHost("api.example.com")
	c2 := f.ClientForHost("api.other.com")

	if c1 == c2 {
		t.Fatal("expected different clients for different hosts")
	}

	tr1 := c1.Transport.(*http.Transport)
	tr2 := c2.Transport.(*http.Transport)

	if tr1.TLSClientConfig != host1TLS {
		t.Fatal("host1 should have its own TLS config")
	}
	if tr2.TLSClientConfig != host2TLS {
		t.Fatal("host2 should have its own TLS config")
	}
}

func TestClientForHost_FallbackToDefault(t *testing.T) {
	f := NewFactory()
	c := f.ClientForHost("unknown.host")
	if c != f.Client() {
		t.Fatal("unknown host should fall back to default client")
	}
}

func TestClientForHost_ConcurrentAccess(t *testing.T) {
	f := NewFactory(
		WithHostTLS("api.example.com", &tls.Config{MinVersion: tls.VersionTLS12}),
	)

	var count atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := f.ClientForHost("api.example.com")
			if c == nil {
				t.Error("expected non-nil client")
				return
			}
			count.Add(1)
		}()
	}
	wg.Wait()

	if count.Load() != 100 {
		t.Fatalf("expected 100 successful accesses, got %d", count.Load())
	}
}

func TestClientForHost_SameClientReturned(t *testing.T) {
	f := NewFactory(
		WithHostTLS("api.example.com", &tls.Config{MinVersion: tls.VersionTLS12}),
	)
	c1 := f.ClientForHost("api.example.com")
	c2 := f.ClientForHost("api.example.com")
	if c1 != c2 {
		t.Fatal("same host should return same client instance")
	}
}

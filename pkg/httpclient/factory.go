package httpclient

import (
	"crypto/tls"
	"net/http"
	"sync"
	"time"
)

type Factory struct {
	defaultClient *http.Client
	hostClients   sync.Map
	defaultOpts   *options
}

type options struct {
	timeout       time.Duration
	tlsConfig     *tls.Config
	maxIdleConns  int
	hostTLS       map[string]*tls.Config
}

type Option func(*options)

func WithTimeout(d time.Duration) Option {
	return func(o *options) { o.timeout = d }
}

func WithTLSConfig(cfg *tls.Config) Option {
	return func(o *options) { o.tlsConfig = cfg }
}

func WithMaxIdleConns(n int) Option {
	return func(o *options) { o.maxIdleConns = n }
}

func WithHostTLS(host string, cfg *tls.Config) Option {
	return func(o *options) {
		if o.hostTLS == nil {
			o.hostTLS = make(map[string]*tls.Config)
		}
		o.hostTLS[host] = cfg
	}
}

func defaultOptions() *options {
	return &options{
		timeout:      30 * time.Second,
		maxIdleConns: 100,
		hostTLS:      make(map[string]*tls.Config),
	}
}

func NewFactory(opts ...Option) *Factory {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	transport := &http.Transport{
		MaxIdleConns:        o.maxIdleConns,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if o.tlsConfig != nil {
		transport.TLSClientConfig = o.tlsConfig
	}

	f := &Factory{
		defaultOpts: o,
		defaultClient: &http.Client{
			Timeout:   o.timeout,
			Transport: transport,
		},
	}

	for host, tlsCfg := range o.hostTLS {
		f.hostClients.Store(host, f.newClientForTLS(tlsCfg, o))
	}

	return f
}

func (f *Factory) newClientForTLS(tlsCfg *tls.Config, o *options) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        o.maxIdleConns,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		TLSClientConfig:     tlsCfg,
	}
	return &http.Client{
		Timeout:   o.timeout,
		Transport: transport,
	}
}

func (f *Factory) Client() *http.Client {
	return f.defaultClient
}

func (f *Factory) ClientForHost(host string) *http.Client {
	if v, ok := f.hostClients.Load(host); ok {
		return v.(*http.Client)
	}

	if tlsCfg, ok := f.defaultOpts.hostTLS[host]; ok {
		client := f.newClientForTLS(tlsCfg, f.defaultOpts)
		actual, loaded := f.hostClients.LoadOrStore(host, client)
		if loaded {
			return actual.(*http.Client)
		}
		return client
	}

	return f.defaultClient
}

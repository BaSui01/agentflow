package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SSETransport struct {
	baseURL    string
	httpClient *http.Client
	headers    map[string]string
	sendCh     chan *MCPMessage
	recvCh     chan *MCPMessage
	done       chan struct{}
	once       sync.Once
}

// SSETransportOption configures optional SSETransport settings.
type SSETransportOption func(*SSETransport)

// WithAuthToken sets a Bearer token on all outgoing requests.
func WithAuthToken(token string) SSETransportOption {
	return func(t *SSETransport) {
		t.headers["Authorization"] = "Bearer " + token
	}
}

// WithSSEHeader sets a custom header on all outgoing requests.
func WithSSEHeader(key, value string) SSETransportOption {
	return func(t *SSETransport) {
		t.headers[key] = value
	}
}

func NewSSETransport(baseURL string, opts ...SSETransportOption) *SSETransport {
	t := &SSETransport{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{},
		headers:    make(map[string]string),
		sendCh:     make(chan *MCPMessage, 16),
		recvCh:     make(chan *MCPMessage, 16),
		done:       make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	go t.sendLoop()
	go t.recvLoop()
	return t
}

func (s *SSETransport) doneCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-s.done
		cancel()
	}()
	return ctx
}

func (s *SSETransport) sendLoop() {
	ctx := s.doneCtx()
	for msg := range s.sendCh {
		body, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/message", bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range s.headers {
			req.Header.Set(k, v)
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			continue
		}
		_ = resp.Body.Close()
	}
}

func (s *SSETransport) recvLoop() {
	ctx := s.doneCtx()
	backoff := 100 * time.Millisecond
	const maxBackoff = 30 * time.Second
	for {
		select {
		case <-s.done:
			return
		default:
		}
		s.connectAndRead(ctx)
		select {
		case <-s.done:
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (s *SSETransport) connectAndRead(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/events", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	scanner := bufio.NewScanner(resp.Body)
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data.WriteString(strings.TrimPrefix(line, "data: "))
			data.WriteByte('\n')
		} else if line == "" && data.Len() > 0 {
			raw := strings.TrimSpace(data.String())
			data.Reset()
			if raw == "" {
				continue
			}
			var msg MCPMessage
			if err := json.Unmarshal([]byte(raw), &msg); err != nil {
				continue
			}
			select {
			case s.recvCh <- &msg:
			case <-s.done:
				return
			}
		}
	}
}

func (s *SSETransport) Send(ctx context.Context, msg *MCPMessage) error {
	select {
	case <-s.done:
		return fmt.Errorf("transport closed")
	case <-ctx.Done():
		return ctx.Err()
	case s.sendCh <- msg:
		return nil
	}
}

func (s *SSETransport) Receive(ctx context.Context) (*MCPMessage, error) {
	select {
	case <-s.done:
		return nil, fmt.Errorf("transport closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg := <-s.recvCh:
		return msg, nil
	}
}

func (s *SSETransport) Close() error {
	s.once.Do(func() {
		close(s.done)
		close(s.sendCh)
	})
	return nil
}

func (s *SSETransport) IsAlive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

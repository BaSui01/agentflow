package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
)

type StdioTransport struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	encoder *json.Encoder
	decoder *json.Decoder
	mu      sync.Mutex
	closed  bool
}

func NewStdioTransport(command string, args ...string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("start process: %w", err)
	}
	br := bufio.NewReader(stdout)
	return &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		encoder: json.NewEncoder(stdin),
		decoder: json.NewDecoder(br),
	}, nil
}

func (s *StdioTransport) Send(ctx context.Context, msg *MCPMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	if err := s.encoder.Encode(msg); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	return nil
}

func (s *StdioTransport) Receive(ctx context.Context) (*MCPMessage, error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, io.ErrClosedPipe
	}
	dec := s.decoder
	s.mu.Unlock()

	var msg MCPMessage
	if err := dec.Decode(&msg); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &msg, nil
}

func (s *StdioTransport) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	_ = s.stdin.Close()
	_ = s.stdout.Close()
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
	return nil
}

func (s *StdioTransport) IsAlive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

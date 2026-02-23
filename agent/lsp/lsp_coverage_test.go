package lsp

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// --- parseMessageID ---

func TestParseMessageID(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantID   int64
		wantOK   bool
	}{
		{"float64", float64(42), 42, true},
		{"int64", int64(7), 7, true},
		{"int", int(3), 3, true},
		{"json.Number", json.Number("99"), 99, true},
		{"json.Number invalid", json.Number("abc"), 0, false},
		{"string", "123", 123, true},
		{"string invalid", "xyz", 0, false},
		{"nil", nil, 0, false},
		{"bool", true, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, ok := parseMessageID(tt.input)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantID, id)
			}
		})
	}
}

// --- SetCapabilities ---

func TestLSPServer_SetCapabilities(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()
	server := NewLSPServer(ServerInfo{Name: "test"}, r, w, zap.NewNop())
	caps := ServerCapabilities{HoverProvider: true}
	server.SetCapabilities(caps)
	assert.True(t, server.capabilities.HoverProvider)
}

// --- RegisterNotificationHandler ---

func TestLSPClient_RegisterNotificationHandler(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	client := NewLSPClient(r, w, zap.NewNop())
	called := false
	client.RegisterNotificationHandler("test/method", func(method string, params json.RawMessage) {
		called = true
	})

	client.handlersMu.RLock()
	_, exists := client.notificationHandlers["test/method"]
	client.handlersMu.RUnlock()
	assert.True(t, exists)
	_ = called
}

// --- NewLSPClient nil logger ---

func TestNewLSPClient_NilLogger(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	client := NewLSPClient(r, w, nil)
	assert.NotNil(t, client)
}

// --- NewLSPServer nil logger ---

func TestNewLSPServer_NilLogger(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	server := NewLSPServer(ServerInfo{Name: "test"}, r, w, nil)
	assert.NotNil(t, server)
}

// --- positionToOffset ---

func TestPositionToOffset(t *testing.T) {
	text := "line0\nline1\nline2"

	t.Run("valid", func(t *testing.T) {
		offset, err := positionToOffset(text, Position{Line: 1, Character: 2})
		require.NoError(t, err)
		assert.Equal(t, 8, offset) // "line0\n" = 6, + "li" = 2
	})

	t.Run("negative line", func(t *testing.T) {
		_, err := positionToOffset(text, Position{Line: -1, Character: 0})
		assert.Error(t, err)
	})

	t.Run("line out of range", func(t *testing.T) {
		_, err := positionToOffset(text, Position{Line: 99, Character: 0})
		assert.Error(t, err)
	})

	t.Run("character out of range", func(t *testing.T) {
		_, err := positionToOffset(text, Position{Line: 0, Character: 99})
		assert.Error(t, err)
	})
}

// --- applyTextChange ---

func TestApplyTextChange(t *testing.T) {
	t.Run("full replace", func(t *testing.T) {
		result, err := applyTextChange("old text", TextDocumentContentChangeEvent{
			Text: "new text",
		})
		require.NoError(t, err)
		assert.Equal(t, "new text", result)
	})

	t.Run("range replace", func(t *testing.T) {
		text := "hello world"
		result, err := applyTextChange(text, TextDocumentContentChangeEvent{
			Range: &Range{
				Start: Position{Line: 0, Character: 6},
				End:   Position{Line: 0, Character: 11},
			},
			Text: "Go",
		})
		require.NoError(t, err)
		assert.Equal(t, "hello Go", result)
	})

	t.Run("invalid start position", func(t *testing.T) {
		_, err := applyTextChange("text", TextDocumentContentChangeEvent{
			Range: &Range{
				Start: Position{Line: 99, Character: 0},
				End:   Position{Line: 99, Character: 1},
			},
			Text: "x",
		})
		assert.Error(t, err)
	})
}

// --- rangesEqual ---

func TestRangesEqual(t *testing.T) {
	r1 := Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 3, Character: 4}}
	r2 := Range{Start: Position{Line: 1, Character: 2}, End: Position{Line: 3, Character: 4}}
	r3 := Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}}

	assert.True(t, rangesEqual(r1, r2))
	assert.False(t, rangesEqual(r1, r3))
}

// --- lineAt ---

func TestLineAt(t *testing.T) {
	text := "line0\nline1\nline2"

	line, ok := lineAt(text, 1)
	assert.True(t, ok)
	assert.Equal(t, "line1", line)

	_, ok = lineAt(text, -1)
	assert.False(t, ok)

	_, ok = lineAt(text, 99)
	assert.False(t, ok)
}

// --- SendNotification ---

func TestLSPServer_SendNotification(t *testing.T) {
	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	server := NewLSPServer(ServerInfo{Name: "test"}, clientToServerReader, serverToClientWriter, zap.NewNop())
	_ = serverToClientReader

	// Read in background to prevent blocking
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := serverToClientReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	err := server.SendNotification("test/notification", map[string]string{"key": "value"})
	assert.NoError(t, err)

	err = server.SendNotification("test/nil", nil)
	assert.NoError(t, err)

	clientToServerWriter.Close()
	serverToClientWriter.Close()
	clientToServerReader.Close()
}

// --- handleMessage: unknown method ---

func TestLSPServer_HandleMessage_UnknownMethod(t *testing.T) {
	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	server := NewLSPServer(ServerInfo{Name: "test"}, clientToServerReader, serverToClientWriter, zap.NewNop())

	// Read responses in background
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := serverToClientReader.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Unknown method with ID -> sendError
	server.handleMessage(context.Background(), &LSPMessage{
		JSONRPC: "2.0",
		ID:      float64(1),
		Method:  "unknown/method",
	})

	// Unknown method without ID -> just logged
	server.handleMessage(context.Background(), &LSPMessage{
		JSONRPC: "2.0",
		Method:  "unknown/notification",
	})

	clientToServerWriter.Close()
	serverToClientWriter.Close()
	clientToServerReader.Close()
}

// --- Client handleMessage: notification dispatch ---

func TestLSPClient_HandleMessage_Notification(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	client := NewLSPClient(r, w, zap.NewNop())

	notified := make(chan string, 1)
	client.RegisterNotificationHandler("test/notify", func(method string, params json.RawMessage) {
		notified <- method
	})

	client.handleMessage(&LSPMessage{
		JSONRPC: "2.0",
		Method:  "test/notify",
		Params:  json.RawMessage(`{}`),
	})

	select {
	case m := <-notified:
		assert.Equal(t, "test/notify", m)
	case <-time.After(time.Second):
		t.Fatal("notification handler not called")
	}
}

// --- Client handleMessage: response with no pending ---

func TestLSPClient_HandleMessage_ResponseNoPending(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	defer w.Close()

	client := NewLSPClient(r, w, zap.NewNop())

	// Response with ID but no pending request - should not panic
	client.handleMessage(&LSPMessage{
		JSONRPC: "2.0",
		ID:      float64(999),
		Result:  "some result",
	})
}

// --- Integration: incremental text change ---

func TestLSPServerClient_IncrementalChange(t *testing.T) {
	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	logger := zap.NewNop()
	server := NewLSPServer(ServerInfo{Name: "test-lsp", Version: "1.0.0"}, clientToServerReader, serverToClientWriter, logger)
	client := NewLSPClient(serverToClientReader, clientToServerWriter, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _ = server.Start(ctx) }()
	go func() { defer wg.Done(); _ = client.Start(ctx) }()

	t.Cleanup(func() {
		cancel()
		clientToServerWriter.Close()
		serverToClientWriter.Close()
		wg.Wait()
	})

	initCtx, initCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer initCancel()

	_, err := client.Initialize(initCtx, InitializeParams{
		ProcessID: 1, RootURI: "file:///workspace",
	})
	require.NoError(t, err)

	uri := "file:///test.go"
	source := "package main\n\nfunc hello() {}\n"

	require.NoError(t, client.TextDocumentDidOpen(DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{URI: uri, LanguageID: "go", Version: 1, Text: source},
	}))

	// Incremental change: replace "hello" with "world"
	require.NoError(t, client.TextDocumentDidChange(DidChangeTextDocumentParams{
		TextDocument: VersionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []TextDocumentContentChangeEvent{
			{
				Range: &Range{
					Start: Position{Line: 2, Character: 5},
					End:   Position{Line: 2, Character: 10},
				},
				Text: "world",
			},
		},
	}))

	reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer reqCancel()

	// Verify the change took effect by checking symbols
	symbols, err := client.TextDocumentDocumentSymbol(reqCtx, DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	})
	require.NoError(t, err)
	assert.True(t, hasDocumentSymbol(symbols, "world"))

	// Hover on the changed function
	hover, err := client.TextDocumentHover(reqCtx, HoverParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 7},
	})
	require.NoError(t, err)
	assert.NotNil(t, hover)
	assert.Contains(t, hover.Contents.Value, "world")
}

// --- identifierPrefixAtPosition ---

func TestIdentifierPrefixAtPosition(t *testing.T) {
	text := "func hello() {}\n"

	prefix, err := identifierPrefixAtPosition(text, Position{Line: 0, Character: 7})
	require.NoError(t, err)
	assert.Equal(t, "he", prefix)

	// Out of range line
	prefix, err = identifierPrefixAtPosition(text, Position{Line: 99, Character: 0})
	require.NoError(t, err)
	assert.Equal(t, "", prefix)

	// Negative character
	_, err = identifierPrefixAtPosition(text, Position{Line: 0, Character: -1})
	assert.Error(t, err)
}

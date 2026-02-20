package agent

import (
	"context"
	"io"
	"strings"
	"sync"

	agentlsp "github.com/BaSui01/agentflow/agent/lsp"
	"go.uber.org/zap"
)

const (
	defaultLSPServerName    = "agentflow-lsp"
	defaultLSPServerVersion = "0.1.0"
)

// ManagedLSP 封装了进程内的 LSP client/server 及其生命周期。
type ManagedLSP struct {
	Client *agentlsp.LSPClient
	Server *agentlsp.LSPServer

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	clientToServerReader *io.PipeReader
	clientToServerWriter *io.PipeWriter
	serverToClientReader *io.PipeReader
	serverToClientWriter *io.PipeWriter

	logger *zap.Logger
}

// NewManagedLSP 创建并启动一个进程内的 LSP runtime。
func NewManagedLSP(info agentlsp.ServerInfo, logger *zap.Logger) *ManagedLSP {
	if logger == nil {
		logger = zap.NewNop()
	}

	if strings.TrimSpace(info.Name) == "" {
		info.Name = defaultLSPServerName
	}
	if strings.TrimSpace(info.Version) == "" {
		info.Version = defaultLSPServerVersion
	}

	clientToServerReader, clientToServerWriter := io.Pipe()
	serverToClientReader, serverToClientWriter := io.Pipe()

	server := agentlsp.NewLSPServer(info, clientToServerReader, serverToClientWriter, logger)
	client := agentlsp.NewLSPClient(serverToClientReader, clientToServerWriter, logger)

	runtimeCtx, cancel := context.WithCancel(context.Background())
	runtime := &ManagedLSP{
		Client:               client,
		Server:               server,
		ctx:                  runtimeCtx,
		cancel:               cancel,
		clientToServerReader: clientToServerReader,
		clientToServerWriter: clientToServerWriter,
		serverToClientReader: serverToClientReader,
		serverToClientWriter: serverToClientWriter,
		logger:               logger.With(zap.String("component", "managed_lsp")),
	}

	runtime.start()

	return runtime
}

func (m *ManagedLSP) start() {
	m.wg.Add(2)

	go func() {
		defer m.wg.Done()
		if err := m.Server.Start(m.ctx); err != nil && err != context.Canceled {
			m.logger.Debug("managed lsp server stopped", zap.Error(err))
		}
	}()

	go func() {
		defer m.wg.Done()
		if err := m.Client.Start(m.ctx); err != nil && err != context.Canceled {
			m.logger.Debug("managed lsp client loop stopped", zap.Error(err))
		}
	}()
}

// Close 关闭 runtime 并回收后台 goroutine。
func (m *ManagedLSP) Close() error {
	if m == nil {
		return nil
	}

	m.cancel()
	_ = m.clientToServerReader.Close()
	_ = m.clientToServerWriter.Close()
	_ = m.serverToClientReader.Close()
	_ = m.serverToClientWriter.Close()
	m.wg.Wait()
	return nil
}

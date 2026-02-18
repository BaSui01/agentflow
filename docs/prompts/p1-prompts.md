# P1 优化提示词

## P1-1: Streaming 双向通信补全

### 需求背景
`agent/streaming/bidirectional.go` 存在两个严重问题：
1. `processInbound`（第 173-197 行）从 `s.outbound` channel 读取数据（第 180 行），逻辑反了，应该从底层连接读取数据写入 `s.inbound`
2. `processOutbound`（第 199-208 行）是空壳，只有 `select done/ctx.Done`，没有任何实际发送逻辑

此外缺少心跳机制、重连逻辑和完善的错误处理。

### 需要修改的文件

#### 文件：agent/streaming/bidirectional.go

**当前有问题的代码：**

`processInbound`（第 173-197 行）错误地从 outbound channel 读取：
```go
func (s *BidirectionalStream) processInbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case chunk := <-s.outbound: // BUG: 应该从底层连接读取，不是从 outbound
			if s.handler != nil {
				response, err := s.handler.OnInbound(ctx, chunk)
				if err != nil {
					s.logger.Error("inbound handler error", zap.Error(err))
					continue
				}
				if response != nil {
					select {
					case s.inbound <- *response:
					default:
						s.logger.Warn("inbound buffer full")
					}
				}
			}
		}
	}
}
```

`processOutbound`（第 199-208 行）是空壳：
```go
func (s *BidirectionalStream) processOutbound(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		}
	}
}
```

### 修改步骤

**步骤 1 — 添加 StreamConnection 接口和心跳配置**

在 `BidirectionalStream` 结构体之前添加底层连接接口：

```go
// StreamConnection 底层流式连接接口（WebSocket、gRPC stream 等）
type StreamConnection interface {
	// ReadChunk 从连接读取一个数据块（阻塞直到有数据或出错）
	ReadChunk(ctx context.Context) (*StreamChunk, error)
	// WriteChunk 向连接写入一个数据块
	WriteChunk(ctx context.Context, chunk StreamChunk) error
	// Close 关闭连接
	Close() error
	// IsAlive 检查连接是否存活
	IsAlive() bool
}
```

在 `StreamConfig` 结构体（第 37-45 行）中添加心跳和重连配置：
```go
type StreamConfig struct {
	BufferSize     int           `json:"buffer_size"`
	MaxLatencyMS   int           `json:"max_latency_ms"`
	SampleRate     int           `json:"sample_rate"`
	Channels       int           `json:"channels"`
	EnableVAD      bool          `json:"enable_vad"`
	ChunkDuration  time.Duration `json:"chunk_duration"`
	ReconnectDelay time.Duration `json:"reconnect_delay"`
	// 新增字段
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`  // 心跳间隔，默认 30s
	HeartbeatTimeout  time.Duration `json:"heartbeat_timeout"`   // 心跳超时，默认 10s
	MaxReconnects     int           `json:"max_reconnects"`      // 最大重连次数，默认 5
	EnableHeartbeat   bool          `json:"enable_heartbeat"`    // 是否启用心跳
}
```

更新 `DefaultStreamConfig`（第 48-58 行）：
```go
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		BufferSize:        1024,
		MaxLatencyMS:      200,
		SampleRate:        16000,
		Channels:          1,
		EnableVAD:         true,
		ChunkDuration:     100 * time.Millisecond,
		ReconnectDelay:    time.Second,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  10 * time.Second,
		MaxReconnects:     5,
		EnableHeartbeat:   true,
	}
}
```

**步骤 2 — 扩展 BidirectionalStream 结构体**

修改 `BidirectionalStream`（第 61-72 行），添加连接和重连相关字段：
```go
type BidirectionalStream struct {
	ID       string
	Config   StreamConfig
	State    StreamState
	inbound  chan StreamChunk
	outbound chan StreamChunk
	handler  StreamHandler
	conn     StreamConnection // 新增：底层连接
	logger   *zap.Logger
	mu       sync.RWMutex
	done     chan struct{}
	sequence int64
	// 新增字段
	connFactory   func() (StreamConnection, error) // 连接工厂，用于重连
	reconnectCount int
	lastHeartbeat  time.Time
	errChan        chan error // 内部错误通道
}
```

更新 `NewBidirectionalStream`（第 94-108 行）：
```go
func NewBidirectionalStream(
	config StreamConfig,
	handler StreamHandler,
	conn StreamConnection,
	connFactory func() (StreamConnection, error),
	logger *zap.Logger,
) *BidirectionalStream {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &BidirectionalStream{
		ID:          fmt.Sprintf("stream_%d", time.Now().UnixNano()),
		Config:      config,
		State:       StateDisconnected,
		inbound:     make(chan StreamChunk, config.BufferSize),
		outbound:    make(chan StreamChunk, config.BufferSize),
		handler:     handler,
		conn:        conn,
		connFactory: connFactory,
		logger:      logger.With(zap.String("component", "bidirectional_stream")),
		done:        make(chan struct{}),
		errChan:     make(chan error, 16),
	}
}
```

**步骤 3 — 修复 processInbound：从底层连接读取数据**

替换整个 `processInbound` 方法（第 173-197 行）：
```go
func (s *BidirectionalStream) processInbound(ctx context.Context) {
	defer s.logger.Debug("processInbound exited")
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		default:
		}

		// 从底层连接读取数据
		chunk, err := s.conn.ReadChunk(ctx)
		if err != nil {
			// 检查是否是正常关闭
			select {
			case <-s.done:
				return
			case <-ctx.Done():
				return
			default:
			}

			s.logger.Error("connection read error", zap.Error(err))
			s.errChan <- fmt.Errorf("inbound read error: %w", err)

			// 尝试重连
			if s.tryReconnect(ctx) {
				continue
			}
			return
		}

		if chunk == nil {
			continue
		}

		// 更新心跳时间
		s.mu.Lock()
		s.lastHeartbeat = time.Now()
		s.mu.Unlock()

		// 跳过心跳包
		if chunk.Type == "heartbeat" {
			continue
		}

		// 调用 handler 处理入站数据
		if s.handler != nil {
			response, err := s.handler.OnInbound(ctx, *chunk)
			if err != nil {
				s.logger.Error("inbound handler error", zap.Error(err))
				continue
			}
			if response != nil {
				select {
				case s.inbound <- *response:
				case <-s.done:
					return
				default:
					s.logger.Warn("inbound buffer full, dropping chunk",
						zap.Int64("sequence", response.Sequence))
				}
			}
		} else {
			// 没有 handler 时直接写入 inbound channel
			select {
			case s.inbound <- *chunk:
			case <-s.done:
				return
			default:
				s.logger.Warn("inbound buffer full, dropping chunk",
					zap.Int64("sequence", chunk.Sequence))
			}
		}
	}
}
```

**步骤 4 — 补全 processOutbound：从 outbound channel 读取并写入连接**

替换整个 `processOutbound` 方法（第 199-208 行）：
```go
func (s *BidirectionalStream) processOutbound(ctx context.Context) {
	defer s.logger.Debug("processOutbound exited")
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case chunk := <-s.outbound:
			// 调用 handler 预处理出站数据
			if s.handler != nil {
				if err := s.handler.OnOutbound(ctx, chunk); err != nil {
					s.logger.Error("outbound handler error",
						zap.Error(err),
						zap.Int64("sequence", chunk.Sequence))
					continue
				}
			}

			// 写入底层连接
			if err := s.conn.WriteChunk(ctx, chunk); err != nil {
				s.logger.Error("connection write error", zap.Error(err))
				s.errChan <- fmt.Errorf("outbound write error: %w", err)

				// 尝试重连后重发
				if s.tryReconnect(ctx) {
					// 重连成功，重新发送当前 chunk
					if retryErr := s.conn.WriteChunk(ctx, chunk); retryErr != nil {
						s.logger.Error("retry write failed after reconnect", zap.Error(retryErr))
					}
					continue
				}
				return
			}
		}
	}
}
```

**步骤 5 — 添加心跳机制**

在 `processOutbound` 方法之后添加：
```go
// processHeartbeat 定期发送心跳并检测超时
func (s *BidirectionalStream) processHeartbeat(ctx context.Context) {
	if !s.Config.EnableHeartbeat {
		return
	}

	ticker := time.NewTicker(s.Config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case <-ticker.C:
			// 发送心跳
			heartbeat := StreamChunk{
				Type:      "heartbeat",
				Timestamp: time.Now(),
				Metadata:  map[string]any{"ping": true},
			}
			if err := s.conn.WriteChunk(ctx, heartbeat); err != nil {
				s.logger.Warn("heartbeat send failed", zap.Error(err))
				s.errChan <- fmt.Errorf("heartbeat failed: %w", err)
			}

			// 检查对端心跳超时
			s.mu.RLock()
			lastBeat := s.lastHeartbeat
			s.mu.RUnlock()

			if !lastBeat.IsZero() && time.Since(lastBeat) > s.Config.HeartbeatTimeout+s.Config.HeartbeatInterval {
				s.logger.Warn("heartbeat timeout detected",
					zap.Duration("since_last", time.Since(lastBeat)))
				s.errChan <- fmt.Errorf("heartbeat timeout: last=%v", lastBeat)

				// 尝试重连
				if !s.tryReconnect(ctx) {
					s.setState(StateError)
					return
				}
			}
		}
	}
}
```

**步骤 6 — 添加重连逻辑**

```go
// tryReconnect 尝试重新建立连接
func (s *BidirectionalStream) tryReconnect(ctx context.Context) bool {
	if s.connFactory == nil {
		s.logger.Error("no connection factory, cannot reconnect")
		return false
	}

	s.mu.Lock()
	if s.reconnectCount >= s.Config.MaxReconnects {
		s.mu.Unlock()
		s.logger.Error("max reconnect attempts reached",
			zap.Int("attempts", s.reconnectCount))
		s.setState(StateError)
		return false
	}
	s.reconnectCount++
	attempt := s.reconnectCount
	s.mu.Unlock()

	s.setState(StateConnecting)
	s.logger.Info("attempting reconnect",
		zap.Int("attempt", attempt),
		zap.Int("max", s.Config.MaxReconnects))

	// 指数退避
	delay := s.Config.ReconnectDelay * time.Duration(1<<uint(attempt-1))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

	select {
	case <-ctx.Done():
		return false
	case <-s.done:
		return false
	case <-time.After(delay):
	}

	// 关闭旧连接
	if s.conn != nil {
		_ = s.conn.Close()
	}

	// 创建新连接
	newConn, err := s.connFactory()
	if err != nil {
		s.logger.Error("reconnect failed", zap.Error(err), zap.Int("attempt", attempt))
		return s.tryReconnect(ctx) // 递归重试
	}

	s.mu.Lock()
	s.conn = newConn
	s.lastHeartbeat = time.Now()
	s.mu.Unlock()

	s.setState(StateConnected)
	s.logger.Info("reconnected successfully", zap.Int("attempt", attempt))

	// 重置重连计数
	s.mu.Lock()
	s.reconnectCount = 0
	s.mu.Unlock()

	return true
}
```

**步骤 7 — 更新 Start 方法启动心跳和错误监控**

修改 `Start` 方法（第 111-123 行）：
```go
func (s *BidirectionalStream) Start(ctx context.Context) error {
	s.setState(StateConnecting)
	s.logger.Info("starting bidirectional stream")

	// 验证连接
	if s.conn == nil && s.connFactory != nil {
		conn, err := s.connFactory()
		if err != nil {
			s.setState(StateError)
			return fmt.Errorf("failed to establish connection: %w", err)
		}
		s.conn = conn
	}
	if s.conn == nil {
		s.setState(StateError)
		return fmt.Errorf("no connection available")
	}

	s.setState(StateConnected)

	s.mu.Lock()
	s.lastHeartbeat = time.Now()
	s.mu.Unlock()

	// 启动处理协程
	go s.processInbound(ctx)
	go s.processOutbound(ctx)
	go s.processHeartbeat(ctx)
	go s.monitorErrors(ctx)

	s.setState(StateStreaming)
	return nil
}

// monitorErrors 监控内部错误
func (s *BidirectionalStream) monitorErrors(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.done:
			return
		case err := <-s.errChan:
			s.logger.Warn("stream error detected", zap.Error(err))
			// 连续错误可以触发状态变更
			if s.GetState() == StateError {
				return
			}
		}
	}
}
```

**步骤 8 — 更新 Close 方法确保资源清理**

修改 `Close` 方法（第 150-162 行）：
```go
func (s *BidirectionalStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.State == StateDisconnected {
		return nil
	}

	close(s.done)
	s.State = StateDisconnected

	// 关闭底层连接
	var connErr error
	if s.conn != nil {
		connErr = s.conn.Close()
	}

	// 排空 channel
	close(s.inbound)
	close(s.outbound)

	s.logger.Info("stream closed")
	return connErr
}
```

**步骤 9 — 更新 StreamManager.CreateStream 签名**

修改 `CreateStream`（第 274-280 行）以传入连接参数：
```go
func (m *StreamManager) CreateStream(
	config StreamConfig,
	handler StreamHandler,
	conn StreamConnection,
	connFactory func() (StreamConnection, error),
) *BidirectionalStream {
	stream := NewBidirectionalStream(config, handler, conn, connFactory, m.logger)
	m.mu.Lock()
	m.streams[stream.ID] = stream
	m.mu.Unlock()
	return stream
}
```

### 验证方法
```bash
# 编译检查
cd D:/code/agentflow && go build ./agent/streaming/...

# 运行现有测试
go test ./agent/streaming/... -v -race

# 验证 channel 方向正确性：
# 1. processInbound 应该从 conn.ReadChunk 读取，写入 s.inbound
# 2. processOutbound 应该从 s.outbound 读取，写入 conn.WriteChunk
# 3. Send() 写入 s.outbound，Receive() 返回 s.inbound
```

### 注意事项
- `NewBidirectionalStream` 签名变更会影响所有调用方，需要同步更新 `StreamManager.CreateStream` 和测试代码
- 向后兼容：如果 `conn` 为 nil 且 `connFactory` 为 nil，保持原有行为（纯 channel 模式）
- 心跳包使用 `Type: "heartbeat"` 标识，processInbound 中需要过滤掉
- 重连使用指数退避，最大延迟 30 秒
- `Close()` 中关闭 inbound/outbound channel 可能导致 panic（向已关闭 channel 发送），需要在 processInbound/processOutbound 中用 select + done 保护

---
## P1-2: RAG Contextual Retrieval 增强

### 需求背景
`rag/contextual_retrieval.go` 存在三个问题：
1. `calculateContextRelevance`（第 234-254 行）只做简单单词重叠计算，准确率低
2. `chunkDocument`（第 171-198 行）硬编码 500 字符分块（第 180 行），不支持滑动窗口和 overlap
3. `CacheContexts` 配置字段存在（第 32 行）但完全未实现缓存逻辑

### 需要修改的文件

#### 文件：rag/contextual_retrieval.go

**改动 1 — 添加 BM25 相关配置和缓存结构**

在 `ContextualRetrievalConfig` 结构体（第 21-33 行）中添加分块和缓存配置：
```go
type ContextualRetrievalConfig struct {
	// 上下文生成
	UseContextPrefix    bool   `json:"use_context_prefix"`
	ContextTemplate     string `json:"context_template"`
	MaxContextLength    int    `json:"max_context_length"`

	// 检索增强
	UseReranking        bool    `json:"use_reranking"`
	ContextWeight       float64 `json:"context_weight"`

	// 缓存
	CacheContexts       bool          `json:"cache_contexts"`
	CacheTTL            time.Duration `json:"cache_ttl"`             // 缓存过期时间，默认 1h

	// 分块配置（新增）
	ChunkSize           int     `json:"chunk_size"`              // 分块大小，默认 500
	ChunkOverlap        int     `json:"chunk_overlap"`           // 重叠大小，默认 50
	ChunkByTokens       bool    `json:"chunk_by_tokens"`         // 按 token 还是字符分块

	// BM25 参数（新增）
	BM25K1              float64 `json:"bm25_k1"`                 // BM25 k1 参数，默认 1.2
	BM25B               float64 `json:"bm25_b"`                  // BM25 b 参数，默认 0.75
}
```

更新 `DefaultContextualRetrievalConfig`（第 36-45 行）：
```go
func DefaultContextualRetrievalConfig() ContextualRetrievalConfig {
	return ContextualRetrievalConfig{
		UseContextPrefix:  true,
		ContextTemplate:   "Document: {{document_title}}\nSection: {{section_title}}\nContext: {{context}}\n\nContent: {{content}}",
		MaxContextLength:  200,
		UseReranking:      true,
		ContextWeight:     0.4,
		CacheContexts:     true,
		CacheTTL:          time.Hour,
		ChunkSize:         500,
		ChunkOverlap:      50,
		ChunkByTokens:     false,
		BM25K1:            1.2,
		BM25B:             0.75,
	}
}
```

**改动 2 — 添加缓存结构和 ContextualRetrieval 扩展**

在 import 中添加 `"math"`, `"sync"`, `"time"`, `"crypto/sha256"`, `"encoding/hex"`。

在 `ContextualRetrieval` 结构体（第 13-18 行）中添加缓存：
```go
type ContextualRetrieval struct {
	retriever       *HybridRetriever
	contextProvider ContextProvider
	config          ContextualRetrievalConfig
	logger          *zap.Logger
	// 新增
	contextCache    sync.Map           // key: docID+chunkHash -> *cacheEntry
	idfCache        map[string]float64 // 词的 IDF 缓存
	avgDocLen        float64           // 平均文档长度（用于 BM25）
	totalDocs        int               // 总文档数
	mu              sync.RWMutex
}

// cacheEntry 缓存条目
type cacheEntry struct {
	context   string
	createdAt time.Time
}
```

**改动 3 — 实现 BM25 算法替换 calculateContextRelevance**

替换 `calculateContextRelevance`（第 234-254 行）：
```go
// calculateContextRelevance 使用 BM25 算法计算上下文相关性
func (r *ContextualRetrieval) calculateContextRelevance(query, context string) float64 {
	if context == "" || query == "" {
		return 0.0
	}

	queryTerms := tokenize(query)
	contextTerms := tokenize(context)

	if len(queryTerms) == 0 || len(contextTerms) == 0 {
		return 0.0
	}

	// 计算 context 中每个词的词频
	tf := make(map[string]int)
	for _, term := range contextTerms {
		tf[term]++
	}

	docLen := float64(len(contextTerms))
	k1 := r.config.BM25K1
	b := r.config.BM25B

	// 获取平均文档长度
	r.mu.RLock()
	avgDL := r.avgDocLen
	if avgDL == 0 {
		avgDL = 100.0 // 默认值
	}
	totalDocs := r.totalDocs
	if totalDocs == 0 {
		totalDocs = 1
	}
	r.mu.RUnlock()

	score := 0.0
	for _, term := range queryTerms {
		termFreq := float64(tf[term])
		if termFreq == 0 {
			continue
		}

		// IDF 计算：log((N - n + 0.5) / (n + 0.5) + 1)
		// 简化版：使用查询词在 context 中的出现作为近似
		idf := r.getIDF(term, totalDocs)

		// BM25 TF 归一化
		tfNorm := (termFreq * (k1 + 1)) / (termFreq + k1*(1-b+b*docLen/avgDL))

		score += idf * tfNorm
	}

	// 归一化到 [0, 1]
	maxScore := float64(len(queryTerms)) * math.Log(float64(totalDocs)+1)
	if maxScore > 0 {
		score = score / maxScore
	}
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// getIDF 获取词的 IDF 值
func (r *ContextualRetrieval) getIDF(term string, totalDocs int) float64 {
	r.mu.RLock()
	if idf, ok := r.idfCache[term]; ok {
		r.mu.RUnlock()
		return idf
	}
	r.mu.RUnlock()

	// 简化 IDF：假设每个词在约 10% 的文档中出现
	n := float64(totalDocs) * 0.1
	if n < 1 {
		n = 1
	}
	idf := math.Log((float64(totalDocs)-n+0.5)/(n+0.5) + 1)

	r.mu.Lock()
	if r.idfCache == nil {
		r.idfCache = make(map[string]float64)
	}
	r.idfCache[term] = idf
	r.mu.Unlock()

	return idf
}

// UpdateIDFStats 更新 IDF 统计信息（在索引文档后调用）
func (r *ContextualRetrieval) UpdateIDFStats(docs []Document) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.totalDocs += len(docs)

	// 计算平均文档长度
	totalLen := 0
	docFreq := make(map[string]int) // 词 -> 包含该词的文档数

	for _, doc := range docs {
		terms := tokenize(doc.Content)
		totalLen += len(terms)

		seen := make(map[string]bool)
		for _, term := range terms {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	if r.totalDocs > 0 {
		r.avgDocLen = float64(totalLen) / float64(len(docs))
	}

	// 更新 IDF 缓存
	if r.idfCache == nil {
		r.idfCache = make(map[string]float64)
	}
	for term, df := range docFreq {
		n := float64(df)
		r.idfCache[term] = math.Log((float64(r.totalDocs)-n+0.5)/(n+0.5) + 1)
	}
}

// tokenize 分词（支持中英文）
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// 按空格和标点分词
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r >= 0x4e00 && r <= 0x9fff)
	})

	// 过滤停用词
	result := make([]string, 0, len(words))
	for _, w := range words {
		if len(w) > 1 || (len([]rune(w)) == 1 && []rune(w)[0] >= 0x4e00) {
			result = append(result, w)
		}
	}
	return result
}
```

**改动 4 — 改造 chunkDocument 为可配置滑动窗口**

替换 `chunkDocument`（第 171-198 行）：
```go
// chunkDocument 使用滑动窗口分块文档
func (r *ContextualRetrieval) chunkDocument(doc Document) []string {
	content := doc.Content
	if content == "" {
		return nil
	}

	chunkSize := r.config.ChunkSize
	if chunkSize <= 0 {
		chunkSize = 500
	}
	overlap := r.config.ChunkOverlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize / 4 // overlap 不能超过 chunk 大小
	}

	// 先按段落分割，保持语义完整性
	paragraphs := strings.Split(content, "\n\n")

	chunks := make([]string, 0)
	currentChunk := ""

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 如果单个段落超过 chunkSize，按句子拆分
		if len(para) > chunkSize {
			subChunks := r.splitLongParagraph(para, chunkSize, overlap)
			for _, sc := range subChunks {
				if currentChunk != "" && len(currentChunk)+len(sc) > chunkSize {
					chunks = append(chunks, strings.TrimSpace(currentChunk))
					// 保留 overlap 部分
					currentChunk = r.getOverlapSuffix(currentChunk, overlap)
				}
				if currentChunk != "" {
					currentChunk += "\n\n"
				}
				currentChunk += sc
			}
			continue
		}

		if len(currentChunk)+len(para)+2 > chunkSize {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
				// 保留 overlap 部分
				currentChunk = r.getOverlapSuffix(currentChunk, overlap)
			}
		}

		if currentChunk != "" {
			currentChunk += "\n\n"
		}
		currentChunk += para
	}

	if strings.TrimSpace(currentChunk) != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

// splitLongParagraph 拆分超长段落
func (r *ContextualRetrieval) splitLongParagraph(para string, chunkSize, overlap int) []string {
	// 按句子分割
	sentences := splitSentences(para)
	chunks := make([]string, 0)
	current := ""

	for _, sent := range sentences {
		if len(current)+len(sent) > chunkSize && current != "" {
			chunks = append(chunks, strings.TrimSpace(current))
			current = r.getOverlapSuffix(current, overlap)
		}
		if current != "" {
			current += " "
		}
		current += sent
	}

	if strings.TrimSpace(current) != "" {
		chunks = append(chunks, strings.TrimSpace(current))
	}

	return chunks
}

// splitSentences 按句子分割
func splitSentences(text string) []string {
	// 简单的句子分割：按 。！？.!? 分割
	var sentences []string
	current := ""
	for _, r := range text {
		current += string(r)
		if r == '。' || r == '！' || r == '？' || r == '.' || r == '!' || r == '?' {
			if strings.TrimSpace(current) != "" {
				sentences = append(sentences, strings.TrimSpace(current))
			}
			current = ""
		}
	}
	if strings.TrimSpace(current) != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}
	return sentences
}

// getOverlapSuffix 获取文本末尾的 overlap 部分
func (r *ContextualRetrieval) getOverlapSuffix(text string, overlap int) string {
	if overlap <= 0 || len(text) <= overlap {
		return ""
	}
	return text[len(text)-overlap:]
}
```

**改动 5 — 实现 CacheContexts 缓存机制**

修改 `IndexDocumentsWithContext`（第 105-152 行），在生成上下文前检查缓存：
```go
func (r *ContextualRetrieval) IndexDocumentsWithContext(ctx context.Context, docs []Document) error {
	if !r.config.UseContextPrefix {
		return r.retriever.IndexDocuments(docs)
	}

	enrichedDocs := make([]Document, 0)

	for _, doc := range docs {
		chunks := r.chunkDocument(doc)

		for i, chunk := range chunks {
			var contextStr string
			var err error

			// 检查缓存
			if r.config.CacheContexts {
				cacheKey := r.buildCacheKey(doc.ID, chunk)
				if cached, ok := r.getFromCache(cacheKey); ok {
					contextStr = cached
					goto buildDoc
				}
			}

			// 生成上下文
			contextStr, err = r.contextProvider.GenerateContext(ctx, doc, chunk)
			if err != nil {
				r.logger.Warn("failed to generate context, using original chunk",
					zap.String("doc_id", doc.ID),
					zap.Int("chunk_idx", i),
					zap.Error(err))
				contextStr = ""
			}

			// 写入缓存
			if r.config.CacheContexts && contextStr != "" {
				cacheKey := r.buildCacheKey(doc.ID, chunk)
				r.putToCache(cacheKey, contextStr)
			}

		buildDoc:
			enrichedContent := r.renderContextTemplate(doc, chunk, contextStr)
			enrichedDoc := Document{
				ID:        fmt.Sprintf("%s_chunk_%d", doc.ID, i),
				Content:   enrichedContent,
				Embedding: nil,
				Metadata: map[string]interface{}{
					"original_doc_id": doc.ID,
					"chunk_index":     i,
					"context":         contextStr,
					"original_chunk":  chunk,
				},
			}
			enrichedDocs = append(enrichedDocs, enrichedDoc)
		}
	}

	// 更新 BM25 统计
	r.UpdateIDFStats(enrichedDocs)

	r.logger.Info("indexed documents with context",
		zap.Int("original_docs", len(docs)),
		zap.Int("enriched_chunks", len(enrichedDocs)))

	return r.retriever.IndexDocuments(enrichedDocs)
}

// buildCacheKey 构建缓存 key
func (r *ContextualRetrieval) buildCacheKey(docID, chunk string) string {
	h := sha256.Sum256([]byte(chunk))
	return docID + ":" + hex.EncodeToString(h[:8])
}

// getFromCache 从缓存获取上下文
func (r *ContextualRetrieval) getFromCache(key string) (string, bool) {
	val, ok := r.contextCache.Load(key)
	if !ok {
		return "", false
	}
	entry := val.(*cacheEntry)
	// 检查 TTL
	if r.config.CacheTTL > 0 && time.Since(entry.createdAt) > r.config.CacheTTL {
		r.contextCache.Delete(key)
		return "", false
	}
	return entry.context, true
}

// putToCache 写入缓存
func (r *ContextualRetrieval) putToCache(key, context string) {
	r.contextCache.Store(key, &cacheEntry{
		context:   context,
		createdAt: time.Now(),
	})
}

// CleanExpiredCache 清理过期缓存
func (r *ContextualRetrieval) CleanExpiredCache() int {
	cleaned := 0
	r.contextCache.Range(func(key, value interface{}) bool {
		entry := value.(*cacheEntry)
		if r.config.CacheTTL > 0 && time.Since(entry.createdAt) > r.config.CacheTTL {
			r.contextCache.Delete(key)
			cleaned++
		}
		return true
	})
	return cleaned
}
```

**改动 6 — 添加 Embedding 相似度计算支持**

在文件末尾添加：
```go
// EmbeddingSimilarity 计算两个 embedding 向量的余弦相似度
func EmbeddingSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// rerankWithEmbedding 使用 embedding 相似度重排序
func (r *ContextualRetrieval) rerankWithEmbedding(queryEmbedding []float64, results []RetrievalResult) []RetrievalResult {
	if len(queryEmbedding) == 0 {
		return results
	}

	for i := range results {
		if results[i].Document.Embedding != nil {
			embScore := EmbeddingSimilarity(queryEmbedding, results[i].Document.Embedding)
			// 混合 BM25 分数和 embedding 分数
			results[i].FinalScore = results[i].FinalScore*0.6 + embScore*0.4
		}
	}

	sortResultsByFinalScore(results)
	return results
}
```

### 验证方法
```bash
# 编译检查
cd D:/code/agentflow && go build ./rag/...

# 运行测试
go test ./rag/... -v -race

# 验证 BM25 计算：
# 1. query="machine learning" context="machine learning is a subset of AI" 应该得到高分
# 2. query="machine learning" context="the weather is nice today" 应该得到低分
# 3. 分块测试：1000 字符文档，chunkSize=500, overlap=50 应该产生 3 个 chunk
```

### 注意事项
- import 需要添加 `"math"`, `"sync"`, `"time"`, `"crypto/sha256"`, `"encoding/hex"`
- `goto buildDoc` 在 Go 中合法但需要确保变量声明在 goto 之前
- `sync.Map` 的 `Range` 不保证遍历期间的一致性，但对缓存清理场景可接受
- BM25 的 IDF 使用简化版本，生产环境应该在索引时精确计算
- `tokenize` 函数是简化版，中文分词建议后续集成 jieba 等分词库

---
## P1-3: DAG 工作流熔断器

### 需求背景
`workflow/dag_executor.go` 有 retry（第 240-304 行）和 timeout 机制，但没有熔断器模式。当某个节点高频失败时，retry 会持续消耗资源。需要实现 Circuit Breaker 模式：连续失败达到阈值后自动熔断，避免雪崩效应。

### 需要修改的文件

#### 新建文件：workflow/circuit_breaker.go

```go
package workflow

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CircuitState 熔断器状态
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // 正常状态，允许请求通过
	CircuitOpen                         // 熔断状态，拒绝所有请求
	CircuitHalfOpen                     // 半开状态，允许探测请求
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig 熔断器配置
type CircuitBreakerConfig struct {
	// FailureThreshold 连续失败次数阈值，达到后触发熔断
	FailureThreshold int `json:"failure_threshold"`
	// RecoveryTimeout 熔断后等待恢复的时间
	RecoveryTimeout time.Duration `json:"recovery_timeout"`
	// HalfOpenMaxProbes 半开状态允许的探测请求数
	HalfOpenMaxProbes int `json:"half_open_max_probes"`
	// SuccessThresholdInHalfOpen 半开状态下连续成功多少次后恢复
	SuccessThresholdInHalfOpen int `json:"success_threshold_in_half_open"`
}

// DefaultCircuitBreakerConfig 默认熔断器配置
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:           5,
		RecoveryTimeout:            30 * time.Second,
		HalfOpenMaxProbes:          3,
		SuccessThresholdInHalfOpen: 2,
	}
}

// CircuitBreakerEvent 熔断器状态变更事件
type CircuitBreakerEvent struct {
	NodeID    string       `json:"node_id"`
	OldState  CircuitState `json:"old_state"`
	NewState  CircuitState `json:"new_state"`
	Timestamp time.Time    `json:"timestamp"`
	Reason    string       `json:"reason"`
	Failures  int          `json:"failures"`
}

// CircuitBreakerEventHandler 事件处理器接口
type CircuitBreakerEventHandler interface {
	OnStateChange(event CircuitBreakerEvent)
}

// CircuitBreaker 熔断器实现
type CircuitBreaker struct {
	nodeID          string
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int       // 连续失败次数
	successes       int       // 半开状态下连续成功次数
	lastFailureTime time.Time // 最后一次失败时间
	probeCount      int       // 半开状态下已探测次数
	eventHandler    CircuitBreakerEventHandler
	logger          *zap.Logger
	mu              sync.RWMutex
}

// NewCircuitBreaker 创建熔断器
func NewCircuitBreaker(
	nodeID string,
	config CircuitBreakerConfig,
	eventHandler CircuitBreakerEventHandler,
	logger *zap.Logger,
) *CircuitBreaker {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &CircuitBreaker{
		nodeID:       nodeID,
		config:       config,
		state:        CircuitClosed,
		eventHandler: eventHandler,
		logger:       logger.With(zap.String("node_id", nodeID)),
	}
}

// AllowRequest 检查是否允许请求通过
func (cb *CircuitBreaker) AllowRequest() (bool, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, nil

	case CircuitOpen:
		// 检查是否到了恢复时间
		if time.Since(cb.lastFailureTime) >= cb.config.RecoveryTimeout {
			cb.transitionTo(CircuitHalfOpen, "recovery timeout elapsed")
			cb.probeCount = 0
			cb.successes = 0
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker open for node %s: %d consecutive failures, retry after %v",
			cb.nodeID, cb.failures, cb.config.RecoveryTimeout-time.Since(cb.lastFailureTime))

	case CircuitHalfOpen:
		if cb.probeCount < cb.config.HalfOpenMaxProbes {
			cb.probeCount++
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker half-open for node %s: max probes (%d) reached",
			cb.nodeID, cb.config.HalfOpenMaxProbes)

	default:
		return false, fmt.Errorf("unknown circuit breaker state: %d", cb.state)
	}
}

// RecordSuccess 记录成功
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		cb.failures = 0 // 重置失败计数

	case CircuitHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThresholdInHalfOpen {
			cb.failures = 0
			cb.successes = 0
			cb.transitionTo(CircuitClosed, fmt.Sprintf("%d consecutive successes in half-open", cb.successes))
		}
	}
}

// RecordFailure 记录失败
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.transitionTo(CircuitOpen, fmt.Sprintf("%d consecutive failures", cb.failures))
		}

	case CircuitHalfOpen:
		// 半开状态下任何失败都重新熔断
		cb.successes = 0
		cb.transitionTo(CircuitOpen, "failure in half-open state")
	}
}

// GetState 获取当前状态
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetFailures 获取当前失败次数
func (cb *CircuitBreaker) GetFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

// Reset 重置熔断器
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	oldState := cb.state
	cb.state = CircuitClosed
	cb.failures = 0
	cb.successes = 0
	cb.probeCount = 0
	if oldState != CircuitClosed {
		cb.emitEvent(oldState, CircuitClosed, "manual reset")
	}
}

// transitionTo 状态转换（必须在锁内调用）
func (cb *CircuitBreaker) transitionTo(newState CircuitState, reason string) {
	oldState := cb.state
	cb.state = newState

	cb.logger.Info("circuit breaker state change",
		zap.String("old_state", oldState.String()),
		zap.String("new_state", newState.String()),
		zap.String("reason", reason),
		zap.Int("failures", cb.failures))

	cb.emitEvent(oldState, newState, reason)
}

// emitEvent 发送事件（必须在锁内调用）
func (cb *CircuitBreaker) emitEvent(oldState, newState CircuitState, reason string) {
	if cb.eventHandler != nil {
		event := CircuitBreakerEvent{
			NodeID:    cb.nodeID,
			OldState:  oldState,
			NewState:  newState,
			Timestamp: time.Now(),
			Reason:    reason,
			Failures:  cb.failures,
		}
		// 异步发送避免死锁
		go cb.eventHandler.OnStateChange(event)
	}
}

// CircuitBreakerRegistry 熔断器注册表，管理所有节点的熔断器
type CircuitBreakerRegistry struct {
	breakers     map[string]*CircuitBreaker
	config       CircuitBreakerConfig
	eventHandler CircuitBreakerEventHandler
	logger       *zap.Logger
	mu           sync.RWMutex
}

// NewCircuitBreakerRegistry 创建熔断器注册表
func NewCircuitBreakerRegistry(
	config CircuitBreakerConfig,
	eventHandler CircuitBreakerEventHandler,
	logger *zap.Logger,
) *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers:     make(map[string]*CircuitBreaker),
		config:       config,
		eventHandler: eventHandler,
		logger:       logger,
	}
}

// GetOrCreate 获取或创建节点的熔断器
func (r *CircuitBreakerRegistry) GetOrCreate(nodeID string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[nodeID]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// 双重检查
	if cb, ok := r.breakers[nodeID]; ok {
		return cb
	}

	cb := NewCircuitBreaker(nodeID, r.config, r.eventHandler, r.logger)
	r.breakers[nodeID] = cb
	return cb
}

// GetAllStates 获取所有熔断器状态
func (r *CircuitBreakerRegistry) GetAllStates() map[string]CircuitState {
	r.mu.RLock()
	defer r.mu.RUnlock()

	states := make(map[string]CircuitState, len(r.breakers))
	for id, cb := range r.breakers {
		states[id] = cb.GetState()
	}
	return states
}

// ResetAll 重置所有熔断器
func (r *CircuitBreakerRegistry) ResetAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, cb := range r.breakers {
		cb.Reset()
	}
}
```

#### 修改文件：workflow/dag_executor.go

**改动 1 — DAGExecutor 结构体添加熔断器注册表**

修改 `DAGExecutor` 结构体（第 14-27 行）：
```go
type DAGExecutor struct {
	checkpointMgr    CheckpointManager
	historyStore     *ExecutionHistoryStore
	logger           *zap.Logger
	circuitBreakers  *CircuitBreakerRegistry // 新增

	// Execution state
	executionID  string
	threadID     string
	nodeResults  map[string]interface{}
	visitedNodes map[string]bool
	loopDepth    map[string]int
	history      *ExecutionHistory
	mu           sync.RWMutex
}
```

**改动 2 — 更新 NewDAGExecutor**

修改 `NewDAGExecutor`（第 38-49 行）：
```go
func NewDAGExecutor(checkpointMgr CheckpointManager, logger *zap.Logger) *DAGExecutor {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &DAGExecutor{
		checkpointMgr:   checkpointMgr,
		historyStore:    NewExecutionHistoryStore(),
		logger:          logger.With(zap.String("component", "dag_executor")),
		nodeResults:     make(map[string]interface{}),
		visitedNodes:    make(map[string]bool),
		circuitBreakers: NewCircuitBreakerRegistry(
			DefaultCircuitBreakerConfig(), nil, logger,
		),
	}
}
```

添加配置方法：
```go
// SetCircuitBreakerConfig 设置熔断器配置
func (e *DAGExecutor) SetCircuitBreakerConfig(config CircuitBreakerConfig, handler CircuitBreakerEventHandler) {
	e.circuitBreakers = NewCircuitBreakerRegistry(config, handler, e.logger)
}

// GetCircuitBreakerStates 获取所有熔断器状态
func (e *DAGExecutor) GetCircuitBreakerStates() map[string]CircuitState {
	return e.circuitBreakers.GetAllStates()
}
```

**改动 3 — 在 executeNode 中集成熔断器**

修改 `executeNode` 方法（第 125-202 行），在执行前检查熔断器，执行后记录结果：

在第 137 行（`e.visitedNodes[node.ID] = true` 之后，`e.mu.Unlock()` 之前）不变。

在第 148 行（`e.logger.Debug("executing node", ...)` 之后）添加熔断器检查：
```go
	// 检查熔断器
	cb := e.circuitBreakers.GetOrCreate(node.ID)
	allowed, cbErr := cb.AllowRequest()
	if !allowed {
		e.logger.Warn("node circuit breaker tripped",
			zap.String("node_id", node.ID),
			zap.String("state", cb.GetState().String()),
			zap.Error(cbErr))

		if nodeExec != nil {
			e.history.RecordNodeEnd(nodeExec, nil, cbErr)
		}

		// 如果有 fallback，使用 fallback
		if node.ErrorConfig != nil && node.ErrorConfig.FallbackValue != nil {
			return node.ErrorConfig.FallbackValue, nil
		}
		return nil, cbErr
	}
```

在第 184 行（错误处理 `if err != nil` 块的 `return nil, err` 之前）添加：
```go
			cb.RecordFailure()
```

在第 194 行（`e.nodeResults[node.ID] = result` 之前）添加：
```go
	// 记录成功
	cb.RecordSuccess()
```

完整的修改后 `executeNode` 方法：
```go
func (e *DAGExecutor) executeNode(ctx context.Context, graph *DAGGraph, node *DAGNode, input interface{}) (interface{}, error) {
	e.mu.Lock()
	if e.visitedNodes[node.ID] {
		result := e.nodeResults[node.ID]
		e.mu.Unlock()
		e.logger.Debug("node already visited, skipping", zap.String("node_id", node.ID))
		return result, nil
	}
	e.visitedNodes[node.ID] = true
	e.mu.Unlock()

	var nodeExec *NodeExecution
	if e.history != nil {
		nodeExec = e.history.RecordNodeStart(node.ID, node.Type, input)
	}

	e.logger.Debug("executing node",
		zap.String("node_id", node.ID),
		zap.String("node_type", string(node.Type)))

	// === 熔断器检查 ===
	cb := e.circuitBreakers.GetOrCreate(node.ID)
	allowed, cbErr := cb.AllowRequest()
	if !allowed {
		e.logger.Warn("node circuit breaker tripped",
			zap.String("node_id", node.ID),
			zap.String("state", cb.GetState().String()),
			zap.Error(cbErr))
		if nodeExec != nil {
			e.history.RecordNodeEnd(nodeExec, nil, cbErr)
		}
		if node.ErrorConfig != nil && node.ErrorConfig.FallbackValue != nil {
			return node.ErrorConfig.FallbackValue, nil
		}
		return nil, cbErr
	}

	startTime := time.Now()
	var result interface{}
	var err error

	switch node.Type {
	case NodeTypeAction:
		result, err = e.executeActionNode(ctx, graph, node, input)
	case NodeTypeCondition:
		result, err = e.executeConditionNode(ctx, graph, node, input)
	case NodeTypeLoop:
		result, err = e.executeLoopNode(ctx, graph, node, input)
	case NodeTypeParallel:
		result, err = e.executeParallelNode(ctx, graph, node, input)
	case NodeTypeSubGraph:
		result, err = e.executeSubGraphNode(ctx, node, input)
	case NodeTypeCheckpoint:
		result, err = e.executeCheckpointNode(ctx, node, input)
	default:
		err = fmt.Errorf("unknown node type: %s", node.Type)
	}

	duration := time.Since(startTime)

	if err != nil {
		result, err = e.handleNodeError(ctx, graph, node, input, err, duration)
		if err != nil {
			cb.RecordFailure() // === 记录失败 ===
			if nodeExec != nil {
				e.history.RecordNodeEnd(nodeExec, nil, err)
			}
			return nil, err
		}
	}

	// === 记录成功 ===
	cb.RecordSuccess()

	if nodeExec != nil {
		e.history.RecordNodeEnd(nodeExec, result, nil)
	}

	e.mu.Lock()
	e.nodeResults[node.ID] = result
	e.mu.Unlock()

	e.logger.Debug("node execution completed",
		zap.String("node_id", node.ID),
		zap.Duration("duration", duration))

	return result, nil
}
```

### 验证方法
```bash
# 编译检查
cd D:/code/agentflow && go build ./workflow/...

# 运行测试
go test ./workflow/... -v -race

# 熔断器单元测试验证：
# 1. 连续 5 次失败后状态变为 Open
# 2. Open 状态下请求被拒绝
# 3. 等待 RecoveryTimeout 后状态变为 HalfOpen
# 4. HalfOpen 下成功 2 次后恢复为 Closed
# 5. HalfOpen 下失败 1 次后重新变为 Open
```

### 注意事项
- 熔断器是按节点 ID 隔离的，不同节点互不影响
- `emitEvent` 使用 `go` 异步发送，避免在锁内阻塞
- 循环节点（LoopType）中的子节点会被反复执行，熔断器需要正确处理 `visitedNodes` 的重置
- 并行节点（ParallelType）中多个 goroutine 可能同时访问同一个熔断器，`CircuitBreaker` 内部已用 `sync.RWMutex` 保护
- `NewDAGExecutor` 默认创建熔断器注册表，不影响现有行为（默认阈值 5 次）

---
## P1-4: Browser Automation 实现

### 需求背景
`agent/browser/` 目录有两个文件：
- `browser.go`：定义了 `Browser` 接口（第 106-113 行）、`BrowserFactory` 接口（第 236-238 行）、`BrowserSession`、`BrowserTool` 等管理层，但没有 `Browser` 接口的实际实现
- `agentic_browser.go`：定义了 `BrowserDriver` 接口（第 62-70 行）和 `VisionModel` 接口（第 47-50 行），`AgenticBrowser` 的 Vision-Action Loop 已实现，但 `BrowserDriver` 和 `VisionModel` 没有具体实现

需要实现基于 chromedp 的 `BrowserDriver`，实现 `VisionModel` 适配器，以及浏览器池管理。

### 需要修改的文件

#### 新建文件：agent/browser/chromedp_driver.go

```go
package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"go.uber.org/zap"
)

// ChromeDPDriver 基于 chromedp 的 BrowserDriver 实现
type ChromeDPDriver struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	cancel      context.CancelFunc
	config      BrowserConfig
	logger      *zap.Logger
	mu          sync.Mutex
}

// ChromeDPDriverOption 配置选项
type ChromeDPDriverOption func(*ChromeDPDriver)

// NewChromeDPDriver 创建 chromedp 驱动
func NewChromeDPDriver(config BrowserConfig, logger *zap.Logger) (*ChromeDPDriver, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", config.Headless),
		chromedp.WindowSize(config.ViewportWidth, config.ViewportHeight),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
	)

	if config.UserAgent != "" {
		opts = append(opts, chromedp.UserAgent(config.UserAgent))
	}
	if config.ProxyURL != "" {
		opts = append(opts, chromedp.ProxyServer(config.ProxyURL))
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(func(format string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(format, args...))
		}),
	)

	// 设置超时
	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
	}

	driver := &ChromeDPDriver{
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		cancel:      cancel,
		config:      config,
		logger:      logger.With(zap.String("component", "chromedp_driver")),
	}

	// 启动浏览器
	if err := chromedp.Run(ctx); err != nil {
		allocCancel()
		cancel()
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	logger.Info("chromedp browser started",
		zap.Bool("headless", config.Headless),
		zap.Int("viewport_w", config.ViewportWidth),
		zap.Int("viewport_h", config.ViewportHeight))

	return driver, nil
}

// Navigate 导航到 URL
func (d *ChromeDPDriver) Navigate(ctx context.Context, url string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("navigating", zap.String("url", url))
	return chromedp.Run(d.ctx, chromedp.Navigate(url))
}

// Screenshot 截取页面截图
func (d *ChromeDPDriver) Screenshot(ctx context.Context) (*Screenshot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var buf []byte
	if err := chromedp.Run(d.ctx, chromedp.FullScreenshot(&buf, 90)); err != nil {
		return nil, fmt.Errorf("screenshot failed: %w", err)
	}

	var currentURL string
	if err := chromedp.Run(d.ctx, chromedp.Location(&currentURL)); err != nil {
		currentURL = "unknown"
	}

	return &Screenshot{
		Data:      buf,
		Width:     d.config.ViewportWidth,
		Height:    d.config.ViewportHeight,
		Timestamp: time.Now(),
		URL:       currentURL,
	}, nil
}

// Click 点击指定坐标
func (d *ChromeDPDriver) Click(ctx context.Context, x, y int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("clicking", zap.Int("x", x), zap.Int("y", y))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(
				input.MousePressed,
				float64(x), float64(y),
			).WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(
				input.MouseReleased,
				float64(x), float64(y),
			).WithButton(input.Left).WithClickCount(1).Do(ctx)
		}),
	)
}

// Type 输入文本
func (d *ChromeDPDriver) Type(ctx context.Context, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("typing", zap.String("text", text))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for _, ch := range text {
				if err := input.DispatchKeyEvent(input.KeyChar).
					WithText(string(ch)).Do(ctx); err != nil {
					return err
				}
			}
			return nil
		}),
	)
}

// Scroll 滚动页面
func (d *ChromeDPDriver) Scroll(ctx context.Context, deltaX, deltaY int) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Debug("scrolling", zap.Int("deltaX", deltaX), zap.Int("deltaY", deltaY))
	return chromedp.Run(d.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseWheel, 0, 0).
				WithDeltaX(float64(deltaX)).
				WithDeltaY(float64(deltaY)).Do(ctx)
		}),
	)
}

// GetURL 获取当前 URL
func (d *ChromeDPDriver) GetURL(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var url string
	if err := chromedp.Run(d.ctx, chromedp.Location(&url)); err != nil {
		return "", fmt.Errorf("failed to get URL: %w", err)
	}
	return url, nil
}

// Close 关闭浏览器
func (d *ChromeDPDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.logger.Info("closing chromedp browser")
	d.cancel()
	d.allocCancel()
	return nil
}

// --- 同时实现 Browser 接口（browser.go 中定义的） ---

// ChromeDPBrowser 实现 Browser 接口
type ChromeDPBrowser struct {
	driver *ChromeDPDriver
	config BrowserConfig
	logger *zap.Logger
}

// NewChromeDPBrowser 创建 ChromeDPBrowser
func NewChromeDPBrowser(config BrowserConfig, logger *zap.Logger) (*ChromeDPBrowser, error) {
	driver, err := NewChromeDPDriver(config, logger)
	if err != nil {
		return nil, err
	}
	return &ChromeDPBrowser{
		driver: driver,
		config: config,
		logger: logger,
	}, nil
}

// Execute 执行浏览器命令
func (b *ChromeDPBrowser) Execute(ctx context.Context, cmd BrowserCommand) (*BrowserResult, error) {
	start := time.Now()
	result := &BrowserResult{
		Action: cmd.Action,
	}

	var err error
	switch cmd.Action {
	case ActionNavigate:
		err = b.driver.Navigate(ctx, cmd.Value)
	case ActionClick:
		// 如果有 selector，先定位元素再点击
		if cmd.Selector != "" {
			err = b.clickBySelector(ctx, cmd.Selector)
		}
	case ActionType:
		if cmd.Selector != "" {
			err = b.typeBySelector(ctx, cmd.Selector, cmd.Value)
		} else {
			err = b.driver.Type(ctx, cmd.Value)
		}
	case ActionScreenshot:
		screenshot, sErr := b.driver.Screenshot(ctx)
		if sErr != nil {
			err = sErr
		} else {
			result.Screenshot = screenshot.Data
		}
	case ActionScroll:
		err = b.driver.Scroll(ctx, 0, 300) // 默认向下滚动
	case ActionWait:
		if cmd.Selector != "" {
			err = chromedp.Run(b.driver.ctx,
				chromedp.WaitVisible(cmd.Selector, chromedp.ByQuery))
		}
	case ActionExtract:
		var text string
		err = chromedp.Run(b.driver.ctx,
			chromedp.Text(cmd.Selector, &text, chromedp.ByQuery))
		if err == nil {
			result.Data = []byte(fmt.Sprintf(`{"text":%q}`, text))
		}
	case ActionBack:
		err = chromedp.Run(b.driver.ctx, chromedp.NavigateBack())
	case ActionForward:
		err = chromedp.Run(b.driver.ctx, chromedp.NavigateForward())
	case ActionRefresh:
		err = chromedp.Run(b.driver.ctx, chromedp.Reload())
	default:
		err = fmt.Errorf("unsupported action: %s", cmd.Action)
	}

	result.Duration = time.Since(start)
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		if b.config.ScreenshotOnError {
			if ss, ssErr := b.driver.Screenshot(ctx); ssErr == nil {
				result.Screenshot = ss.Data
			}
		}
		return result, err
	}

	result.Success = true
	// 获取当前 URL 和 Title
	if url, urlErr := b.driver.GetURL(ctx); urlErr == nil {
		result.URL = url
	}

	return result, nil
}

func (b *ChromeDPBrowser) clickBySelector(ctx context.Context, selector string) error {
	return chromedp.Run(b.driver.ctx, chromedp.Click(selector, chromedp.ByQuery))
}

func (b *ChromeDPBrowser) typeBySelector(ctx context.Context, selector, text string) error {
	return chromedp.Run(b.driver.ctx,
		chromedp.Clear(selector, chromedp.ByQuery),
		chromedp.SendKeys(selector, text, chromedp.ByQuery),
	)
}

// GetState 获取页面状态
func (b *ChromeDPBrowser) GetState(ctx context.Context) (*PageState, error) {
	state := &PageState{}

	// 获取 URL
	if url, err := b.driver.GetURL(ctx); err == nil {
		state.URL = url
	}

	// 获取 Title
	var title string
	if err := chromedp.Run(b.driver.ctx, chromedp.Title(&title)); err == nil {
		state.Title = title
	}

	// 获取页面文本内容
	var content string
	if err := chromedp.Run(b.driver.ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.GetDocument().Do(ctx)
			if err != nil {
				return err
			}
			content, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
			return err
		}),
	); err == nil {
		// 截断过长内容
		if len(content) > 10000 {
			content = content[:10000] + "..."
		}
		state.Content = content
	}

	return state, nil
}

// Close 关闭浏览器
func (b *ChromeDPBrowser) Close() error {
	return b.driver.Close()
}
```

#### 新建文件：agent/browser/browser_pool.go

```go
package browser

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// BrowserPool 浏览器实例池
type BrowserPool struct {
	config    BrowserConfig
	pool      chan *ChromeDPBrowser
	active    map[*ChromeDPBrowser]bool
	maxSize   int
	minIdle   int
	logger    *zap.Logger
	mu        sync.Mutex
	closed    bool
}

// BrowserPoolConfig 浏览器池配置
type BrowserPoolConfig struct {
	MaxSize       int           `json:"max_size"`        // 最大实例数
	MinIdle       int           `json:"min_idle"`        // 最小空闲数
	MaxIdleTime   time.Duration `json:"max_idle_time"`   // 最大空闲时间
	BrowserConfig BrowserConfig `json:"browser_config"`
}

// DefaultBrowserPoolConfig 默认池配置
func DefaultBrowserPoolConfig() BrowserPoolConfig {
	return BrowserPoolConfig{
		MaxSize:       5,
		MinIdle:       1,
		MaxIdleTime:   5 * time.Minute,
		BrowserConfig: DefaultBrowserConfig(),
	}
}

// NewBrowserPool 创建浏览器池
func NewBrowserPool(config BrowserPoolConfig, logger *zap.Logger) (*BrowserPool, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	pool := &BrowserPool{
		config:  config.BrowserConfig,
		pool:    make(chan *ChromeDPBrowser, config.MaxSize),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: config.MaxSize,
		minIdle: config.MinIdle,
		logger:  logger.With(zap.String("component", "browser_pool")),
	}

	// 预创建最小空闲实例
	for i := 0; i < config.MinIdle; i++ {
		browser, err := NewChromeDPBrowser(config.BrowserConfig, logger)
		if err != nil {
			pool.Close() // 清理已创建的
			return nil, fmt.Errorf("failed to pre-create browser %d: %w", i, err)
		}
		pool.pool <- browser
	}

	logger.Info("browser pool created",
		zap.Int("max_size", config.MaxSize),
		zap.Int("min_idle", config.MinIdle),
		zap.Int("pre_created", config.MinIdle))

	return pool, nil
}

// Acquire 获取一个浏览器实例
func (p *BrowserPool) Acquire(ctx context.Context) (*ChromeDPBrowser, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("browser pool is closed")
	}
	p.mu.Unlock()

	// 尝试从池中获取
	select {
	case browser := <-p.pool:
		p.mu.Lock()
		p.active[browser] = true
		p.mu.Unlock()
		p.logger.Debug("acquired browser from pool")
		return browser, nil
	default:
	}

	// 池为空，检查是否可以创建新实例
	p.mu.Lock()
	totalCount := len(p.active) + len(p.pool)
	if totalCount >= p.maxSize {
		p.mu.Unlock()
		// 等待可用实例
		p.logger.Debug("pool exhausted, waiting for available browser")
		select {
		case browser := <-p.pool:
			p.mu.Lock()
			p.active[browser] = true
			p.mu.Unlock()
			return browser, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	p.mu.Unlock()

	// 创建新实例
	browser, err := NewChromeDPBrowser(p.config, p.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create browser: %w", err)
	}

	p.mu.Lock()
	p.active[browser] = true
	p.mu.Unlock()

	p.logger.Debug("created new browser instance")
	return browser, nil
}

// Release 归还浏览器实例
func (p *BrowserPool) Release(browser *ChromeDPBrowser) {
	p.mu.Lock()
	delete(p.active, browser)

	if p.closed {
		p.mu.Unlock()
		_ = browser.Close()
		return
	}
	p.mu.Unlock()

	// 尝试放回池中
	select {
	case p.pool <- browser:
		p.logger.Debug("browser returned to pool")
	default:
		// 池满，关闭多余实例
		_ = browser.Close()
		p.logger.Debug("pool full, closing excess browser")
	}
}

// Close 关闭浏览器池
func (p *BrowserPool) Close() error {
	p.mu.Lock()
	p.closed = true
	// 关闭所有活跃实例
	for browser := range p.active {
		_ = browser.Close()
	}
	p.active = make(map[*ChromeDPBrowser]bool)
	p.mu.Unlock()

	// 关闭池中的实例
	close(p.pool)
	for browser := range p.pool {
		_ = browser.Close()
	}

	p.logger.Info("browser pool closed")
	return nil
}

// Stats 返回池统计信息
func (p *BrowserPool) Stats() (idle, active, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idle = len(p.pool)
	active = len(p.active)
	total = idle + active
	return
}

// ChromeDPBrowserFactory 实现 BrowserFactory 接口
type ChromeDPBrowserFactory struct {
	logger *zap.Logger
}

// NewChromeDPBrowserFactory 创建工厂
func NewChromeDPBrowserFactory(logger *zap.Logger) *ChromeDPBrowserFactory {
	return &ChromeDPBrowserFactory{logger: logger}
}

// Create 创建浏览器实例
func (f *ChromeDPBrowserFactory) Create(config BrowserConfig) (Browser, error) {
	return NewChromeDPBrowser(config, f.logger)
}
```

#### 新建文件：agent/browser/vision_adapter.go

```go
package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// LLMVisionProvider LLM 视觉能力提供者接口
type LLMVisionProvider interface {
	// AnalyzeImage 分析图片，返回 JSON 格式的分析结果
	AnalyzeImage(ctx context.Context, imageBase64 string, prompt string) (string, error)
}

// LLMVisionAdapter 将 LLM 视觉能力适配为 VisionModel 接口
type LLMVisionAdapter struct {
	provider LLMVisionProvider
	logger   *zap.Logger
}

// NewLLMVisionAdapter 创建视觉适配器
func NewLLMVisionAdapter(provider LLMVisionProvider, logger *zap.Logger) *LLMVisionAdapter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &LLMVisionAdapter{
		provider: provider,
		logger:   logger.With(zap.String("component", "vision_adapter")),
	}
}

// Analyze 分析截图
func (a *LLMVisionAdapter) Analyze(ctx context.Context, screenshot *Screenshot) (*VisionAnalysis, error) {
	if screenshot == nil || len(screenshot.Data) == 0 {
		return nil, fmt.Errorf("empty screenshot")
	}

	imageB64 := base64.StdEncoding.EncodeToString(screenshot.Data)

	prompt := `Analyze this webpage screenshot and return a JSON object with:
{
  "elements": [{"id": "elem_N", "type": "button|input|link|text|image", "text": "visible text", "x": 0, "y": 0, "width": 0, "height": 0, "clickable": true, "confidence": 0.9}],
  "page_title": "page title",
  "page_type": "login|search|form|article|dashboard|other",
  "description": "brief description of what's on the page",
  "suggestions": ["possible actions"]
}
Only return valid JSON, no markdown.`

	response, err := a.provider.AnalyzeImage(ctx, imageB64, prompt)
	if err != nil {
		return nil, fmt.Errorf("vision analysis failed: %w", err)
	}

	var analysis VisionAnalysis
	if err := json.Unmarshal([]byte(response), &analysis); err != nil {
		a.logger.Warn("failed to parse vision response as JSON, using raw",
			zap.Error(err))
		analysis = VisionAnalysis{
			Description: response,
			PageTitle:   screenshot.URL,
		}
	}

	a.logger.Debug("vision analysis complete",
		zap.Int("elements", len(analysis.Elements)),
		zap.String("page_type", analysis.PageType))

	return &analysis, nil
}

// PlanActions 规划下一步操作
func (a *LLMVisionAdapter) PlanActions(ctx context.Context, goal string, analysis *VisionAnalysis) ([]AgenticAction, error) {
	analysisJSON, _ := json.Marshal(analysis)

	prompt := fmt.Sprintf(`Given the goal: "%s"
And the current page analysis: %s

Plan the next browser action. Return a JSON array of actions:
[{"type": "click|type|scroll|navigate|wait", "selector": "css selector if applicable", "value": "text to type or url", "x": 0, "y": 0}]

Rules:
- Return at most 3 actions
- Prefer clicking interactive elements to achieve the goal
- If a form needs filling, use "type" action
- Only return valid JSON array, no markdown.`, goal, string(analysisJSON))

	response, err := a.provider.AnalyzeImage(ctx, "", prompt)
	if err != nil {
		return nil, fmt.Errorf("action planning failed: %w", err)
	}

	var actions []AgenticAction
	if err := json.Unmarshal([]byte(response), &actions); err != nil {
		a.logger.Warn("failed to parse action plan", zap.Error(err))
		return nil, fmt.Errorf("failed to parse action plan: %w", err)
	}

	return actions, nil
}
```

### 验证方法
```bash
# 编译检查（需要先 go get chromedp）
cd D:/code/agentflow && go get github.com/chromedp/chromedp
go build ./agent/browser/...

# 运行测试
go test ./agent/browser/... -v

# 集成测试（需要 Chrome 安装）：
# 1. 创建 ChromeDPDriver，导航到 https://example.com
# 2. 截图并验证返回数据非空
# 3. 获取 URL 验证为 https://example.com/
# 4. 关闭浏览器
# 5. 浏览器池：Acquire 5 个实例，验证第 6 个阻塞
```

### 注意事项
- `chromedp` 需要系统安装 Chrome/Chromium，CI 环境需要配置
- `go.mod` 需要添加 `github.com/chromedp/chromedp` 依赖
- `ChromeDPDriver` 的所有操作都加了 `sync.Mutex`，因为 chromedp 不是线程安全的
- `BrowserPool.Close()` 先关闭 active 实例再 close channel，避免 panic
- `LLMVisionAdapter` 依赖 LLM Provider 的多模态能力，需要确保 provider 支持图片输入
- 截图使用 `FullScreenshot` 质量 90，大页面可能产生较大图片，考虑压缩

---
## P1-5: LLM Provider 空壳补全

### 需求背景
`llm/providers/` 目录有 13 个 provider。经过代码审查，所有 provider 都已有完整实现：

| Provider | 包路径 | 实现方式 | 状态 |
|----------|--------|----------|------|
| OpenAI | `openai/provider.go` | 独立实现 | 完整 |
| Anthropic (Claude) | `anthropic/provider.go` | 独立实现（Claude 专有格式） | 完整 |
| Gemini | `gemini/provider.go` | 独立实现（Gemini 专有格式） | 完整 |
| DeepSeek | `deepseek/provider.go` | 独立实现（OpenAI 兼容） | 完整 |
| Qwen | `qwen/provider.go` | 独立实现（DashScope OpenAI 兼容） | 完整 |
| GLM | `glm/provider.go` | 独立实现（OpenAI 兼容） | 完整 |
| Doubao | `doubao/provider.go` | 独立实现（火山方舟 OpenAI 兼容） | 完整 |
| Grok | `grok/provider.go` | 独立实现（OpenAI 兼容） | 完整 |
| MiniMax | `minimax/provider.go` | 独立实现（XML tool calls 特殊处理） | 完整 |
| Mistral | `mistral/provider.go` | 嵌入 OpenAIProvider 委托 | 完整（薄包装） |
| Kimi | `kimi/provider.go` | 嵌入 OpenAIProvider 委托 | 完整（薄包装） |
| Llama | `llama/provider.go` | 嵌入 OpenAIProvider 委托 | 完整（薄包装） |
| Hunyuan | `hunyuan/provider.go` | 嵌入 OpenAIProvider 委托 | 完整（薄包装） |

虽然没有空壳 provider，但存在以下可优化问题：

1. Mistral/Kimi/Llama/Hunyuan 四个薄包装 provider 直接嵌入 `*openai.OpenAIProvider`，`Completion` 和 `Stream` 方法完全委托给 OpenAI，但 `Name()` 返回的是自己的名字，而内部 OpenAI provider 的错误信息中 `Provider` 字段会显示 "openai" 而非实际 provider 名
2. 所有独立实现的 provider 都有大量重复的 OpenAI 兼容类型定义（`openAIMessage`, `openAIToolCall` 等）
3. 缺少 provider 级别的统一重试和超时包装

### 需要修改的文件

#### 改动 1 — 修复薄包装 provider 的错误信息 Provider 字段

**文件：llm/providers/mistral/provider.go**

当前 Mistral 直接嵌入 OpenAIProvider，Completion/Stream 返回的错误中 Provider 字段是 "openai"。

添加 Completion 和 Stream 方法覆盖，修正 Provider 字段：
```go
// Completion 覆盖 OpenAI 的 Completion，修正 Provider 字段
func (p *MistralProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	resp, err := p.OpenAIProvider.Completion(ctx, req)
	if err != nil {
		// 修正错误中的 Provider 字段
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}
	resp.Provider = p.Name()
	return resp, nil
}

// Stream 覆盖 OpenAI 的 Stream，修正 Provider 字段
func (p *MistralProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	ch, err := p.OpenAIProvider.Stream(ctx, req)
	if err != nil {
		if llmErr, ok := err.(*llm.Error); ok {
			llmErr.Provider = p.Name()
			return nil, llmErr
		}
		return nil, err
	}

	// 包装 channel，修正每个 chunk 的 Provider
	wrappedCh := make(chan llm.StreamChunk)
	go func() {
		defer close(wrappedCh)
		for chunk := range ch {
			chunk.Provider = p.Name()
			if chunk.Err != nil {
				if llmErr, ok := chunk.Err.(*llm.Error); ok {
					llmErr.Provider = p.Name()
				}
			}
			wrappedCh <- chunk
		}
	}()
	return wrappedCh, nil
}
```

对 **kimi/provider.go**、**llama/provider.go**、**hunyuan/provider.go** 做同样的修改，只需替换 `p.Name()` 返回的名字。

**文件：llm/providers/kimi/provider.go** 添加同样的 Completion/Stream 覆盖。

**文件：llm/providers/llama/provider.go** 添加同样的 Completion/Stream 覆盖。

**文件：llm/providers/hunyuan/provider.go** 添加同样的 Completion/Stream 覆盖。

#### 改动 2 — 提取公共 OpenAI 兼容类型到 common.go

**文件：llm/providers/common.go**

当前 `common.go` 已存在，添加公共的 OpenAI 兼容类型定义，供所有 OpenAI 兼容 provider 复用：

```go
// === OpenAI 兼容 API 公共类型 ===
// 以下类型被 deepseek、qwen、glm、doubao、grok 等 OpenAI 兼容 provider 使用。
// 各 provider 包内目前各自定义了一份，后续重构时可统一引用此处。

// OpenAICompatMessage OpenAI 兼容消息格式
type OpenAICompatMessage struct {
	Role         string              `json:"role"`
	Content      string              `json:"content,omitempty"`
	Name         string              `json:"name,omitempty"`
	ToolCalls    []OpenAICompatToolCall `json:"tool_calls,omitempty"`
	ToolCallID   string              `json:"tool_call_id,omitempty"`
}

// OpenAICompatToolCall OpenAI 兼容工具调用
type OpenAICompatToolCall struct {
	ID       string              `json:"id"`
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatFunction OpenAI 兼容函数定义
type OpenAICompatFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// OpenAICompatTool OpenAI 兼容工具定义
type OpenAICompatTool struct {
	Type     string              `json:"type"`
	Function OpenAICompatFunction `json:"function"`
}

// OpenAICompatRequest OpenAI 兼容请求
type OpenAICompatRequest struct {
	Model            string                `json:"model"`
	Messages         []OpenAICompatMessage `json:"messages"`
	Tools            []OpenAICompatTool    `json:"tools,omitempty"`
	ToolChoice       interface{}           `json:"tool_choice,omitempty"`
	MaxTokens        int                   `json:"max_tokens,omitempty"`
	Temperature      float32               `json:"temperature,omitempty"`
	TopP             float32               `json:"top_p,omitempty"`
	Stop             []string              `json:"stop,omitempty"`
	Stream           bool                  `json:"stream,omitempty"`
}

// OpenAICompatChoice OpenAI 兼容选择
type OpenAICompatChoice struct {
	Index        int                  `json:"index"`
	FinishReason string               `json:"finish_reason"`
	Message      OpenAICompatMessage  `json:"message"`
	Delta        *OpenAICompatMessage `json:"delta,omitempty"`
}

// OpenAICompatUsage OpenAI 兼容用量
type OpenAICompatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// OpenAICompatResponse OpenAI 兼容响应
type OpenAICompatResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Choices []OpenAICompatChoice `json:"choices"`
	Usage   *OpenAICompatUsage   `json:"usage,omitempty"`
	Created int64                `json:"created,omitempty"`
}

// OpenAICompatErrorResp OpenAI 兼容错误响应
type OpenAICompatErrorResp struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
		Param   string `json:"param"`
	} `json:"error"`
}

// ConvertMessagesToOpenAI 将 llm.Message 转换为 OpenAI 兼容格式
func ConvertMessagesToOpenAI(msgs []llm.Message) []OpenAICompatMessage {
	out := make([]OpenAICompatMessage, 0, len(msgs))
	for _, m := range msgs {
		oa := OpenAICompatMessage{
			Role:       string(m.Role),
			Name:       m.Name,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			oa.ToolCalls = make([]OpenAICompatToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				oa.ToolCalls = append(oa.ToolCalls, OpenAICompatToolCall{
					ID:   tc.ID,
					Type: "function",
					Function: OpenAICompatFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}
		out = append(out, oa)
	}
	return out
}

// ConvertToolsToOpenAI 将 llm.ToolSchema 转换为 OpenAI 兼容格式
func ConvertToolsToOpenAI(tools []llm.ToolSchema) []OpenAICompatTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]OpenAICompatTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, OpenAICompatTool{
			Type: "function",
			Function: OpenAICompatFunction{
				Name:      t.Name,
				Arguments: t.Parameters,
			},
		})
	}
	return out
}

// MapHTTPError 将 HTTP 状态码映射为 llm.Error
func MapHTTPError(status int, msg string, provider string) *llm.Error {
	switch status {
	case http.StatusUnauthorized:
		return &llm.Error{Code: llm.ErrUnauthorized, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusForbidden:
		return &llm.Error{Code: llm.ErrForbidden, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusTooManyRequests:
		return &llm.Error{Code: llm.ErrRateLimited, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case http.StatusBadRequest:
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "quota") || strings.Contains(lower, "credit") {
			return &llm.Error{Code: llm.ErrQuotaExceeded, Message: msg, HTTPStatus: status, Provider: provider}
		}
		return &llm.Error{Code: llm.ErrInvalidRequest, Message: msg, HTTPStatus: status, Provider: provider}
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	case 529:
		return &llm.Error{Code: llm.ErrModelOverloaded, Message: msg, HTTPStatus: status, Retryable: true, Provider: provider}
	default:
		return &llm.Error{Code: llm.ErrUpstreamError, Message: msg, HTTPStatus: status, Retryable: status >= 500, Provider: provider}
	}
}

// ReadErrorMessage 从 HTTP 响应体读取错误信息
func ReadErrorMessage(body io.Reader) string {
	data, _ := io.ReadAll(body)
	var errResp OpenAICompatErrorResp
	if err := json.Unmarshal(data, &errResp); err == nil && errResp.Error.Message != "" {
		return errResp.Error.Message
	}
	return string(data)
}

// ToLLMChatResponse 将 OpenAI 兼容响应转换为 llm.ChatResponse
func ToLLMChatResponse(oa OpenAICompatResponse, provider string) *llm.ChatResponse {
	choices := make([]llm.ChatChoice, 0, len(oa.Choices))
	for _, c := range oa.Choices {
		msg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: c.Message.Content,
			Name:    c.Message.Name,
		}
		if len(c.Message.ToolCalls) > 0 {
			msg.ToolCalls = make([]llm.ToolCall, 0, len(c.Message.ToolCalls))
			for _, tc := range c.Message.ToolCalls {
				msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				})
			}
		}
		choices = append(choices, llm.ChatChoice{
			Index:        c.Index,
			FinishReason: c.FinishReason,
			Message:      msg,
		})
	}
	resp := &llm.ChatResponse{
		ID:       oa.ID,
		Provider: provider,
		Model:    oa.Model,
		Choices:  choices,
	}
	if oa.Usage != nil {
		resp.Usage = llm.ChatUsage{
			PromptTokens:     oa.Usage.PromptTokens,
			CompletionTokens: oa.Usage.CompletionTokens,
			TotalTokens:      oa.Usage.TotalTokens,
		}
	}
	if oa.Created != 0 {
		resp.CreatedAt = time.Unix(oa.Created, 0)
	}
	return resp
}
```

注意：`common.go` 中需要添加 `"encoding/json"`, `"io"`, `"net/http"`, `"strings"`, `"time"` 到 import。

#### 改动 3 — 添加 Provider 级别的重试包装

**新建文件：llm/providers/retry_wrapper.go**

```go
package providers

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"go.uber.org/zap"
)

// RetryConfig 重试配置
type RetryConfig struct {
	MaxRetries     int           `json:"max_retries"`      // 最大重试次数，默认 3
	InitialDelay   time.Duration `json:"initial_delay"`    // 初始延迟，默认 1s
	MaxDelay       time.Duration `json:"max_delay"`        // 最大延迟，默认 30s
	BackoffFactor  float64       `json:"backoff_factor"`   // 退避因子，默认 2.0
	RetryableOnly  bool          `json:"retryable_only"`   // 只重试标记为 Retryable 的错误
}

// DefaultRetryConfig 默认重试配置
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:    3,
		InitialDelay:  time.Second,
		MaxDelay:      30 * time.Second,
		BackoffFactor: 2.0,
		RetryableOnly: true,
	}
}

// RetryableProvider 带重试的 Provider 包装
type RetryableProvider struct {
	inner  llm.Provider
	config RetryConfig
	logger *zap.Logger
}

// NewRetryableProvider 创建带重试的 Provider
func NewRetryableProvider(inner llm.Provider, config RetryConfig, logger *zap.Logger) *RetryableProvider {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RetryableProvider{
		inner:  inner,
		config: config,
		logger: logger.With(zap.String("component", "retry_provider"), zap.String("provider", inner.Name())),
	}
}

func (p *RetryableProvider) Name() string                          { return p.inner.Name() }
func (p *RetryableProvider) SupportsNativeFunctionCalling() bool   { return p.inner.SupportsNativeFunctionCalling() }
func (p *RetryableProvider) HealthCheck(ctx context.Context) (*llm.HealthStatus, error) {
	return p.inner.HealthCheck(ctx)
}

// Completion 带重试的 Completion
func (p *RetryableProvider) Completion(ctx context.Context, req *llm.ChatRequest) (*llm.ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)
			p.logger.Debug("retrying completion",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := p.inner.Completion(ctx, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 检查是否可重试
		if p.config.RetryableOnly {
			if llmErr, ok := err.(*llm.Error); ok && !llmErr.Retryable {
				return nil, err // 不可重试的错误直接返回
			}
		}

		p.logger.Warn("completion failed, will retry",
			zap.Int("attempt", attempt),
			zap.Error(err))
	}

	return nil, fmt.Errorf("completion failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

// Stream 带重试的 Stream（只重试建立连接阶段）
func (p *RetryableProvider) Stream(ctx context.Context, req *llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	var lastErr error
	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := p.calculateDelay(attempt)
			p.logger.Debug("retrying stream",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		ch, err := p.inner.Stream(ctx, req)
		if err == nil {
			return ch, nil
		}

		lastErr = err

		if p.config.RetryableOnly {
			if llmErr, ok := err.(*llm.Error); ok && !llmErr.Retryable {
				return nil, err
			}
		}

		p.logger.Warn("stream connection failed, will retry",
			zap.Int("attempt", attempt),
			zap.Error(err))
	}

	return nil, fmt.Errorf("stream failed after %d retries: %w", p.config.MaxRetries, lastErr)
}

func (p *RetryableProvider) calculateDelay(attempt int) time.Duration {
	delay := float64(p.config.InitialDelay) * math.Pow(p.config.BackoffFactor, float64(attempt-1))
	if delay > float64(p.config.MaxDelay) {
		delay = float64(p.config.MaxDelay)
	}
	return time.Duration(delay)
}
```

### 验证方法
```bash
# 编译检查
cd D:/code/agentflow && go build ./llm/providers/...

# 运行所有 provider 测试
go test ./llm/providers/... -v -race

# 验证 Provider 名称修正：
# 1. 创建 MistralProvider，调用 Completion 触发错误
# 2. 检查返回的 llm.Error.Provider 字段是否为 "mistral" 而非 "openai"

# 验证重试包装：
# 1. 创建 RetryableProvider 包装一个 mock provider
# 2. mock 前 2 次返回 Retryable 错误，第 3 次成功
# 3. 验证最终成功返回
# 4. mock 返回非 Retryable 错误，验证不重试直接返回
```

### 注意事项
- 改动 2（公共类型提取）是渐进式重构，不要求各 provider 立即切换到公共类型，避免大规模改动
- 各 provider 包内的 `openAIMessage` 等类型暂时保留，后续逐步替换为 `providers.OpenAICompatMessage`
- `RetryableProvider` 的 `Stream` 方法只重试连接建立阶段，流式传输中的错误不重试
- `common.go` 中的 `MapHTTPError` 和各 provider 的 `mapError` 逻辑一致，后续可统一
- 薄包装 provider 的 Completion/Stream 覆盖会引入一个额外的 goroutine（Stream 的 channel 包装），性能影响可忽略

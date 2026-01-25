# Technology Stack

## Language & Runtime

- **Go**: 1.24+ (latest stable)
- **Module**: `github.com/yourusername/agentflow`

## Core Dependencies

### LLM & AI
- OpenAI API integration
- Claude API integration
- Gemini API integration (in development)

### Infrastructure
- **Redis**: Short-term memory and caching
- **PostgreSQL**: Metadata storage (via GORM)
- **Qdrant/Pinecone**: Vector storage for embeddings
- **InfluxDB**: Time-series metrics
- **Neo4j**: Knowledge graph (optional)

### Observability
- **Prometheus**: Metrics collection (`prometheus/client_golang`)
- **OpenTelemetry**: Distributed tracing (`go.opentelemetry.io/otel`)
- **Zap**: Structured logging (`go.uber.org/zap`)

### Testing
- **testify**: Assertions and mocking (`stretchr/testify`)
- Table-driven tests (Go standard pattern)

## 常用命令

```bash
go build ./...           # 构建
go test ./...            # 测试
go test -cover ./...     # 覆盖率
go fmt ./...             # 格式化
go mod tidy              # 整理依赖
```

## Performance Considerations

- Use `sync.Pool` for object reuse
- Goroutine management with context cancellation
- Efficient memory allocation patterns
- Caching with TTL and LRU eviction

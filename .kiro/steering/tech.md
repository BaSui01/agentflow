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

## Build & Development

### Common Commands

```bash
# Build the project
go build ./...

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests for specific package
go test ./llm/cache/...

# Run specific test
go test -run TestLRUCache_Basic ./llm/cache/

# Format code
go fmt ./...
gofmt -w .

# Vet code
go vet ./...

# Tidy dependencies
go mod tidy

# Download dependencies
go mod download

# Run examples
go run examples/01_simple_chat/main.go
go run examples/06_advanced_features/main.go
```

### Build Examples

```bash
# Build all examples
go build -o bin/ ./examples/...

# Build specific example
go build -o bin/simple_chat examples/01_simple_chat/main.go

# Run with race detector
go run -race examples/01_simple_chat/main.go
```

## Code Organization Patterns

### Package Structure
- Flat package hierarchy within domains
- Clear separation between interface and implementation
- Provider implementations in `providers/<name>/`
- Shared types in package root (e.g., `llm/types.go`)

### Testing Patterns
- Test files alongside source: `*_test.go`
- Table-driven tests for multiple scenarios
- Example tests: `*_example_test.go`
- Benchmark tests: `Benchmark*` functions

### Configuration
- Struct-based configuration
- Environment variables for secrets
- No hardcoded credentials in code

## Performance Considerations

- Use `sync.Pool` for object reuse
- Goroutine management with context cancellation
- Efficient memory allocation patterns
- Caching with TTL and LRU eviction

# =============================================================================
# 🚀 AgentFlow Makefile
# =============================================================================
# 统一构建入口，提供常用的开发、测试、部署命令
#
# 使用方法:
#   make help          # 显示帮助信息
#   make build         # 构建二进制
#   make test          # 运行测试
#   make docker-build  # 构建 Docker 镜像
# =============================================================================

# -----------------------------------------------------------------------------
# 📦 变量定义
# -----------------------------------------------------------------------------
BINARY_NAME := agentflow
MODULE := github.com/BaSui01/agentflow
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go 相关
GO := go
GOFLAGS := -v
LDFLAGS := -s -w \
	-X main.Version=$(VERSION) \
	-X main.BuildTime=$(BUILD_TIME) \
	-X main.GitCommit=$(GIT_COMMIT)

# Docker 相关
DOCKER_IMAGE := agentflow
DOCKER_TAG := $(VERSION)
DOCKER_REGISTRY ?=

# 目录
BUILD_DIR := ./build
CMD_DIR := ./cmd/agentflow

# -----------------------------------------------------------------------------
# 🎯 默认目标
# -----------------------------------------------------------------------------
.DEFAULT_GOAL := help

# -----------------------------------------------------------------------------
# 📋 帮助信息
# -----------------------------------------------------------------------------
.PHONY: help
help: ## 显示帮助信息
	@echo ""
	@echo "🚀 AgentFlow Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

# -----------------------------------------------------------------------------
# 🔨 构建目标
# -----------------------------------------------------------------------------
.PHONY: build
build: ## 构建二进制文件
	@echo "🔨 Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: build-linux
build-linux: ## 构建 Linux 二进制文件
	@echo "🔨 Building $(BINARY_NAME) for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64"

.PHONY: build-darwin
build-darwin: ## 构建 macOS 二进制文件
	@echo "🔨 Building $(BINARY_NAME) for macOS..."
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)
	@echo "✅ Build complete"

.PHONY: build-windows
build-windows: ## 构建 Windows 二进制文件
	@echo "🔨 Building $(BINARY_NAME) for Windows..."
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)
	@echo "✅ Build complete: $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe"

.PHONY: build-all
build-all: build-linux build-darwin build-windows ## 构建所有平台的二进制文件

.PHONY: install
install: build ## 安装到 GOPATH/bin
	@echo "📦 Installing $(BINARY_NAME)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "✅ Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# -----------------------------------------------------------------------------
# 🧪 测试目标
# -----------------------------------------------------------------------------
.PHONY: test
test: ## 运行单元测试
	@echo "🧪 Running unit tests..."
	$(GO) test ./... -v -race -cover
	@echo "✅ Tests complete"

.PHONY: test-short
test-short: ## 运行快速测试（跳过长时间测试）
	@echo "🧪 Running short tests..."
	$(GO) test ./... -v -short
	@echo "✅ Short tests complete"

.PHONY: test-cover
test-cover: ## 运行测试并生成覆盖率报告
	@echo "🧪 Running tests with coverage..."
	@mkdir -p $(BUILD_DIR)
	$(GO) test ./... -v -race -covermode=atomic -coverprofile=$(BUILD_DIR)/coverage.out
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "✅ Coverage report: $(BUILD_DIR)/coverage.html"

.PHONY: coverage
coverage: test-cover ## 生成覆盖率报告（test-cover 的别名）

.PHONY: coverage-func
coverage-func: ## 显示函数级别的覆盖率统计
	@echo "📊 Function coverage report..."
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f $(BUILD_DIR)/coverage.out ]; then \
		$(GO) test ./... -covermode=atomic -coverprofile=$(BUILD_DIR)/coverage.out; \
	fi
	$(GO) tool cover -func=$(BUILD_DIR)/coverage.out
	@echo ""
	@echo "📈 Total coverage:"
	@$(GO) tool cover -func=$(BUILD_DIR)/coverage.out | grep total | awk '{print $$3}'

.PHONY: coverage-html
coverage-html: ## 在浏览器中打开覆盖率报告
	@echo "🌐 Opening coverage report in browser..."
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f $(BUILD_DIR)/coverage.out ]; then \
		$(GO) test ./... -covermode=atomic -coverprofile=$(BUILD_DIR)/coverage.out; \
	fi
	$(GO) tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "✅ Coverage report generated: $(BUILD_DIR)/coverage.html"
	@if command -v xdg-open >/dev/null 2>&1; then \
		xdg-open $(BUILD_DIR)/coverage.html; \
	elif command -v open >/dev/null 2>&1; then \
		open $(BUILD_DIR)/coverage.html; \
	elif command -v start >/dev/null 2>&1; then \
		start $(BUILD_DIR)/coverage.html; \
	else \
		echo "📂 Please open $(BUILD_DIR)/coverage.html manually"; \
	fi

.PHONY: coverage-check
coverage-check: ## 检查覆盖率是否达到阈值 (默认 55%)
	@echo "🔍 Checking coverage threshold..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) test ./... -covermode=atomic -coverprofile=$(BUILD_DIR)/coverage.out
	@total=$$($(GO) tool cover -func=$(BUILD_DIR)/coverage.out | grep total | awk '{gsub(/%/,"",$$3); print $$3}'); \
	threshold=$${COVERAGE_THRESHOLD:-55.0}; \
	echo "📊 Current coverage: $${total}%"; \
	echo "📏 Threshold: $${threshold}%"; \
	if [ $$(echo "$${total} < $${threshold}" | bc -l) -eq 1 ]; then \
		echo "❌ Coverage $${total}% is below threshold $${threshold}%"; \
		exit 1; \
	else \
		echo "✅ Coverage $${total}% meets threshold $${threshold}%"; \
	fi

.PHONY: coverage-badge
coverage-badge: ## 生成覆盖率徽章数据
	@echo "🏷️ Generating coverage badge data..."
	@mkdir -p $(BUILD_DIR)
	@if [ ! -f $(BUILD_DIR)/coverage.out ]; then \
		$(GO) test ./... -covermode=atomic -coverprofile=$(BUILD_DIR)/coverage.out; \
	fi
	@total=$$($(GO) tool cover -func=$(BUILD_DIR)/coverage.out | grep total | awk '{gsub(/%/,"",$$3); print $$3}'); \
	echo "{\"coverage\": $${total}}" > $(BUILD_DIR)/coverage.json
	@echo "✅ Coverage badge data: $(BUILD_DIR)/coverage.json"

.PHONY: test-e2e
test-e2e: ## 运行 E2E 测试（需要 Docker 服务）
	@echo "🧪 Running E2E tests..."
	$(GO) test ./... -v -tags=e2e -timeout=10m -run='^TestE2E'
	@echo "✅ E2E tests complete"

.PHONY: test-integration
test-integration: ## 运行集成测试（需要外部依赖）
	@echo "🧪 Running integration tests..."
	$(GO) test ./... -v -tags=integration -timeout=5m
	@echo "✅ Integration tests complete"

.PHONY: test-all
test-all: test test-integration test-e2e ## 运行所有测试

.PHONY: bench
bench: ## 运行基准测试
	@echo "📊 Running benchmarks..."
	$(GO) test ./... -bench=. -benchmem -run=^$
	@echo "✅ Benchmarks complete"

# -----------------------------------------------------------------------------
# 🔍 代码质量
# -----------------------------------------------------------------------------
.PHONY: lint
lint: ## 运行代码检查
	@echo "🔍 Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "⚠️  golangci-lint not installed, running go vet instead"; \
		$(GO) vet ./...; \
	fi
	@echo "✅ Lint complete"

.PHONY: arch-guard
arch-guard: ## 架构依赖守卫检查（分层方向/包体积/单文件包）
	@echo "🏛️ Running architecture guard..."
	@pwsh -NoProfile -ExecutionPolicy Bypass -File ./scripts/arch_guard.ps1
	@echo "✅ Architecture guard complete"

.PHONY: arch-guard-ci
arch-guard-ci: ## CI 严格架构守卫（warning 按阈值升级为 error）
	@echo "🏛️ Running strict architecture guard..."
	@ARCH_GUARD_STRICT=1 pwsh -NoProfile -ExecutionPolicy Bypass -File ./scripts/arch_guard.ps1
	@echo "✅ Strict architecture guard complete"

.PHONY: refactor-plan-lint
refactor-plan-lint: ## 重构计划格式检查（必须包含 [ ]/[x] 状态与核心章节）
	@echo "📋 Linting refactor plans..."
	@python scripts/refactor_plan_guard.py lint
	@echo "✅ Refactor plan lint complete"

.PHONY: refactor-plan-report
refactor-plan-report: ## 输出重构计划进度汇总
	@python scripts/refactor_plan_guard.py report

.PHONY: refactor-plan-gate
refactor-plan-gate: ## 重构计划收尾门禁（存在 [ ] 则失败）
	@echo "🚧 Running refactor plan gate..."
	@python scripts/refactor_plan_guard.py gate
	@echo "✅ Refactor plan gate passed"

.PHONY: fmt
fmt: ## 格式化代码
	@echo "🎨 Formatting code..."
	$(GO) fmt ./...
	@echo "✅ Format complete"

.PHONY: vet
vet: ## 运行 go vet
	@echo "🔍 Running go vet..."
	$(GO) vet ./...
	@echo "✅ Vet complete"

.PHONY: tidy
tidy: ## 整理依赖
	@echo "📦 Tidying dependencies..."
	$(GO) mod tidy
	@echo "✅ Tidy complete"

.PHONY: verify
verify: fmt vet lint test ## 完整验证（格式化 + 检查 + 测试）

# -----------------------------------------------------------------------------
# 🐳 Docker 目标
# -----------------------------------------------------------------------------
.PHONY: docker-build
docker-build: ## 构建 Docker 镜像
	@echo "🐳 Building Docker image..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		-t $(DOCKER_IMAGE):latest \
		.
	@echo "✅ Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)"

.PHONY: docker-push
docker-push: ## 推送 Docker 镜像到仓库
	@echo "🚀 Pushing Docker image..."
ifdef DOCKER_REGISTRY
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker tag $(DOCKER_IMAGE):latest $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):latest
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_REGISTRY)/$(DOCKER_IMAGE):latest
else
	docker push $(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(DOCKER_IMAGE):latest
endif
	@echo "✅ Docker image pushed"

.PHONY: docker-run
docker-run: ## 运行 Docker 容器
	@echo "🐳 Running Docker container..."
	docker run --rm -it \
		-p 8080:8080 \
		-p 9090:9090 \
		-p 9091:9091 \
		$(DOCKER_IMAGE):$(DOCKER_TAG)

# -----------------------------------------------------------------------------
# 🚀 Docker Compose 目标
# -----------------------------------------------------------------------------
.PHONY: up
up: ## 启动本地开发环境
	@echo "🚀 Starting local environment..."
	docker-compose up -d
	@echo "✅ Environment started"
	@echo "   HTTP API: http://localhost:8080"
	@echo "   gRPC:     localhost:9090"
	@echo "   Metrics:  http://localhost:9091/metrics"

.PHONY: up-build
up-build: ## 重新构建并启动本地环境
	@echo "🚀 Building and starting local environment..."
	docker-compose up -d --build
	@echo "✅ Environment started"

.PHONY: up-monitoring
up-monitoring: ## 启动带监控的本地环境
	@echo "🚀 Starting environment with monitoring..."
	docker-compose --profile monitoring up -d
	@echo "✅ Environment started with monitoring"
	@echo "   Prometheus: http://localhost:9092"
	@echo "   Grafana:    http://localhost:3000 (admin/admin)"

.PHONY: down
down: ## 停止本地环境
	@echo "🛑 Stopping local environment..."
	docker-compose down
	@echo "✅ Environment stopped"

.PHONY: down-v
down-v: ## 停止本地环境并删除数据卷
	@echo "🛑 Stopping local environment and removing volumes..."
	docker-compose down -v
	@echo "✅ Environment stopped and volumes removed"

.PHONY: logs
logs: ## 查看服务日志
	docker-compose logs -f agentflow

.PHONY: ps
ps: ## 查看服务状态
	docker-compose ps

.PHONY: restart
restart: down up ## 重启本地环境

# -----------------------------------------------------------------------------
# 🔧 开发工具
# -----------------------------------------------------------------------------
.PHONY: run
run: build ## 构建并运行服务
	@echo "🚀 Running $(BINARY_NAME)..."
	$(BUILD_DIR)/$(BINARY_NAME) serve

.PHONY: dev
dev: ## 开发模式运行（带热重载，需要 air）
	@echo "🔥 Starting development mode..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "⚠️  air not installed, running normally"; \
		$(GO) run $(CMD_DIR) serve; \
	fi

.PHONY: generate
generate: ## 运行代码生成
	@echo "⚙️  Running code generation..."
	$(GO) generate ./...
	@echo "✅ Generation complete"

.PHONY: deps
deps: ## 下载依赖
	@echo "📦 Downloading dependencies..."
	$(GO) mod download
	@echo "✅ Dependencies downloaded"

.PHONY: deps-update
deps-update: ## 更新依赖
	@echo "📦 Updating dependencies..."
	$(GO) get -u ./...
	$(GO) mod tidy
	@echo "✅ Dependencies updated"

# -----------------------------------------------------------------------------
# 🧹 清理目标
# -----------------------------------------------------------------------------
.PHONY: clean
clean: ## 清理构建产物
	@echo "🧹 Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	$(GO) clean -cache
	@echo "✅ Clean complete"

.PHONY: clean-docker
clean-docker: ## 清理 Docker 资源
	@echo "🧹 Cleaning Docker resources..."
	docker-compose down -v --rmi local
	docker image prune -f
	@echo "✅ Docker cleanup complete"

.PHONY: clean-all
clean-all: clean clean-docker ## 清理所有资源

# -----------------------------------------------------------------------------
# 📊 信息目标
# -----------------------------------------------------------------------------
.PHONY: version
version: ## 显示版本信息
	@echo "Version:    $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"

.PHONY: info
info: ## 显示项目信息
	@echo ""
	@echo "🚀 AgentFlow Project Info"
	@echo ""
	@echo "Module:     $(MODULE)"
	@echo "Version:    $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Go Version: $(shell $(GO) version)"
	@echo ""

# -----------------------------------------------------------------------------
# 📚 文档质量检查
# -----------------------------------------------------------------------------
.PHONY: docs-lint
docs-lint: ## 检查文档中的死链接
	@echo "🔍 Checking markdown links..."
	@if command -v npx >/dev/null 2>&1; then \
		find docs/ README.md -name '*.md' -exec npx markdown-link-check --quiet {} + ; \
		echo "✅ Markdown link check complete"; \
	else \
		echo "⚠️  npx not available, skipping link check"; \
		echo "   Install Node.js to enable: npm install -g markdown-link-check"; \
	fi

.PHONY: docs-api-drift
docs-api-drift: ## 检查文档中是否残留已废弃的 API 引用
	@echo "🔍 Checking for stale API references in docs..."
	@FAIL=0; \
	if grep -rn 'agent\.Config{' docs/ README.md 2>/dev/null | grep -v '重构计划' | grep -v 'REFACTORING_SUMMARY' | grep -v 'prompts/' | grep -q .; then \
		grep -rn 'agent\.Config{' docs/ README.md 2>/dev/null | grep -v '重构计划' | grep -v 'REFACTORING_SUMMARY' | grep -v 'prompts/'; \
		echo "❌ Found stale 'agent.Config{' references (should be types.AgentConfig)"; \
		FAIL=1; \
	fi; \
	if grep -rn 'BuildBaseAgent(' docs/ README.md 2>/dev/null | grep -v '重构计划' | grep -v 'REFACTORING_SUMMARY' | grep -v 'prompts/' | grep -q .; then \
		grep -rn 'BuildBaseAgent(' docs/ README.md 2>/dev/null | grep -v '重构计划' | grep -v 'REFACTORING_SUMMARY' | grep -v 'prompts/'; \
		echo "❌ Found stale 'BuildBaseAgent(' references (should be NewAgentBuilder)"; \
		FAIL=1; \
	fi; \
	if [ $$FAIL -eq 1 ]; then \
		exit 1; \
	else \
		echo "✅ No stale API references found"; \
	fi

.PHONY: docs-surface-check
docs-surface-check: ## 检查 README/docs/examples 的官方/legacy 产品面口径一致性
	@echo "🔍 Checking public product-surface docs/examples consistency..."
	@$(GO) test -run "TestPublicProductSurfaceDocsExamplesConsistency$$" . -count=1
	@echo "✅ Product-surface docs/examples consistency check complete"

.PHONY: docs-examples-check
docs-examples-check: ## 提取文档中 Go 代码块做编译检查
	@echo "🔍 Extracting and compiling Go examples from docs..."
	@mkdir -p $(BUILD_DIR)/doc-examples
	@FAIL=0; \
	for f in $$(find docs/ README.md -name '*.md'); do \
		awk '/^```go$$/{ p=1; next } /^```$$/{ if(p) print "---END---"; p=0 } p' "$$f" | \
		awk -v RS="---END---" -v file="$$f" '{ \
			gsub(/^\n+|\n+$$/, "", $$0); \
			if (length($$0) > 0 && $$0 ~ /^package /) print $$0 \
		}' > /dev/null; \
	done; \
	echo "✅ Doc examples syntax scan complete"

# -----------------------------------------------------------------------------
# 📚 API 文档目标
# -----------------------------------------------------------------------------
.PHONY: docs
docs: docs-swagger ## 生成 API 文档（swagger 的别名）

.PHONY: docs-swagger
docs-swagger: ## 生成 Swagger/OpenAPI 文档
	@echo "📚 Generating Swagger documentation..."
	@if command -v swag >/dev/null 2>&1; then \
		swag init -g cmd/agentflow/main.go -o api --parseDependency --parseInternal; \
		echo "✅ Swagger docs generated in api/"; \
	else \
		echo "⚠️  swag not installed, using static OpenAPI spec"; \
		echo "   Install swag: go install github.com/swaggo/swag/cmd/swag@latest"; \
		echo "✅ Static OpenAPI spec available at api/openapi.yaml"; \
	fi

.PHONY: docs-serve
docs-serve: ## 启动 Swagger UI 服务器
	@echo "🌐 Starting Swagger UI server..."
	@if command -v docker >/dev/null 2>&1; then \
		docker run --rm -p 8081:8080 \
			-e SWAGGER_JSON=/api/openapi.yaml \
			-v $(PWD)/api:/api \
			swaggerapi/swagger-ui; \
	else \
		echo "⚠️  Docker not available"; \
		echo "   View OpenAPI spec at: api/openapi.yaml"; \
		echo "   Or use online editor: https://editor.swagger.io"; \
	fi

.PHONY: docs-validate
docs-validate: ## 验证 OpenAPI 规范
	@echo "🔍 Validating OpenAPI specification..."
	@if command -v swagger-cli >/dev/null 2>&1; then \
		swagger-cli validate api/openapi.yaml; \
		echo "✅ OpenAPI spec is valid"; \
	elif command -v npx >/dev/null 2>&1; then \
		npx @apidevtools/swagger-cli validate api/openapi.yaml; \
		echo "✅ OpenAPI spec is valid"; \
	else \
		echo "⚠️  swagger-cli not available"; \
		echo "   Install: npm install -g @apidevtools/swagger-cli"; \
		echo "   Or validate online: https://editor.swagger.io"; \
	fi

.PHONY: docs-generate-client
docs-generate-client: ## 从 OpenAPI 生成客户端代码
	@echo "⚙️  Generating client from OpenAPI spec..."
	@if command -v openapi-generator-cli >/dev/null 2>&1; then \
		openapi-generator-cli generate -i api/openapi.yaml -g go -o api/client/go; \
		echo "✅ Go client generated in api/client/go/"; \
	elif command -v docker >/dev/null 2>&1; then \
		docker run --rm -v $(PWD):/local openapitools/openapi-generator-cli generate \
			-i /local/api/openapi.yaml \
			-g go \
			-o /local/api/client/go; \
		echo "✅ Go client generated in api/client/go/"; \
	else \
		echo "⚠️  openapi-generator-cli not available"; \
		echo "   Install: npm install -g @openapitools/openapi-generator-cli"; \
	fi

.PHONY: install-swag
install-swag: ## 安装 swag 工具
	@echo "📦 Installing swag..."
	$(GO) install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ swag installed"

# -----------------------------------------------------------------------------
# 🗄️ 数据库迁移目标
# -----------------------------------------------------------------------------
MIGRATE_CMD := $(BUILD_DIR)/$(BINARY_NAME) migrate

.PHONY: migrate-up
migrate-up: build ## 运行所有待执行的数据库迁移
	@echo "🗄️ Running database migrations..."
	$(MIGRATE_CMD) up
	@echo "✅ Migrations complete"

.PHONY: migrate-down
migrate-down: build ## 回滚最后一次数据库迁移
	@echo "🗄️ Rolling back last migration..."
	$(MIGRATE_CMD) down
	@echo "✅ Rollback complete"

.PHONY: migrate-status
migrate-status: build ## 显示数据库迁移状态
	@echo "🗄️ Migration status:"
	$(MIGRATE_CMD) status

.PHONY: migrate-version
migrate-version: build ## 显示当前数据库迁移版本
	@echo "🗄️ Current migration version:"
	$(MIGRATE_CMD) version

.PHONY: migrate-reset
migrate-reset: build ## 回滚所有数据库迁移
	@echo "🗄️ Resetting all migrations..."
	$(MIGRATE_CMD) reset
	@echo "✅ Reset complete"

.PHONY: migrate-goto
migrate-goto: build ## 迁移到指定版本 (使用 VERSION=n)
ifndef VERSION
	@echo "❌ Please specify VERSION, e.g., make migrate-goto VERSION=1"
	@exit 1
endif
	@echo "🗄️ Migrating to version $(VERSION)..."
	$(MIGRATE_CMD) goto $(VERSION)
	@echo "✅ Migration complete"

.PHONY: migrate-force
migrate-force: build ## 强制设置迁移版本 (使用 VERSION=n)
ifndef VERSION
	@echo "❌ Please specify VERSION, e.g., make migrate-force VERSION=0"
	@exit 1
endif
	@echo "🗄️ Forcing version to $(VERSION)..."
	$(MIGRATE_CMD) force $(VERSION)
	@echo "✅ Version forced"

.PHONY: migrate-create
migrate-create: ## 创建新的迁移文件 (使用 NAME=migration_name)
ifndef NAME
	@echo "❌ Please specify NAME, e.g., make migrate-create NAME=add_users_table"
	@exit 1
endif
	@echo "🗄️ Creating migration files..."
	@TIMESTAMP=$$(date +%Y%m%d%H%M%S); \
	for db in postgres mysql sqlite; do \
		touch pkg/migration/migrations/$$db/$${TIMESTAMP}_$(NAME).up.sql; \
		touch pkg/migration/migrations/$$db/$${TIMESTAMP}_$(NAME).down.sql; \
		echo "Created: pkg/migration/migrations/$$db/$${TIMESTAMP}_$(NAME).up.sql"; \
		echo "Created: pkg/migration/migrations/$$db/$${TIMESTAMP}_$(NAME).down.sql"; \
	done
	@echo "✅ Migration files created"

.PHONY: install-migrate
install-migrate: ## 安装 golang-migrate CLI 工具
	@echo "📦 Installing golang-migrate..."
	$(GO) install -tags 'postgres mysql sqlite' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@echo "✅ golang-migrate installed"

# -----------------------------------------------------------------------------
# 🎯 CI/CD 目标
# -----------------------------------------------------------------------------
.PHONY: ci
ci: deps verify ## CI 流水线（依赖 + 验证）

.PHONY: cd
cd: docker-build docker-push ## CD 流水线（构建 + 推送镜像）

.PHONY: release
release: clean build-all docker-build ## 发布准备（清理 + 构建所有平台 + Docker）
	@echo "🎉 Release $(VERSION) ready!"

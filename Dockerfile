# =============================================================================
# 🐳 AgentFlow Dockerfile - 多阶段构建
# =============================================================================
# 构建最小化的生产镜像，包含完整服务（HTTP/gRPC + 健康检查 + 指标）
#
# 使用方法:
#   docker build -t agentflow:latest .
#   docker build --build-arg VERSION=v1.0.0 -t agentflow:v1.0.0 .
# =============================================================================

# -----------------------------------------------------------------------------
# Stage 1: Builder - 编译 Go 应用
# -----------------------------------------------------------------------------
FROM golang:1.24-alpine AS builder

# 安装构建依赖（CGO 需要 gcc）
RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# 先复制依赖文件，利用 Docker 缓存层
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建参数
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

# 编译二进制文件
# -s -w: 去除调试信息，减小体积
# -X: 注入版本信息
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w \
        -X main.Version=${VERSION} \
        -X main.BuildTime=${BUILD_TIME} \
        -X main.GitCommit=${GIT_COMMIT}" \
    -o /app/agentflow \
    ./cmd/agentflow

# -----------------------------------------------------------------------------
# Stage 2: Runtime - 最小化运行时镜像
# -----------------------------------------------------------------------------
FROM alpine:3.19

# 安装运行时依赖
# ca-certificates: HTTPS 请求需要
# tzdata: 时区支持
# curl: 健康检查备用工具
RUN apk add --no-cache ca-certificates tzdata curl

# 创建非 root 用户（安全最佳实践）
RUN addgroup -g 1000 agentflow && \
    adduser -u 1000 -G agentflow -s /bin/sh -D agentflow

# 创建必要目录
RUN mkdir -p /app/config /app/data /app/scripts && \
    chown -R agentflow:agentflow /app

WORKDIR /app

# 从 builder 阶段复制二进制文件
COPY --from=builder /app/agentflow /app/agentflow

# 复制健康检查脚本
COPY --chmod=755 scripts/healthcheck.sh /app/scripts/healthcheck.sh

# 切换到非 root 用户
USER agentflow

# 暴露端口
# 8080: HTTP API / 健康检查
# 9090: gRPC 服务
# 9091: Prometheus 指标
EXPOSE 8080 9090 9091

# -----------------------------------------------------------------------------
# 健康检查配置
# -----------------------------------------------------------------------------
# --interval: 检查间隔（30秒）
# --timeout: 单次检查超时（10秒，增加以支持更多检查项）
# --start-period: 启动宽限期（30秒，给服务足够的启动时间）
# --retries: 失败重试次数（3次）
#
# 环境变量配置（可在 docker run 时覆盖）:
#   HEALTH_CHECK_REDIS=true   - 启用 Redis 检查
#   HEALTH_CHECK_DB=true      - 启用数据库检查
#   HEALTH_CHECK_GRPC=true    - 启用 gRPC 检查（默认开启）
#   HEALTH_CHECK_METRICS=true - 启用指标端点检查（默认开启）
# -----------------------------------------------------------------------------
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD /app/scripts/healthcheck.sh

# 入口点
ENTRYPOINT ["/app/agentflow"]

# 默认命令（可被覆盖）
CMD ["serve"]

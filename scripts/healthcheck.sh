#!/bin/sh
# =============================================================================
# 🏥 AgentFlow 健康检查脚本
# =============================================================================
# 全面检查服务及其依赖的健康状态
# 用于 Docker HEALTHCHECK 和 Kubernetes liveness/readiness 探针
#
# 退出码:
#   0 - 所有检查通过，服务健康
#   1 - 至少一个检查失败，服务不健康
# =============================================================================

set -e

# -----------------------------------------------------------------------------
# 配置变量（可通过环境变量覆盖）
# -----------------------------------------------------------------------------
HTTP_HOST="${HEALTH_HTTP_HOST:-localhost}"
HTTP_PORT="${HEALTH_HTTP_PORT:-8080}"
METRICS_PORT="${HEALTH_METRICS_PORT:-9091}"

# 超时设置（秒）
TIMEOUT="${HEALTH_TIMEOUT:-5}"

# 可选依赖检查开关
CHECK_REDIS="${HEALTH_CHECK_REDIS:-false}"
CHECK_DB="${HEALTH_CHECK_DB:-false}"
CHECK_METRICS="${HEALTH_CHECK_METRICS:-true}"

# Redis 配置
REDIS_HOST="${REDIS_HOST:-localhost}"
REDIS_PORT="${REDIS_PORT:-6379}"

# 数据库配置
DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"

# -----------------------------------------------------------------------------
# 辅助函数
# -----------------------------------------------------------------------------
log_info() {
    echo "[INFO] $1"
}

log_error() {
    echo "[ERROR] $1" >&2
}

log_success() {
    echo "[OK] $1"
}

# 检查命令是否存在
check_command() {
    command -v "$1" >/dev/null 2>&1
}

# -----------------------------------------------------------------------------
# 健康检查函数
# -----------------------------------------------------------------------------

# 检查 HTTP 健康端点
check_http_health() {
    log_info "Checking HTTP health endpoint..."

    if check_command wget; then
        if wget --no-verbose --tries=1 --timeout="$TIMEOUT" --spider "http://${HTTP_HOST}:${HTTP_PORT}/health" 2>/dev/null; then
            log_success "HTTP health check passed"
            return 0
        fi
    elif check_command curl; then
        if curl --silent --fail --max-time "$TIMEOUT" "http://${HTTP_HOST}:${HTTP_PORT}/health" >/dev/null 2>&1; then
            log_success "HTTP health check passed"
            return 0
        fi
    else
        log_error "Neither wget nor curl available for HTTP check"
        return 1
    fi

    log_error "HTTP health check failed"
    return 1
}

# 检查 HTTP readiness 端点（更详细的就绪检查）
check_http_ready() {
    log_info "Checking HTTP readiness endpoint..."

    local response=""
    if check_command wget; then
        response=$(wget --quiet --timeout="$TIMEOUT" -O - "http://${HTTP_HOST}:${HTTP_PORT}/ready" 2>/dev/null) || true
    elif check_command curl; then
        response=$(curl --silent --max-time "$TIMEOUT" "http://${HTTP_HOST}:${HTTP_PORT}/ready" 2>/dev/null) || true
    fi

    if [ -n "$response" ]; then
        log_success "HTTP readiness check passed"
        return 0
    fi

    # 如果 /ready 不存在，回退到 /health
    log_info "Readiness endpoint not available, falling back to health check"
    return 0
}

# 检查 Prometheus 指标端点
check_metrics() {
    if [ "$CHECK_METRICS" != "true" ]; then
        log_info "Metrics check skipped (disabled)"
        return 0
    fi

    log_info "Checking Prometheus metrics endpoint..."

    if check_command wget; then
        if wget --no-verbose --tries=1 --timeout="$TIMEOUT" --spider "http://${HTTP_HOST}:${METRICS_PORT}/metrics" 2>/dev/null; then
            log_success "Metrics endpoint check passed"
            return 0
        fi
    elif check_command curl; then
        if curl --silent --fail --max-time "$TIMEOUT" "http://${HTTP_HOST}:${METRICS_PORT}/metrics" >/dev/null 2>&1; then
            log_success "Metrics endpoint check passed"
            return 0
        fi
    fi

    log_info "Metrics endpoint not available (may be expected)"
    return 0
}

# 检查 Redis 连接
check_redis() {
    if [ "$CHECK_REDIS" != "true" ]; then
        log_info "Redis check skipped (disabled)"
        return 0
    fi

    log_info "Checking Redis connection..."

    if check_command redis-cli; then
        if redis-cli -h "$REDIS_HOST" -p "$REDIS_PORT" ping 2>/dev/null | grep -q "PONG"; then
            log_success "Redis connection check passed"
            return 0
        fi
        log_error "Redis connection check failed"
        return 1
    fi

    # 回退：检查端口是否监听
    if check_command nc; then
        if nc -z -w "$TIMEOUT" "$REDIS_HOST" "$REDIS_PORT" 2>/dev/null; then
            log_success "Redis port is listening"
            return 0
        fi
        log_error "Redis port is not listening"
        return 1
    fi

    log_info "Redis client not available, skipping check"
    return 0
}

# 检查数据库连接
check_database() {
    if [ "$CHECK_DB" != "true" ]; then
        log_info "Database check skipped (disabled)"
        return 0
    fi

    log_info "Checking database connection..."

    # PostgreSQL
    if check_command pg_isready; then
        if pg_isready -h "$DB_HOST" -p "$DB_PORT" -t "$TIMEOUT" >/dev/null 2>&1; then
            log_success "PostgreSQL connection check passed"
            return 0
        fi
        log_error "PostgreSQL connection check failed"
        return 1
    fi

    # 回退：检查端口是否监听
    if check_command nc; then
        if nc -z -w "$TIMEOUT" "$DB_HOST" "$DB_PORT" 2>/dev/null; then
            log_success "Database port is listening"
            return 0
        fi
        log_error "Database port is not listening"
        return 1
    fi

    log_info "Database client not available, skipping check"
    return 0
}

# -----------------------------------------------------------------------------
# 主函数
# -----------------------------------------------------------------------------
main() {
    echo "=============================================="
    echo "🏥 AgentFlow Health Check"
    echo "=============================================="
    echo ""

    local failed=0

    # 核心检查（必须通过）
    if ! check_http_health; then
        failed=1
    fi

    # 可选检查
    check_http_ready || true

    check_metrics || true

    if ! check_redis; then
        failed=1
    fi

    if ! check_database; then
        failed=1
    fi

    echo ""
    echo "=============================================="

    if [ $failed -eq 0 ]; then
        echo "✅ All health checks passed!"
        echo "=============================================="
        exit 0
    else
        echo "❌ Some health checks failed!"
        echo "=============================================="
        exit 1
    fi
}

# 运行主函数
main "$@"

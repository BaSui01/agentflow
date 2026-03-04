-- =============================================================================
-- AgentFlow Database Migration: Tool Provider Configs
-- Database: MySQL
-- Version: 000003
-- Description: Persist web_search provider configuration for hosted tools
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_provider_configs (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    api_key TEXT,
    base_url TEXT,
    timeout_seconds INT NOT NULL DEFAULT 15,
    priority INT NOT NULL DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_sc_tool_provider_configs_provider (provider),
    INDEX idx_sc_tool_provider_configs_enabled (enabled),
    INDEX idx_sc_tool_provider_configs_priority (priority)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='DB-managed web_search provider configuration';

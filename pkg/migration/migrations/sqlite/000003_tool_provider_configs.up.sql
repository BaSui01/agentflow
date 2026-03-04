-- =============================================================================
-- AgentFlow Database Migration: Tool Provider Configs
-- Database: SQLite
-- Version: 000003
-- Description: Persist web_search provider configuration for hosted tools
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_provider_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL UNIQUE,
    api_key TEXT,
    base_url TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 15,
    priority INTEGER NOT NULL DEFAULT 100,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sc_tool_provider_configs_enabled ON sc_tool_provider_configs(enabled);
CREATE INDEX IF NOT EXISTS idx_sc_tool_provider_configs_priority ON sc_tool_provider_configs(priority);

CREATE TRIGGER IF NOT EXISTS update_sc_tool_provider_configs_updated_at
    AFTER UPDATE ON sc_tool_provider_configs
    FOR EACH ROW
BEGIN
    UPDATE sc_tool_provider_configs SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

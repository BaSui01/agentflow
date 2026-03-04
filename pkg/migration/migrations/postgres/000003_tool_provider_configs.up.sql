-- =============================================================================
-- AgentFlow Database Migration: Tool Provider Configs
-- Database: PostgreSQL
-- Version: 000003
-- Description: Persist web_search provider configuration for hosted tools
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_provider_configs (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    api_key TEXT,
    base_url TEXT,
    timeout_seconds INTEGER NOT NULL DEFAULT 15,
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sc_tool_provider_configs_provider ON sc_tool_provider_configs(provider);
CREATE INDEX IF NOT EXISTS idx_sc_tool_provider_configs_enabled ON sc_tool_provider_configs(enabled);
CREATE INDEX IF NOT EXISTS idx_sc_tool_provider_configs_priority ON sc_tool_provider_configs(priority);

COMMENT ON TABLE sc_tool_provider_configs IS 'DB-managed web_search provider configuration';

CREATE TRIGGER update_sc_tool_provider_configs_updated_at
    BEFORE UPDATE ON sc_tool_provider_configs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

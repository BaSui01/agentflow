-- =============================================================================
-- AgentFlow Database Migration: Tool Registrations
-- Database: PostgreSQL
-- Version: 000002
-- Description: Create DB-managed hosted tool registration table
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_registrations (
    id SERIAL PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    description TEXT,
    target VARCHAR(120) NOT NULL,
    parameters JSONB,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_sc_tool_registrations_name ON sc_tool_registrations(name);
CREATE INDEX IF NOT EXISTS idx_sc_tool_registrations_enabled ON sc_tool_registrations(enabled);

COMMENT ON TABLE sc_tool_registrations IS 'DB-managed hosted tool alias registrations';
COMMENT ON COLUMN sc_tool_registrations.target IS 'Runtime target tool name (e.g., retrieval, mcp_xxx)';

CREATE TRIGGER update_sc_tool_registrations_updated_at
    BEFORE UPDATE ON sc_tool_registrations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

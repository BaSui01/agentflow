-- =============================================================================
-- AgentFlow Database Migration Rollback: Tool Registrations
-- Database: SQLite
-- Version: 000002
-- Description: Drop DB-managed hosted tool registration table
-- =============================================================================

DROP TRIGGER IF EXISTS update_sc_tool_registrations_updated_at;
DROP TABLE IF EXISTS sc_tool_registrations;

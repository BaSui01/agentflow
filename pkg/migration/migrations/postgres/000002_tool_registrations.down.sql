-- =============================================================================
-- AgentFlow Database Migration Rollback: Tool Registrations
-- Database: PostgreSQL
-- Version: 000002
-- Description: Drop DB-managed hosted tool registration table
-- =============================================================================

DROP TRIGGER IF EXISTS update_sc_tool_registrations_updated_at ON sc_tool_registrations;
DROP TABLE IF EXISTS sc_tool_registrations CASCADE;

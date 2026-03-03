-- =============================================================================
-- AgentFlow Database Migration Rollback: Tool Registrations
-- Database: MySQL
-- Version: 000002
-- Description: Drop DB-managed hosted tool registration table
-- =============================================================================

DROP TABLE IF EXISTS sc_tool_registrations;

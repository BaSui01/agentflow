-- =============================================================================
-- AgentFlow Database Migration: Tool Registrations
-- Database: SQLite
-- Version: 000002
-- Description: Create DB-managed hosted tool registration table
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_registrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    target TEXT NOT NULL,
    parameters TEXT,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_sc_tool_registrations_enabled ON sc_tool_registrations(enabled);

CREATE TRIGGER IF NOT EXISTS update_sc_tool_registrations_updated_at
    AFTER UPDATE ON sc_tool_registrations
    FOR EACH ROW
BEGIN
    UPDATE sc_tool_registrations SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

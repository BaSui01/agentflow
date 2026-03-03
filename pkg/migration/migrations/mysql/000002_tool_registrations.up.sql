-- =============================================================================
-- AgentFlow Database Migration: Tool Registrations
-- Database: MySQL
-- Version: 000002
-- Description: Create DB-managed hosted tool registration table
-- =============================================================================

CREATE TABLE IF NOT EXISTS sc_tool_registrations (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(120) NOT NULL,
    description TEXT,
    target VARCHAR(120) NOT NULL,
    parameters JSON,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_sc_tool_registrations_name (name),
    INDEX idx_sc_tool_registrations_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='DB-managed hosted tool alias registrations';

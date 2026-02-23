-- =============================================================================
-- AgentFlow Database Migration: Rollback Initial Schema
-- Database: MySQL
-- Version: 000001
-- Description: Drop all initial tables
-- =============================================================================

-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS sc_llm_provider_api_keys;
DROP TABLE IF EXISTS sc_llm_provider_models;
DROP TABLE IF EXISTS sc_llm_models;
DROP TABLE IF EXISTS sc_llm_providers;

-- =============================================================================
-- AgentFlow Database Migration: Rollback Initial Schema
-- Database: SQLite
-- Version: 000001
-- Description: Drop all initial tables
-- =============================================================================

-- Drop triggers first
DROP TRIGGER IF EXISTS update_sc_llm_provider_api_keys_updated_at;
DROP TRIGGER IF EXISTS update_sc_llm_provider_models_updated_at;
DROP TRIGGER IF EXISTS update_sc_llm_models_updated_at;
DROP TRIGGER IF EXISTS update_sc_llm_providers_updated_at;

-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS sc_llm_provider_api_keys;
DROP TABLE IF EXISTS sc_llm_provider_models;
DROP TABLE IF EXISTS sc_llm_models;
DROP TABLE IF EXISTS sc_llm_providers;

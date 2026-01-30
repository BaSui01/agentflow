-- =============================================================================
-- AgentFlow Database Migration: Rollback Initial Schema
-- Database: PostgreSQL
-- Version: 000001
-- Description: Drop all initial tables
-- =============================================================================

-- Drop triggers first
DROP TRIGGER IF EXISTS update_sc_llm_provider_api_keys_updated_at ON sc_llm_provider_api_keys;
DROP TRIGGER IF EXISTS update_sc_llm_provider_models_updated_at ON sc_llm_provider_models;
DROP TRIGGER IF EXISTS update_sc_llm_models_updated_at ON sc_llm_models;
DROP TRIGGER IF EXISTS update_sc_llm_providers_updated_at ON sc_llm_providers;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse order (respecting foreign key constraints)
DROP TABLE IF EXISTS sc_llm_provider_api_keys CASCADE;
DROP TABLE IF EXISTS sc_llm_provider_models CASCADE;
DROP TABLE IF EXISTS sc_llm_models CASCADE;
DROP TABLE IF EXISTS sc_llm_providers CASCADE;

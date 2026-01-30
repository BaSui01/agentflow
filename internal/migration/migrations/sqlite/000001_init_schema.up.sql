-- =============================================================================
-- AgentFlow Database Migration: Initial Schema
-- Database: SQLite
-- Version: 000001
-- Description: Create initial tables for LLM provider management
-- =============================================================================

-- =============================================================================
-- Table: sc_llm_providers
-- Description: LLM providers (e.g., OpenAI, Anthropic, DeepSeek)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_providers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    description TEXT,
    status INTEGER DEFAULT 1,  -- 0=inactive, 1=active, 2=disabled
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create index
CREATE INDEX IF NOT EXISTS idx_llm_providers_code ON sc_llm_providers(code);

-- =============================================================================
-- Table: sc_llm_models
-- Description: Abstract LLM models (e.g., gpt-4, claude-3-opus)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_name TEXT NOT NULL UNIQUE,
    display_name TEXT,
    description TEXT,
    enabled INTEGER DEFAULT 1,  -- SQLite uses INTEGER for BOOLEAN
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create index
CREATE INDEX IF NOT EXISTS idx_llm_models_model_name ON sc_llm_models(model_name);

-- =============================================================================
-- Table: sc_llm_provider_models
-- Description: Provider-specific model configurations (many-to-many)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_models (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    model_id INTEGER NOT NULL,
    provider_id INTEGER NOT NULL,
    remote_model_name TEXT NOT NULL,
    base_url TEXT,
    price_input REAL DEFAULT 0,
    price_completion REAL DEFAULT 0,
    max_tokens INTEGER DEFAULT 0,
    priority INTEGER DEFAULT 100,
    enabled INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (model_id) REFERENCES sc_llm_models(id) ON DELETE CASCADE,
    FOREIGN KEY (provider_id) REFERENCES sc_llm_providers(id) ON DELETE CASCADE
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_provider_models_model_provider ON sc_llm_provider_models(model_id, provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON sc_llm_provider_models(provider_id);

-- =============================================================================
-- Table: sc_llm_provider_api_keys
-- Description: API key pool for load balancing and failover
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider_id INTEGER NOT NULL,
    api_key TEXT NOT NULL,
    label TEXT,
    priority INTEGER DEFAULT 100,
    weight INTEGER DEFAULT 100,
    enabled INTEGER DEFAULT 1,

    -- Usage statistics
    total_requests INTEGER DEFAULT 0,
    failed_requests INTEGER DEFAULT 0,
    last_used_at DATETIME,
    last_error_at DATETIME,
    last_error TEXT,

    -- Rate limiting
    rate_limit_rpm INTEGER DEFAULT 0,
    rate_limit_rpd INTEGER DEFAULT 0,
    current_rpm INTEGER DEFAULT 0,
    current_rpd INTEGER DEFAULT 0,
    rpm_reset_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    rpd_reset_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (provider_id) REFERENCES sc_llm_providers(id) ON DELETE CASCADE
);

-- Create index
CREATE INDEX IF NOT EXISTS idx_provider_api_keys_provider_id ON sc_llm_provider_api_keys(provider_id);

-- =============================================================================
-- Triggers: Update updated_at timestamp
-- SQLite doesn't support ON UPDATE, so we use triggers
-- =============================================================================
CREATE TRIGGER IF NOT EXISTS update_sc_llm_providers_updated_at
    AFTER UPDATE ON sc_llm_providers
    FOR EACH ROW
BEGIN
    UPDATE sc_llm_providers SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_sc_llm_models_updated_at
    AFTER UPDATE ON sc_llm_models
    FOR EACH ROW
BEGIN
    UPDATE sc_llm_models SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_sc_llm_provider_models_updated_at
    AFTER UPDATE ON sc_llm_provider_models
    FOR EACH ROW
BEGIN
    UPDATE sc_llm_provider_models SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

CREATE TRIGGER IF NOT EXISTS update_sc_llm_provider_api_keys_updated_at
    AFTER UPDATE ON sc_llm_provider_api_keys
    FOR EACH ROW
BEGIN
    UPDATE sc_llm_provider_api_keys SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- =============================================================================
-- AgentFlow Database Migration: Initial Schema
-- Database: PostgreSQL
-- Version: 000001
-- Description: Create initial tables for LLM provider management
-- =============================================================================

-- Enable UUID extension if not exists
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- =============================================================================
-- Table: sc_llm_providers
-- Description: LLM providers (e.g., OpenAI, Anthropic, DeepSeek)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_providers (
    id SERIAL PRIMARY KEY,
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    status SMALLINT DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create unique index on code
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_providers_code ON sc_llm_providers(code);

-- Add comments
COMMENT ON TABLE sc_llm_providers IS 'LLM providers registry';
COMMENT ON COLUMN sc_llm_providers.code IS 'Unique provider code (e.g., openai, anthropic)';
COMMENT ON COLUMN sc_llm_providers.name IS 'Display name of the provider';
COMMENT ON COLUMN sc_llm_providers.status IS '0=inactive, 1=active, 2=disabled';

-- =============================================================================
-- Table: sc_llm_models
-- Description: Abstract LLM models (e.g., gpt-4, claude-3-opus)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_models (
    id SERIAL PRIMARY KEY,
    model_name VARCHAR(100) NOT NULL,
    display_name VARCHAR(200),
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Create unique index on model_name
CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_models_model_name ON sc_llm_models(model_name);

-- Add comments
COMMENT ON TABLE sc_llm_models IS 'Abstract LLM models registry';
COMMENT ON COLUMN sc_llm_models.model_name IS 'Unique model identifier';
COMMENT ON COLUMN sc_llm_models.display_name IS 'Human-readable model name';

-- =============================================================================
-- Table: sc_llm_provider_models
-- Description: Provider-specific model configurations (many-to-many)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_models (
    id SERIAL PRIMARY KEY,
    model_id INTEGER NOT NULL,
    provider_id INTEGER NOT NULL,
    remote_model_name VARCHAR(100) NOT NULL,
    base_url VARCHAR(500),
    price_input DECIMAL(10, 6) DEFAULT 0,
    price_completion DECIMAL(10, 6) DEFAULT 0,
    max_tokens INTEGER DEFAULT 0,
    priority INTEGER DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Foreign key constraints
    CONSTRAINT fk_provider_models_model FOREIGN KEY (model_id)
        REFERENCES sc_llm_models(id) ON DELETE CASCADE,
    CONSTRAINT fk_provider_models_provider FOREIGN KEY (provider_id)
        REFERENCES sc_llm_providers(id) ON DELETE CASCADE
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_provider_models_model_provider ON sc_llm_provider_models(model_id, provider_id);
CREATE INDEX IF NOT EXISTS idx_provider_models_provider_id ON sc_llm_provider_models(provider_id);

-- Add comments
COMMENT ON TABLE sc_llm_provider_models IS 'Provider-specific model configurations';
COMMENT ON COLUMN sc_llm_provider_models.remote_model_name IS 'Model name used in API calls';
COMMENT ON COLUMN sc_llm_provider_models.price_input IS 'Price per 1K input tokens (USD)';
COMMENT ON COLUMN sc_llm_provider_models.price_completion IS 'Price per 1K output tokens (USD)';

-- =============================================================================
-- Table: sc_llm_provider_api_keys
-- Description: API key pool for load balancing and failover
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_api_keys (
    id SERIAL PRIMARY KEY,
    provider_id INTEGER NOT NULL,
    api_key VARCHAR(500) NOT NULL,
    label VARCHAR(100),
    priority INTEGER DEFAULT 100,
    weight INTEGER DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,

    -- Usage statistics
    total_requests BIGINT DEFAULT 0,
    failed_requests BIGINT DEFAULT 0,
    last_used_at TIMESTAMP WITH TIME ZONE,
    last_error_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,

    -- Rate limiting
    rate_limit_rpm INTEGER DEFAULT 0,
    rate_limit_rpd INTEGER DEFAULT 0,
    current_rpm INTEGER DEFAULT 0,
    current_rpd INTEGER DEFAULT 0,
    rpm_reset_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    rpd_reset_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Foreign key constraint
    CONSTRAINT fk_api_keys_provider FOREIGN KEY (provider_id)
        REFERENCES sc_llm_providers(id) ON DELETE CASCADE
);

-- Create index
CREATE INDEX IF NOT EXISTS idx_provider_api_keys_provider_id ON sc_llm_provider_api_keys(provider_id);

-- Add comments
COMMENT ON TABLE sc_llm_provider_api_keys IS 'API key pool for providers';
COMMENT ON COLUMN sc_llm_provider_api_keys.weight IS 'Weight for load balancing (higher = more traffic)';
COMMENT ON COLUMN sc_llm_provider_api_keys.rate_limit_rpm IS 'Requests per minute limit (0 = unlimited)';
COMMENT ON COLUMN sc_llm_provider_api_keys.rate_limit_rpd IS 'Requests per day limit (0 = unlimited)';

-- =============================================================================
-- Table: sc_schema_migrations
-- Description: Track migration history (used by golang-migrate)
-- =============================================================================
-- Note: This table is automatically created by golang-migrate
-- We include it here for documentation purposes

-- =============================================================================
-- Trigger: Update updated_at timestamp
-- =============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to all tables
CREATE TRIGGER update_sc_llm_providers_updated_at
    BEFORE UPDATE ON sc_llm_providers
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sc_llm_models_updated_at
    BEFORE UPDATE ON sc_llm_models
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sc_llm_provider_models_updated_at
    BEFORE UPDATE ON sc_llm_provider_models
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_sc_llm_provider_api_keys_updated_at
    BEFORE UPDATE ON sc_llm_provider_api_keys
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

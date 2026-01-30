-- =============================================================================
-- AgentFlow Database Migration: Initial Schema
-- Database: MySQL
-- Version: 000001
-- Description: Create initial tables for LLM provider management
-- =============================================================================

-- =============================================================================
-- Table: sc_llm_providers
-- Description: LLM providers (e.g., OpenAI, Anthropic, DeepSeek)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_providers (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code VARCHAR(50) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT,
    status SMALLINT DEFAULT 1 COMMENT '0=inactive, 1=active, 2=disabled',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_llm_providers_code (code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='LLM providers registry';

-- =============================================================================
-- Table: sc_llm_models
-- Description: Abstract LLM models (e.g., gpt-4, claude-3-opus)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_models (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    model_name VARCHAR(100) NOT NULL COMMENT 'Unique model identifier',
    display_name VARCHAR(200) COMMENT 'Human-readable model name',
    description TEXT,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE INDEX idx_llm_models_model_name (model_name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Abstract LLM models registry';

-- =============================================================================
-- Table: sc_llm_provider_models
-- Description: Provider-specific model configurations (many-to-many)
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_models (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    model_id INT UNSIGNED NOT NULL,
    provider_id INT UNSIGNED NOT NULL,
    remote_model_name VARCHAR(100) NOT NULL COMMENT 'Model name used in API calls',
    base_url VARCHAR(500),
    price_input DECIMAL(10, 6) DEFAULT 0 COMMENT 'Price per 1K input tokens (USD)',
    price_completion DECIMAL(10, 6) DEFAULT 0 COMMENT 'Price per 1K output tokens (USD)',
    max_tokens INT DEFAULT 0,
    priority INT DEFAULT 100,
    enabled BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_provider_models_model_provider (model_id, provider_id),
    INDEX idx_provider_models_provider_id (provider_id),

    CONSTRAINT fk_provider_models_model FOREIGN KEY (model_id)
        REFERENCES sc_llm_models(id) ON DELETE CASCADE,
    CONSTRAINT fk_provider_models_provider FOREIGN KEY (provider_id)
        REFERENCES sc_llm_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='Provider-specific model configurations';

-- =============================================================================
-- Table: sc_llm_provider_api_keys
-- Description: API key pool for load balancing and failover
-- =============================================================================
CREATE TABLE IF NOT EXISTS sc_llm_provider_api_keys (
    id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    provider_id INT UNSIGNED NOT NULL,
    api_key VARCHAR(500) NOT NULL,
    label VARCHAR(100),
    priority INT DEFAULT 100,
    weight INT DEFAULT 100 COMMENT 'Weight for load balancing (higher = more traffic)',
    enabled BOOLEAN DEFAULT TRUE,

    -- Usage statistics
    total_requests BIGINT DEFAULT 0,
    failed_requests BIGINT DEFAULT 0,
    last_used_at TIMESTAMP NULL,
    last_error_at TIMESTAMP NULL,
    last_error TEXT,

    -- Rate limiting
    rate_limit_rpm INT DEFAULT 0 COMMENT 'Requests per minute limit (0 = unlimited)',
    rate_limit_rpd INT DEFAULT 0 COMMENT 'Requests per day limit (0 = unlimited)',
    current_rpm INT DEFAULT 0,
    current_rpd INT DEFAULT 0,
    rpm_reset_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    rpd_reset_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_provider_api_keys_provider_id (provider_id),

    CONSTRAINT fk_api_keys_provider FOREIGN KEY (provider_id)
        REFERENCES sc_llm_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
COMMENT='API key pool for providers';

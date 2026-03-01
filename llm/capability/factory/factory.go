package factory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/llm"
	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/rerank"
	"gorm.io/gorm"
)

// Capability 标识可统一构建的能力。
type Capability string

const (
	CapabilityEmbedding Capability = "embedding"
	CapabilityImage     Capability = "image"
	CapabilityRerank    Capability = "rerank"
)

// ProviderBinding 表示从数据库解析出的 provider 连接信息。
type ProviderBinding struct {
	ProviderID   uint
	ProviderCode string
	APIKey       string
	BaseURL      string
	Model        string
}

type providerRow struct {
	ProviderID uint
	Code       string
	APIKey     string
	BaseURL    string
	Model      string
}

// LoadBindingsFromDB 从数据库按能力读取 provider 绑定，作为统一入口的数据源。
func LoadBindingsFromDB(ctx context.Context, db *gorm.DB, cap Capability) ([]ProviderBinding, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}

	rows := make([]providerRow, 0)
	err := db.WithContext(ctx).
		Table("sc_llm_providers p").
		Select("p.id as provider_id, p.code as code, k.api_key as api_key, k.base_url as base_url, pm.remote_model_name as model").
		Joins("JOIN sc_llm_provider_api_keys k ON k.provider_id = p.id AND k.enabled = TRUE").
		Joins("LEFT JOIN sc_llm_provider_models pm ON pm.provider_id = p.id AND pm.enabled = TRUE").
		Where("p.status = ?", llm.LLMProviderStatusActive).
		Order("p.id ASC, k.priority ASC, pm.priority ASC, pm.id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	out := make([]ProviderBinding, 0, len(rows))
	for _, row := range rows {
		code := normalizeProviderCode(row.Code)
		if !supportsCapability(cap, code) {
			continue
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, ProviderBinding{
			ProviderID:   row.ProviderID,
			ProviderCode: code,
			APIKey:       row.APIKey,
			BaseURL:      row.BaseURL,
			Model:        row.Model,
		})
	}
	return out, nil
}

// SelectBinding 选择指定 provider（为空则取第一个）。
func SelectBinding(bindings []ProviderBinding, providerCode string) (ProviderBinding, error) {
	if len(bindings) == 0 {
		return ProviderBinding{}, fmt.Errorf("no provider bindings available")
	}
	if providerCode == "" {
		return bindings[0], nil
	}
	want := normalizeProviderCode(providerCode)
	for _, b := range bindings {
		if b.ProviderCode == want {
			return b, nil
		}
	}
	return ProviderBinding{}, fmt.Errorf("provider %q not available", providerCode)
}

// NewEmbeddingProvider 通过统一入口构建 embedding provider。
func NewEmbeddingProvider(binding ProviderBinding, timeout time.Duration) (embedding.Provider, error) {
	return embedding.NewProviderFromConfig(embedding.FactoryConfig{
		Type:    embedding.ProviderType(binding.ProviderCode),
		APIKey:  binding.APIKey,
		BaseURL: binding.BaseURL,
		Model:   binding.Model,
		Timeout: timeout,
	})
}

// NewImageProvider 通过统一入口构建 image provider。
func NewImageProvider(binding ProviderBinding, timeout time.Duration) (image.Provider, error) {
	return image.NewProviderFromConfig(image.FactoryConfig{
		Type:    image.ProviderType(binding.ProviderCode),
		APIKey:  binding.APIKey,
		BaseURL: binding.BaseURL,
		Model:   binding.Model,
		Timeout: timeout,
	})
}

// NewRerankProvider 通过统一入口构建 rerank provider。
func NewRerankProvider(binding ProviderBinding, timeout time.Duration) (rerank.Provider, error) {
	return rerank.NewProviderFromConfig(rerank.FactoryConfig{
		Type:    rerank.ProviderType(binding.ProviderCode),
		APIKey:  binding.APIKey,
		BaseURL: binding.BaseURL,
		Model:   binding.Model,
		Timeout: timeout,
	})
}

func supportsCapability(cap Capability, providerCode string) bool {
	switch cap {
	case CapabilityEmbedding:
		return providerCode == "openai" || providerCode == "cohere" || providerCode == "voyage" || providerCode == "jina" || providerCode == "gemini"
	case CapabilityImage:
		return providerCode == "openai" || providerCode == "gemini" || providerCode == "flux"
	case CapabilityRerank:
		return providerCode == "cohere" || providerCode == "voyage" || providerCode == "jina"
	default:
		return false
	}
}

func normalizeProviderCode(code string) string {
	c := strings.ToLower(strings.TrimSpace(code))
	switch c {
	case "bfl", "black-forest-labs":
		return "flux"
	default:
		return c
	}
}

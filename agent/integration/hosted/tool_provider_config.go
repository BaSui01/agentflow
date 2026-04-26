package hosted

import "time"

// ToolProviderName identifies supported external providers for hosted tools.
type ToolProviderName string

const (
	ToolProviderTavily     ToolProviderName = "tavily"
	ToolProviderFirecrawl  ToolProviderName = "firecrawl"
	ToolProviderDuckDuckGo ToolProviderName = "duckduckgo"
	ToolProviderSearXNG    ToolProviderName = "searxng"
	ToolProviderBrave      ToolProviderName = "brave"
	ToolProviderBing       ToolProviderName = "bing"
)

// ToolProviderConfig stores DB-managed provider configuration for hosted tools.
// For now this table is scoped to web_search provider wiring.
type ToolProviderConfig struct {
	ID uint `gorm:"primaryKey" json:"id"`
	// Provider must be unique (tavily/firecrawl/duckduckgo/searxng).
	Provider string `gorm:"size:32;not null;uniqueIndex" json:"provider"`
	// APIKey is optional for duckduckgo/searxng and required for tavily/firecrawl.
	APIKey string `gorm:"type:text" json:"-"`
	// BaseURL is optional. When empty, provider-specific defaults are used.
	BaseURL string `gorm:"type:text" json:"base_url,omitempty"`
	// TimeoutSeconds controls provider request timeout.
	TimeoutSeconds int `gorm:"not null;default:15" json:"timeout_seconds"`
	// Priority controls active-provider selection among enabled rows (lower wins).
	Priority  int       `gorm:"not null;default:100;index" json:"priority"`
	Enabled   bool      `gorm:"default:true;index" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (ToolProviderConfig) TableName() string {
	return "sc_tool_provider_configs"
}

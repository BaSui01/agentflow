package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogLookupByIDAndAlias(t *testing.T) {
	catalog := NewModelCatalog([]ModelDescriptor{{
		Provider:            "openai",
		ID:                  "gpt-5.4",
		Aliases:             []string{"gpt-5"},
		Stage:               ModelStageStable,
		ContextWindowTokens: 400000,
		MaxOutputTokens:     128000,
		Capabilities: []ModelCapability{
			ModelCapabilityReasoning,
			ModelCapabilityToolCalling,
			ModelCapabilityStructuredOutput,
		},
		EndpointFamilies: []ModelEndpointFamily{ModelEndpointOpenAIResponses},
		VerifiedAt:       "2026-04-24",
		SourceURLs:       []string{"https://developers.openai.com/"},
		Metadata:         map[string]string{"api_status": "available"},
	}})

	model, ok := catalog.Lookup("OpenAI", "GPT-5")
	require.True(t, ok)
	assert.Equal(t, "gpt-5.4", model.ID)
	assert.True(t, model.Supports(ModelCapabilityReasoning))
	assert.False(t, model.Supports(ModelCapabilityWebSearch))

	model.Aliases[0] = "mutated"
	model.Metadata["api_status"] = "mutated"

	again, ok := catalog.Lookup("openai", "gpt-5.4")
	require.True(t, ok)
	assert.Equal(t, []string{"gpt-5"}, again.Aliases)
	assert.Equal(t, "available", again.Metadata["api_status"])
}

func TestModelCatalogModelsForProviderReturnsClones(t *testing.T) {
	catalog := NewModelCatalog([]ModelDescriptor{
		{Provider: "openai", ID: "gpt-5.4", Capabilities: []ModelCapability{ModelCapabilityStreaming}},
		{Provider: "anthropic", ID: "claude-sonnet-4-5", Capabilities: []ModelCapability{ModelCapabilityThinking}},
	})

	models := catalog.ModelsForProvider("OPENAI")
	require.Len(t, models, 1)
	models[0].Capabilities[0] = ModelCapabilityWebSearch

	again := catalog.ModelsForProvider("openai")
	require.Len(t, again, 1)
	assert.Equal(t, []ModelCapability{ModelCapabilityStreaming}, again[0].Capabilities)
}

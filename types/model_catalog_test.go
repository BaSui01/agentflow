package types

import (
	"strings"
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

func TestDefaultModelCatalogIncludesMainstreamAgentModels(t *testing.T) {
	catalog := DefaultModelCatalog()
	require.NotNil(t, catalog)

	openaiModel, ok := catalog.Lookup("openai", "gpt-5")
	require.True(t, ok)
	assert.Equal(t, "gpt-5.4", openaiModel.ID)
	assert.Equal(t, DefaultModelCatalogVerifiedAt, openaiModel.VerifiedAt)
	assert.True(t, openaiModel.Default)
	assert.True(t, openaiModel.Supports(ModelCapabilityReasoning))
	assert.True(t, openaiModel.Supports(ModelCapabilityWebSearch))
	assert.Contains(t, openaiModel.EndpointFamilies, ModelEndpointOpenAIResponses)
	assert.Equal(t, "max_output_tokens", openaiModel.Metadata["length_field"])

	qwenModel, ok := catalog.Lookup("qwen", "qwen3-max")
	require.True(t, ok)
	assert.Equal(t, "qwen3-max-2026-01-23", qwenModel.ID)
	assert.True(t, qwenModel.Supports(ModelCapabilityThinking))
	assert.Equal(t, "thinking_budget", qwenModel.Metadata["thinking_budget_field"])

	geminiModels := catalog.ModelsForProvider("GEMINI")
	require.NotEmpty(t, geminiModels)
	assert.Contains(t, geminiModels[0].InputModalities, "image")
}

func TestDefaultModelDescriptorsReturnClones(t *testing.T) {
	models := DefaultModelDescriptors()
	require.NotEmpty(t, models)
	models[0].Aliases[0] = "mutated"
	models[0].Metadata["length_field"] = "mutated"

	again := DefaultModelDescriptors()
	require.NotEmpty(t, again)
	assert.NotEqual(t, "mutated", again[0].Aliases[0])
	assert.NotEqual(t, "mutated", again[0].Metadata["length_field"])
}

func TestLoadModelCatalogJSON_ObjectSnapshot(t *testing.T) {
	payload := `{
		"verified_at":"2026-05-02",
		"source":"unit-test",
		"models":[{
			"provider":"openai",
			"id":"gpt-test",
			"aliases":["gpt-alias"],
			"stage":"preview",
			"capabilities":["reasoning","tool_calling"],
			"endpoint_families":["openai_responses"],
			"metadata":{"length_field":"max_output_tokens"}
		}]
	}`

	catalog, err := LoadModelCatalogJSON(strings.NewReader(payload))
	require.NoError(t, err)
	model, ok := catalog.Lookup("OPENAI", "gpt-alias")
	require.True(t, ok)
	assert.Equal(t, "gpt-test", model.ID)
	assert.Equal(t, ModelStagePreview, model.Stage)
	assert.True(t, model.Supports(ModelCapabilityReasoning))
	assert.Equal(t, "max_output_tokens", model.Metadata["length_field"])

	model.Metadata["length_field"] = "mutated"
	again, ok := catalog.Lookup("openai", "gpt-test")
	require.True(t, ok)
	assert.Equal(t, "max_output_tokens", again.Metadata["length_field"])
}

func TestLoadModelCatalogJSON_ArraySnapshot(t *testing.T) {
	payload := `[{"provider":"qwen","id":"qwen-test","aliases":["qwen-alias"]}]`

	models, err := LoadModelDescriptorsJSON(strings.NewReader(payload))
	require.NoError(t, err)
	require.Len(t, models, 1)
	assert.Equal(t, "qwen-test", models[0].ID)

	models[0].Aliases[0] = "mutated"
	again, err := LoadModelDescriptorsJSON(strings.NewReader(payload))
	require.NoError(t, err)
	assert.Equal(t, "qwen-alias", again[0].Aliases[0])
}

func TestLoadModelCatalogJSON_RejectsInvalidSnapshot(t *testing.T) {
	_, err := LoadModelCatalogJSON(strings.NewReader(`{"models":[{"provider":"openai"}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id")

	_, err = LoadModelCatalogJSON(strings.NewReader(`{"models":[{"provider":"openai","id":"gpt"},{"provider":"OPENAI","id":"GPT"}]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestRequiredModelCapabilitiesFromExecutionOptions(t *testing.T) {
	thinkingBudget := int32(1024)
	options := ExecutionOptions{
		Model: ModelOptions{
			Provider:         "openai",
			Model:            "gpt-5",
			ResponseFormat:   &ResponseFormat{Type: ResponseFormatJSONSchema},
			ThinkingBudget:   &thinkingBudget,
			WebSearchOptions: &WebSearchOptions{},
		},
		Tools: ToolProtocolOptions{
			AllowedTools: []string{"web_search"},
		},
	}

	requirements := RequiredModelCapabilities(options)
	assert.Contains(t, requirements, ModelCapabilityRequirement{Capability: ModelCapabilityToolCalling, Reason: "tools requested"})
	assert.Contains(t, requirements, ModelCapabilityRequirement{Capability: ModelCapabilityStructuredOutput, Reason: "structured response format requested"})
	assert.Contains(t, requirements, ModelCapabilityRequirement{Capability: ModelCapabilityReasoning, Reason: "reasoning or thinking requested"})
	assert.Contains(t, requirements, ModelCapabilityRequirement{Capability: ModelCapabilityWebSearch, Reason: "web search requested"})
}

func TestValidateModelCapabilitiesRejectsUnsupportedRequestedFeature(t *testing.T) {
	catalog := NewModelCatalog([]ModelDescriptor{{
		Provider:     "openai",
		ID:           "text-only",
		Capabilities: []ModelCapability{ModelCapabilityTextInput, ModelCapabilityTextOutput},
	}})
	options := ExecutionOptions{
		Model: ModelOptions{
			Provider:       "openai",
			Model:          "text-only",
			ResponseFormat: &ResponseFormat{Type: ResponseFormatJSONObject},
		},
	}

	err := ValidateModelCapabilities(catalog, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "structured_output")
}

func TestValidateModelCapabilitiesAllowsSupportedOrUnknownModels(t *testing.T) {
	catalog := NewModelCatalog([]ModelDescriptor{{
		Provider: "openai",
		ID:       "agent-ready",
		Capabilities: []ModelCapability{
			ModelCapabilityToolCalling,
			ModelCapabilityStructuredOutput,
			ModelCapabilityReasoning,
		},
	}})
	options := ExecutionOptions{
		Model: ModelOptions{
			Provider:        "openai",
			Model:           "agent-ready",
			ReasoningEffort: "medium",
			ResponseFormat:  &ResponseFormat{Type: ResponseFormatJSONSchema},
		},
		Tools: ToolProtocolOptions{ToolChoice: &ToolChoice{Mode: ToolChoiceModeAuto}},
	}
	require.NoError(t, ValidateModelCapabilities(catalog, options))

	options.Model.Model = "not-in-catalog"
	require.NoError(t, ValidateModelCapabilities(catalog, options))
}

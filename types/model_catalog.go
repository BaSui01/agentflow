package types

import "strings"

// ModelStage describes lifecycle state for a model entry in a catalog.
type ModelStage string

const (
	ModelStageStable     ModelStage = "stable"
	ModelStagePreview    ModelStage = "preview"
	ModelStageDeprecated ModelStage = "deprecated"
	ModelStageRetired    ModelStage = "retired"
	ModelStageComingSoon ModelStage = "coming_soon"
)

// ModelCapability describes provider-neutral capabilities used by routing,
// validation, and documentation without leaking provider SDK structs upward.
type ModelCapability string

const (
	ModelCapabilityTextInput        ModelCapability = "text_input"
	ModelCapabilityTextOutput       ModelCapability = "text_output"
	ModelCapabilityImageInput       ModelCapability = "image_input"
	ModelCapabilityAudioInput       ModelCapability = "audio_input"
	ModelCapabilityVideoInput       ModelCapability = "video_input"
	ModelCapabilityMultimodalOutput ModelCapability = "multimodal_output"
	ModelCapabilityToolCalling      ModelCapability = "tool_calling"
	ModelCapabilityStructuredOutput ModelCapability = "structured_output"
	ModelCapabilityReasoning        ModelCapability = "reasoning"
	ModelCapabilityThinking         ModelCapability = "thinking"
	ModelCapabilityStreaming        ModelCapability = "streaming"
	ModelCapabilityPromptCaching    ModelCapability = "prompt_caching"
	ModelCapabilityWebSearch        ModelCapability = "web_search"
)

// ModelEndpointFamily names a protocol family supported by a model.
type ModelEndpointFamily string

const (
	ModelEndpointOpenAIChat       ModelEndpointFamily = "openai_chat_completions"
	ModelEndpointOpenAIResponses  ModelEndpointFamily = "openai_responses"
	ModelEndpointAnthropicMessage ModelEndpointFamily = "anthropic_messages"
	ModelEndpointGeminiGenerate   ModelEndpointFamily = "gemini_generate_content"
	ModelEndpointVertexGenerate   ModelEndpointFamily = "vertex_generate_content"
)

// ModelDescriptor records model facts. It is catalog data, not per-request
// runtime options. Request fields belong in ModelOptions and ChatRequest.
type ModelDescriptor struct {
	Provider            string                `json:"provider"`
	ID                  string                `json:"id"`
	Aliases             []string              `json:"aliases,omitempty"`
	Family              string                `json:"family,omitempty"`
	DisplayName         string                `json:"display_name,omitempty"`
	Stage               ModelStage            `json:"stage,omitempty"`
	Default             bool                  `json:"default,omitempty"`
	ContextWindowTokens int                   `json:"context_window_tokens,omitempty"`
	MaxOutputTokens     int                   `json:"max_output_tokens,omitempty"`
	InputModalities     []string              `json:"input_modalities,omitempty"`
	OutputModalities    []string              `json:"output_modalities,omitempty"`
	Capabilities        []ModelCapability     `json:"capabilities,omitempty"`
	EndpointFamilies    []ModelEndpointFamily `json:"endpoint_families,omitempty"`
	ReleaseDate         string                `json:"release_date,omitempty"`
	RetiresAt           string                `json:"retires_at,omitempty"`
	VerifiedAt          string                `json:"verified_at,omitempty"`
	SourceURLs          []string              `json:"source_urls,omitempty"`
	Metadata            map[string]string     `json:"metadata,omitempty"`
}

// ModelCatalog provides provider/id and alias lookup over immutable descriptor
// snapshots supplied by runtime composition or documentation-generated data.
type ModelCatalog struct {
	models []ModelDescriptor
	index  map[string]int
}

// NewModelCatalog returns a lookup catalog for the provided descriptors.
func NewModelCatalog(models []ModelDescriptor) *ModelCatalog {
	catalog := &ModelCatalog{
		models: cloneModelDescriptors(models),
		index:  make(map[string]int),
	}
	for i, model := range catalog.models {
		catalog.addIndex(model.Provider, model.ID, i)
		for _, alias := range model.Aliases {
			catalog.addIndex(model.Provider, alias, i)
		}
	}
	return catalog
}

// Lookup returns a descriptor by provider and model id or alias.
func (c *ModelCatalog) Lookup(provider, model string) (ModelDescriptor, bool) {
	if c == nil || len(c.index) == 0 {
		return ModelDescriptor{}, false
	}
	idx, ok := c.index[modelCatalogKey(provider, model)]
	if !ok {
		return ModelDescriptor{}, false
	}
	return c.models[idx].clone(), true
}

// ModelsForProvider returns descriptors registered for a provider.
func (c *ModelCatalog) ModelsForProvider(provider string) []ModelDescriptor {
	if c == nil {
		return nil
	}
	provider = normalizeModelCatalogPart(provider)
	var out []ModelDescriptor
	for _, model := range c.models {
		if normalizeModelCatalogPart(model.Provider) == provider {
			out = append(out, model.clone())
		}
	}
	return out
}

// All returns all descriptors in catalog order.
func (c *ModelCatalog) All() []ModelDescriptor {
	if c == nil {
		return nil
	}
	return cloneModelDescriptors(c.models)
}

// Supports reports whether the descriptor declares a capability.
func (d ModelDescriptor) Supports(capability ModelCapability) bool {
	for _, value := range d.Capabilities {
		if value == capability {
			return true
		}
	}
	return false
}

func (c *ModelCatalog) addIndex(provider, model string, idx int) {
	key := modelCatalogKey(provider, model)
	if key == "" {
		return
	}
	c.index[key] = idx
}

func modelCatalogKey(provider, model string) string {
	provider = normalizeModelCatalogPart(provider)
	model = normalizeModelCatalogPart(model)
	if provider == "" || model == "" {
		return ""
	}
	return provider + "/" + model
}

func normalizeModelCatalogPart(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cloneModelDescriptors(models []ModelDescriptor) []ModelDescriptor {
	if len(models) == 0 {
		return nil
	}
	out := make([]ModelDescriptor, len(models))
	for i, model := range models {
		out[i] = model.clone()
	}
	return out
}

func (d ModelDescriptor) clone() ModelDescriptor {
	cloned := d
	cloned.Aliases = cloneExecutionStrings(d.Aliases)
	cloned.InputModalities = cloneExecutionStrings(d.InputModalities)
	cloned.OutputModalities = cloneExecutionStrings(d.OutputModalities)
	cloned.Capabilities = cloneModelCapabilities(d.Capabilities)
	cloned.EndpointFamilies = cloneModelEndpointFamilies(d.EndpointFamilies)
	cloned.SourceURLs = cloneExecutionStrings(d.SourceURLs)
	cloned.Metadata = cloneExecutionMetadata(d.Metadata)
	return cloned
}

func cloneModelCapabilities(values []ModelCapability) []ModelCapability {
	if len(values) == 0 {
		return nil
	}
	return append([]ModelCapability(nil), values...)
}

func cloneModelEndpointFamilies(values []ModelEndpointFamily) []ModelEndpointFamily {
	if len(values) == 0 {
		return nil
	}
	return append([]ModelEndpointFamily(nil), values...)
}

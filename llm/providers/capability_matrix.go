package providers

// ProviderCapability declares implemented multimodal capabilities per chat provider.
// Keep this as the single source of truth for docs matrix generation.
type ProviderCapability struct {
	Provider      string
	Image         bool
	Video         bool
	AudioGenerate bool
	AudioSTT      bool
	Embedding     bool
	FineTuning    bool
	Rerank        bool
}

// ChatProviderCapabilityMatrix lists the implemented capabilities for the 13 chat providers.
// Order is stable and used by docs generator.
var ChatProviderCapabilityMatrix = []ProviderCapability{
	{Provider: "OpenAI", Image: true, Video: true, AudioGenerate: true, AudioSTT: true, Embedding: true, FineTuning: true},
	{Provider: "Claude", Image: false, Video: false, AudioGenerate: false, AudioSTT: false, Embedding: false, FineTuning: false},
	{Provider: "Gemini", Image: true, Video: true, AudioGenerate: true, AudioSTT: true, Embedding: true, FineTuning: true},
	{Provider: "DeepSeek", Image: false, Video: false, AudioGenerate: false, AudioSTT: false, Embedding: false, FineTuning: false},
	{Provider: "Qwen", Image: true, Video: true, AudioGenerate: true, AudioSTT: false, Embedding: true, FineTuning: false, Rerank: true},
	{Provider: "GLM", Image: true, Video: true, AudioGenerate: true, AudioSTT: false, Embedding: true, FineTuning: true, Rerank: true},
	{Provider: "Grok", Image: true, Video: true, AudioGenerate: false, AudioSTT: false, Embedding: true, FineTuning: false},
	{Provider: "Doubao", Image: true, Video: false, AudioGenerate: true, AudioSTT: false, Embedding: true, FineTuning: false},
	{Provider: "Kimi", Image: false, Video: false, AudioGenerate: false, AudioSTT: false, Embedding: false, FineTuning: false},
	{Provider: "Mistral", Image: false, Video: false, AudioGenerate: false, AudioSTT: true, Embedding: true, FineTuning: true},
	{Provider: "Hunyuan", Image: false, Video: false, AudioGenerate: false, AudioSTT: false, Embedding: false, FineTuning: false},
	{Provider: "MiniMax", Image: false, Video: false, AudioGenerate: true, AudioSTT: false, Embedding: false, FineTuning: false},
	{Provider: "Llama", Image: false, Video: false, AudioGenerate: false, AudioSTT: false, Embedding: false, FineTuning: false},
}

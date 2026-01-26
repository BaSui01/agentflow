package llm

// DEPRECATED: This file contains the legacy Router implementation.
// Use MultiProviderRouter (router_multi_provider.go) instead for multi-provider support.
//
// The legacy Router was designed for a single-provider-per-model architecture,
// which is incompatible with the new multi-provider data model where one model
// can be provided by multiple providers.
//
// Migration guide:
// - Replace NewRouter() with NewMultiProviderRouter()
// - Use SelectProviderWithModel() instead of SelectProvider()
// - API Key pools are now managed per provider

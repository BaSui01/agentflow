package main

//lint:file-ignore SA4017 Reachability integration calls are gated and intentionally side-effect free.
//lint:file-ignore SA1012 Reachability integration calls are gated and intentionally side-effect free.

// Code generated from .snow/deadcode_latest.txt for module integration reachability.
// 保留并集成：将已决策保留模块统一挂接到可执行主链，避免悬空实现。

import (
	"context"
	llm_capabilities_audio "github.com/BaSui01/agentflow/llm/capabilities/audio"
	llm_capabilities_embedding "github.com/BaSui01/agentflow/llm/capabilities/embedding"
	llm_capabilities_image "github.com/BaSui01/agentflow/llm/capabilities/image"
	llm_capabilities_moderation "github.com/BaSui01/agentflow/llm/capabilities/moderation"
	llm_capabilities_multimodal "github.com/BaSui01/agentflow/llm/capabilities/multimodal"
	llm_capabilities_music "github.com/BaSui01/agentflow/llm/capabilities/music"
	llm_capabilities_rerank "github.com/BaSui01/agentflow/llm/capabilities/rerank"
	llm_capabilities_threed "github.com/BaSui01/agentflow/llm/capabilities/threed"
	llm_capabilities_tools "github.com/BaSui01/agentflow/llm/capabilities/tools"
	llm_capabilities_video "github.com/BaSui01/agentflow/llm/capabilities/video"
	llm_circuitbreaker "github.com/BaSui01/agentflow/llm/circuitbreaker"
	llm_config "github.com/BaSui01/agentflow/llm/config"
	llm_gateway "github.com/BaSui01/agentflow/llm/gateway"
	llm_idempotency "github.com/BaSui01/agentflow/llm/idempotency"
	llm_middleware "github.com/BaSui01/agentflow/llm/middleware"
	llm_observability "github.com/BaSui01/agentflow/llm/observability"
	llm_providers_base "github.com/BaSui01/agentflow/llm/providers/base"
	llm_providers_openai "github.com/BaSui01/agentflow/llm/providers/openai"
	llm_providers_vendor "github.com/BaSui01/agentflow/llm/providers/vendor"
	llm_runtime_policy "github.com/BaSui01/agentflow/llm/runtime/policy"
	llm_runtime_router "github.com/BaSui01/agentflow/llm/runtime/router"
	llm_streaming "github.com/BaSui01/agentflow/llm/streaming"
	llm_tokenizer "github.com/BaSui01/agentflow/llm/tokenizer"
	pkg_cache "github.com/BaSui01/agentflow/pkg/cache"
	pkg_database "github.com/BaSui01/agentflow/pkg/database"
	pkg_metrics "github.com/BaSui01/agentflow/pkg/metrics"
	pkg_middleware "github.com/BaSui01/agentflow/pkg/middleware"
	pkg_migration "github.com/BaSui01/agentflow/pkg/migration"
	pkg_mongodb "github.com/BaSui01/agentflow/pkg/mongodb"
	pkg_openapi "github.com/BaSui01/agentflow/pkg/openapi"
	pkg_server "github.com/BaSui01/agentflow/pkg/server"
	pkg_service "github.com/BaSui01/agentflow/pkg/service"
	pkg_telemetry "github.com/BaSui01/agentflow/pkg/telemetry"
	rag "github.com/BaSui01/agentflow/rag"
	rag_core "github.com/BaSui01/agentflow/rag/core"
	rag_loader "github.com/BaSui01/agentflow/rag/loader"
	rag_retrieval "github.com/BaSui01/agentflow/rag/retrieval"
	rag_sources "github.com/BaSui01/agentflow/rag/sources"
	testutil "github.com/BaSui01/agentflow/testutil"
	testutil_mocks "github.com/BaSui01/agentflow/testutil/mocks"
	af_types "github.com/BaSui01/agentflow/types"
	"os"
	"time"
)

func demoFullModuleIntegrationReachability() {
	// type-based method references
	var ref_llm_capabilities_audio_DeepgramProvider llm_capabilities_audio.DeepgramProvider
	var ref_llm_capabilities_image_FluxProvider llm_capabilities_image.FluxProvider
	var ref_llm_capabilities_moderation_OpenAIProvider llm_capabilities_moderation.OpenAIProvider
	var ref_llm_capabilities_multimodal_MultimodalProvider llm_capabilities_multimodal.MultimodalProvider
	var ref_llm_capabilities_multimodal_Processor llm_capabilities_multimodal.Processor
	var ref_llm_capabilities_music_MiniMaxProvider llm_capabilities_music.MiniMaxProvider
	var ref_llm_capabilities_music_SunoProvider llm_capabilities_music.SunoProvider
	var ref_llm_capabilities_threed_MeshyProvider llm_capabilities_threed.MeshyProvider
	var ref_llm_capabilities_threed_TripoProvider llm_capabilities_threed.TripoProvider
	var ref_llm_capabilities_tools_BatchExecutor llm_capabilities_tools.BatchExecutor
	var ref_llm_capabilities_tools_DefaultAuditLogger llm_capabilities_tools.DefaultAuditLogger
	var ref_llm_capabilities_tools_DefaultCostController llm_capabilities_tools.DefaultCostController
	var ref_llm_capabilities_tools_DefaultExecutor llm_capabilities_tools.DefaultExecutor
	var ref_llm_capabilities_tools_DefaultPermissionManager llm_capabilities_tools.DefaultPermissionManager
	var ref_llm_capabilities_tools_DefaultRateLimitManager llm_capabilities_tools.DefaultRateLimitManager
	var ref_llm_capabilities_tools_DuckDuckGoSearchProvider llm_capabilities_tools.DuckDuckGoSearchProvider
	var ref_llm_capabilities_tools_FileAuditBackend llm_capabilities_tools.FileAuditBackend
	var ref_llm_capabilities_tools_FirecrawlProvider llm_capabilities_tools.FirecrawlProvider
	var ref_llm_capabilities_tools_FixedWindowLimiter llm_capabilities_tools.FixedWindowLimiter
	var ref_llm_capabilities_tools_HTTPScrapeProvider llm_capabilities_tools.HTTPScrapeProvider
	var ref_llm_capabilities_tools_JinaScraperProvider llm_capabilities_tools.JinaScraperProvider
	var ref_llm_capabilities_tools_MemoryAuditBackend llm_capabilities_tools.MemoryAuditBackend
	var ref_llm_capabilities_tools_ParallelExecutor llm_capabilities_tools.ParallelExecutor
	var ref_llm_capabilities_tools_ReActExecutor llm_capabilities_tools.ReActExecutor
	var ref_llm_capabilities_tools_ResilientExecutor llm_capabilities_tools.ResilientExecutor
	var ref_llm_capabilities_tools_SearXNGSearchProvider llm_capabilities_tools.SearXNGSearchProvider
	var ref_llm_capabilities_tools_SlidingWindowLimiter llm_capabilities_tools.SlidingWindowLimiter
	var ref_llm_capabilities_tools_TavilySearchProvider llm_capabilities_tools.TavilySearchProvider
	var ref_llm_capabilities_tools_TokenBucketLimiter llm_capabilities_tools.TokenBucketLimiter
	var ref_llm_capabilities_tools_ToolCallChain llm_capabilities_tools.ToolCallChain
	var ref_llm_capabilities_video_GeminiProvider llm_capabilities_video.GeminiProvider
	var ref_llm_config_LLMConfig llm_config.LLMConfig
	var ref_llm_config_PolicyManager llm_config.PolicyManager
	var ref_llm_gateway_StateMachine llm_gateway.StateMachine
	var ref_llm_middleware_Chain llm_middleware.Chain
	var ref_llm_observability_ConversationTracer llm_observability.ConversationTracer
	var ref_llm_observability_CostTracker llm_observability.CostTracker
	var ref_llm_observability_Tracer llm_observability.Tracer
	var ref_llm_providers_base_BaseCapabilityProvider llm_providers_base.BaseCapabilityProvider
	var ref_llm_providers_vendor_Profile llm_providers_vendor.Profile
	var ref_llm_runtime_policy_RetryableError llm_runtime_policy.RetryableError
	var ref_llm_runtime_router_ABMetrics llm_runtime_router.ABMetrics
	var ref_llm_runtime_router_ABRouter llm_runtime_router.ABRouter
	var ref_llm_runtime_router_APIKeyPool llm_runtime_router.APIKeyPool
	var ref_llm_runtime_router_DefaultProviderFactory llm_runtime_router.DefaultProviderFactory
	var ref_llm_runtime_router_HealthChecker llm_runtime_router.HealthChecker
	var ref_llm_runtime_router_HealthMonitor llm_runtime_router.HealthMonitor
	var ref_llm_runtime_router_MultiProviderRouter llm_runtime_router.MultiProviderRouter
	var ref_llm_runtime_router_PrefixRouter llm_runtime_router.PrefixRouter
	var ref_llm_runtime_router_SemanticRouter llm_runtime_router.SemanticRouter
	var ref_llm_runtime_router_WeightedRouter llm_runtime_router.WeightedRouter
	var ref_llm_streaming_BackpressureStream llm_streaming.BackpressureStream
	var ref_llm_streaming_ChunkReader llm_streaming.ChunkReader
	var ref_llm_streaming_DropPolicy llm_streaming.DropPolicy
	var ref_llm_streaming_RateLimiter llm_streaming.RateLimiter
	var ref_llm_streaming_RingBuffer llm_streaming.RingBuffer
	var ref_llm_streaming_StreamMultiplexer llm_streaming.StreamMultiplexer
	var ref_llm_streaming_StringView llm_streaming.StringView
	var ref_llm_streaming_ZeroCopyBuffer llm_streaming.ZeroCopyBuffer
	var ref_llm_tokenizer_TiktokenTokenizer llm_tokenizer.TiktokenTokenizer
	var ref_pkg_cache_Manager pkg_cache.Manager
	var ref_pkg_database_PoolManager pkg_database.PoolManager
	var ref_pkg_metrics_Collector pkg_metrics.Collector
	var ref_pkg_migration_CLI pkg_migration.CLI
	var ref_pkg_openapi_Generator pkg_openapi.Generator
	var ref_pkg_server_Manager pkg_server.Manager
	var ref_pkg_service_Registry pkg_service.Registry
	var ref_rag_core_RAGError rag_core.RAGError
	var ref_rag_loader_ArxivSourceAdapter rag_loader.ArxivSourceAdapter
	var ref_rag_loader_CSVLoader rag_loader.CSVLoader
	var ref_rag_loader_GitHubSourceAdapter rag_loader.GitHubSourceAdapter
	var ref_rag_loader_JSONLoader rag_loader.JSONLoader
	var ref_rag_loader_LoaderRegistry rag_loader.LoaderRegistry
	var ref_rag_loader_MarkdownLoader rag_loader.MarkdownLoader
	var ref_rag_loader_TextLoader rag_loader.TextLoader
	var ref_rag_retrieval_Pipeline rag_retrieval.Pipeline
	var ref_rag_retrieval_StrategyRegistry rag_retrieval.StrategyRegistry
	var ref_rag_sources_ArxivSource rag_sources.ArxivSource
	var ref_rag_sources_GitHubSource rag_sources.GitHubSource
	var ref_rag_ContextualRetrieval rag.ContextualRetrieval
	var ref_rag_CrossEncoderReranker rag.CrossEncoderReranker
	var ref_rag_DocumentChunker rag.DocumentChunker
	var ref_rag_EnhancedTokenizer rag.EnhancedTokenizer
	var ref_rag_GraphRAG rag.GraphRAG
	var ref_rag_HNSWIndex rag.HNSWIndex
	var ref_rag_KnowledgeGraph rag.KnowledgeGraph
	var ref_rag_LLMContextProvider rag.LLMContextProvider
	var ref_rag_LLMReranker rag.LLMReranker
	var ref_rag_LLMTokenizerAdapter rag.LLMTokenizerAdapter
	var ref_rag_MultiHopReasoner rag.MultiHopReasoner
	var ref_rag_QueryRouter rag.QueryRouter
	var ref_rag_QueryTransformer rag.QueryTransformer
	var ref_rag_RoutingDecision rag.RoutingDecision
	var ref_rag_SemanticCache rag.SemanticCache
	var ref_rag_SimpleContextProvider rag.SimpleContextProvider
	var ref_rag_SimpleGraphEmbedder rag.SimpleGraphEmbedder
	var ref_rag_SimpleReranker rag.SimpleReranker
	var ref_rag_SimpleTokenizer rag.SimpleTokenizer
	var ref_rag_TransformedQuery rag.TransformedQuery
	var ref_rag_WebRetriever rag.WebRetriever

	// exported method references
	_ = ref_llm_capabilities_audio_DeepgramProvider.Name
	_ = ref_llm_capabilities_audio_DeepgramProvider.SupportedFormats
	_ = ref_llm_capabilities_audio_DeepgramProvider.Transcribe
	_ = ref_llm_capabilities_audio_DeepgramProvider.TranscribeFile
	_ = ref_llm_capabilities_image_FluxProvider.CreateVariation
	_ = ref_llm_capabilities_image_FluxProvider.Edit
	_ = ref_llm_capabilities_image_FluxProvider.Generate
	_ = ref_llm_capabilities_image_FluxProvider.Name
	_ = ref_llm_capabilities_image_FluxProvider.SupportedSizes
	_ = ref_llm_capabilities_moderation_OpenAIProvider.Moderate
	_ = ref_llm_capabilities_moderation_OpenAIProvider.Name
	_ = ref_llm_capabilities_multimodal_MultimodalProvider.Completion
	_ = ref_llm_capabilities_multimodal_MultimodalProvider.Name
	_ = ref_llm_capabilities_multimodal_MultimodalProvider.Stream
	_ = ref_llm_capabilities_multimodal_MultimodalProvider.SupportedModalities
	_ = ref_llm_capabilities_multimodal_MultimodalProvider.SupportsMultimodal
	_ = ref_llm_capabilities_multimodal_Processor.ConvertToProviderFormat
	_ = ref_llm_capabilities_music_MiniMaxProvider.Generate
	_ = ref_llm_capabilities_music_MiniMaxProvider.Name
	_ = ref_llm_capabilities_music_SunoProvider.Generate
	_ = ref_llm_capabilities_music_SunoProvider.Name
	_ = ref_llm_capabilities_threed_MeshyProvider.Generate
	_ = ref_llm_capabilities_threed_MeshyProvider.Name
	_ = ref_llm_capabilities_threed_TripoProvider.Generate
	_ = ref_llm_capabilities_threed_TripoProvider.Name
	_ = ref_llm_capabilities_tools_BatchExecutor.ExecuteBatched
	_ = ref_llm_capabilities_tools_DefaultAuditLogger.Log
	_ = ref_llm_capabilities_tools_DefaultAuditLogger.LogAsync
	_ = ref_llm_capabilities_tools_DefaultAuditLogger.Query
	_ = ref_llm_capabilities_tools_DefaultCostController.AddBudget
	_ = ref_llm_capabilities_tools_DefaultCostController.CalculateCost
	_ = ref_llm_capabilities_tools_DefaultCostController.CheckBudget
	_ = ref_llm_capabilities_tools_DefaultCostController.GetBudget
	_ = ref_llm_capabilities_tools_DefaultCostController.GetCostReport
	_ = ref_llm_capabilities_tools_DefaultCostController.GetOptimizations
	_ = ref_llm_capabilities_tools_DefaultCostController.GetToolCost
	_ = ref_llm_capabilities_tools_DefaultCostController.GetUsage
	_ = ref_llm_capabilities_tools_DefaultCostController.ListBudgets
	_ = ref_llm_capabilities_tools_DefaultCostController.RecordCost
	_ = ref_llm_capabilities_tools_DefaultCostController.RemoveBudget
	_ = ref_llm_capabilities_tools_DefaultCostController.SetAlertHandler
	_ = ref_llm_capabilities_tools_DefaultCostController.SetTokenCounter
	_ = ref_llm_capabilities_tools_DefaultCostController.SetToolCost
	_ = ref_llm_capabilities_tools_DefaultExecutor.ExecuteOneStream
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.AddRole
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.AddRule
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.AssignRoleToUser
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.CheckPermission
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.GetAgentPermission
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.GetRole
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.GetRule
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.GetUserRoles
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.ListRules
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.RemoveRole
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.RemoveRule
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.SetAgentPermission
	_ = ref_llm_capabilities_tools_DefaultPermissionManager.SetApprovalHandler
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.AddRule
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.CheckRateLimit
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.GetRule
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.GetStats
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.ListRules
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.RemoveRule
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.Reset
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.SetDegradeHandler
	_ = ref_llm_capabilities_tools_DefaultRateLimitManager.SetQueueHandler
	_ = ref_llm_capabilities_tools_DuckDuckGoSearchProvider.Name
	_ = ref_llm_capabilities_tools_DuckDuckGoSearchProvider.Search
	_ = ref_llm_capabilities_tools_FileAuditBackend.Close
	_ = ref_llm_capabilities_tools_FileAuditBackend.Query
	_ = ref_llm_capabilities_tools_FileAuditBackend.Write
	_ = ref_llm_capabilities_tools_FirecrawlProvider.Name
	_ = ref_llm_capabilities_tools_FirecrawlProvider.Scrape
	_ = ref_llm_capabilities_tools_FirecrawlProvider.Search
	_ = ref_llm_capabilities_tools_FixedWindowLimiter.Allow
	_ = ref_llm_capabilities_tools_FixedWindowLimiter.Remaining
	_ = ref_llm_capabilities_tools_FixedWindowLimiter.Reset
	_ = ref_llm_capabilities_tools_FixedWindowLimiter.ResetAt
	_ = ref_llm_capabilities_tools_HTTPScrapeProvider.Name
	_ = ref_llm_capabilities_tools_HTTPScrapeProvider.Scrape
	_ = ref_llm_capabilities_tools_JinaScraperProvider.Name
	_ = ref_llm_capabilities_tools_JinaScraperProvider.Scrape
	_ = ref_llm_capabilities_tools_MemoryAuditBackend.Close
	_ = ref_llm_capabilities_tools_MemoryAuditBackend.Query
	_ = ref_llm_capabilities_tools_MemoryAuditBackend.Write
	_ = ref_llm_capabilities_tools_ParallelExecutor.Execute
	_ = ref_llm_capabilities_tools_ParallelExecutor.ExecuteWithDependencies
	_ = ref_llm_capabilities_tools_ParallelExecutor.Stats
	_ = ref_llm_capabilities_tools_ReActExecutor.ExecuteWithTrace
	_ = ref_llm_capabilities_tools_ResilientExecutor.Execute
	_ = ref_llm_capabilities_tools_ResilientExecutor.ExecuteOne
	_ = ref_llm_capabilities_tools_SearXNGSearchProvider.Name
	_ = ref_llm_capabilities_tools_SearXNGSearchProvider.Search
	_ = ref_llm_capabilities_tools_SlidingWindowLimiter.Allow
	_ = ref_llm_capabilities_tools_SlidingWindowLimiter.Remaining
	_ = ref_llm_capabilities_tools_SlidingWindowLimiter.Reset
	_ = ref_llm_capabilities_tools_SlidingWindowLimiter.ResetAt
	_ = ref_llm_capabilities_tools_TavilySearchProvider.Name
	_ = ref_llm_capabilities_tools_TavilySearchProvider.Search
	_ = ref_llm_capabilities_tools_TokenBucketLimiter.Allow
	_ = ref_llm_capabilities_tools_TokenBucketLimiter.Remaining
	_ = ref_llm_capabilities_tools_TokenBucketLimiter.Reset
	_ = ref_llm_capabilities_tools_TokenBucketLimiter.ResetAt
	_ = ref_llm_capabilities_tools_ToolCallChain.ExecuteChain
	_ = ref_llm_capabilities_video_GeminiProvider.Analyze
	_ = ref_llm_capabilities_video_GeminiProvider.Generate
	_ = ref_llm_capabilities_video_GeminiProvider.Name
	_ = ref_llm_capabilities_video_GeminiProvider.SupportedFormats
	_ = ref_llm_capabilities_video_GeminiProvider.SupportsGeneration
	_ = ref_llm_config_LLMConfig.Validate
	_ = ref_llm_config_PolicyManager.FindPolicy
	_ = ref_llm_config_PolicyManager.GetFallbackAction
	_ = ref_llm_config_PolicyManager.GetFallbackChain
	_ = ref_llm_config_PolicyManager.GetRetryDelay
	_ = ref_llm_config_PolicyManager.ShouldRetry
	_ = ref_llm_config_PolicyManager.Update
	_ = ref_llm_gateway_StateMachine.Current
	_ = ref_llm_gateway_StateMachine.Transition
	_ = ref_llm_middleware_Chain.Len
	_ = ref_llm_observability_ConversationTracer.EndConversation
	_ = ref_llm_observability_ConversationTracer.ExportJSON
	_ = ref_llm_observability_ConversationTracer.GetConversation
	_ = ref_llm_observability_ConversationTracer.TraceTurn
	_ = ref_llm_observability_CostTracker.Record
	_ = ref_llm_observability_CostTracker.Reset
	_ = ref_llm_observability_CostTracker.TotalCost
	_ = ref_llm_observability_Tracer.AddFeedback
	_ = ref_llm_observability_Tracer.GetRun
	_ = ref_llm_observability_Tracer.GetTrace
	_ = ref_llm_observability_Tracer.TraceLLMCall
	_ = ref_llm_observability_Tracer.TraceToolCall
	_ = ref_llm_providers_base_BaseCapabilityProvider.DoRaw
	_ = ref_llm_providers_base_BaseCapabilityProvider.GetJSON
	_ = ref_llm_providers_base_BaseCapabilityProvider.GetJSONDecode
	_ = ref_llm_providers_base_BaseCapabilityProvider.PostJSON
	_ = ref_llm_providers_base_BaseCapabilityProvider.PostJSONDecode
	_ = ref_llm_providers_vendor_Profile.ModelForLanguage
	_ = ref_llm_runtime_policy_RetryableError.Error
	_ = ref_llm_runtime_policy_RetryableError.Unwrap
	_ = ref_llm_runtime_router_ABMetrics.GetAvgLatencyMs
	_ = ref_llm_runtime_router_ABMetrics.GetAvgQualityScore
	_ = ref_llm_runtime_router_ABMetrics.GetSuccessRate
	_ = ref_llm_runtime_router_ABMetrics.RecordRequest
	_ = ref_llm_runtime_router_ABRouter.Completion
	_ = ref_llm_runtime_router_ABRouter.Endpoints
	_ = ref_llm_runtime_router_ABRouter.GetMetrics
	_ = ref_llm_runtime_router_ABRouter.GetReport
	_ = ref_llm_runtime_router_ABRouter.HealthCheck
	_ = ref_llm_runtime_router_ABRouter.ListModels
	_ = ref_llm_runtime_router_ABRouter.Name
	_ = ref_llm_runtime_router_ABRouter.Stream
	_ = ref_llm_runtime_router_ABRouter.SupportsNativeFunctionCalling
	_ = ref_llm_runtime_router_ABRouter.UpdateWeights
	_ = ref_llm_runtime_router_APIKeyPool.GetStats
	_ = ref_llm_runtime_router_APIKeyPool.LoadKeys
	_ = ref_llm_runtime_router_APIKeyPool.RecordFailure
	_ = ref_llm_runtime_router_APIKeyPool.RecordSuccess
	_ = ref_llm_runtime_router_APIKeyPool.SelectKey
	_ = ref_llm_runtime_router_DefaultProviderFactory.CreateProvider
	_ = ref_llm_runtime_router_DefaultProviderFactory.RegisterProvider
	_ = ref_llm_runtime_router_HealthChecker.Start
	_ = ref_llm_runtime_router_HealthChecker.Stop
	_ = ref_llm_runtime_router_HealthMonitor.ForceHealthCheck
	_ = ref_llm_runtime_router_HealthMonitor.GetAllProviderStats
	_ = ref_llm_runtime_router_HealthMonitor.GetCurrentQPS
	_ = ref_llm_runtime_router_HealthMonitor.GetHealthScore
	_ = ref_llm_runtime_router_HealthMonitor.IncrementQPS
	_ = ref_llm_runtime_router_HealthMonitor.SetMaxQPS
	_ = ref_llm_runtime_router_HealthMonitor.Stop
	_ = ref_llm_runtime_router_HealthMonitor.UpdateProbe
	_ = ref_llm_runtime_router_MultiProviderRouter.GetAPIKeyPool
	_ = ref_llm_runtime_router_MultiProviderRouter.GetAPIKeyStats
	_ = ref_llm_runtime_router_MultiProviderRouter.InitAPIKeyPools
	_ = ref_llm_runtime_router_MultiProviderRouter.RecordAPIKeyUsage
	_ = ref_llm_runtime_router_MultiProviderRouter.SelectAPIKey
	_ = ref_llm_runtime_router_MultiProviderRouter.SelectProviderWithModel
	_ = ref_llm_runtime_router_MultiProviderRouter.Stop
	_ = ref_llm_runtime_router_PrefixRouter.GetRules
	_ = ref_llm_runtime_router_PrefixRouter.RouteByModelID
	_ = ref_llm_runtime_router_SemanticRouter.AddProvider
	_ = ref_llm_runtime_router_SemanticRouter.AddRoute
	_ = ref_llm_runtime_router_SemanticRouter.ClassifyIntent
	_ = ref_llm_runtime_router_SemanticRouter.Route
	_ = ref_llm_runtime_router_WeightedRouter.GetCandidates
	_ = ref_llm_runtime_router_WeightedRouter.LoadCandidates
	_ = ref_llm_runtime_router_WeightedRouter.Select
	_ = ref_llm_runtime_router_WeightedRouter.UpdateHealth
	_ = ref_llm_runtime_router_WeightedRouter.UpdateWeights
	_ = ref_llm_streaming_BackpressureStream.BufferLevel
	_ = ref_llm_streaming_BackpressureStream.Close
	_ = ref_llm_streaming_BackpressureStream.IsPaused
	_ = ref_llm_streaming_BackpressureStream.Read
	_ = ref_llm_streaming_BackpressureStream.ReadChan
	_ = ref_llm_streaming_BackpressureStream.Stats
	_ = ref_llm_streaming_BackpressureStream.Write
	_ = ref_llm_streaming_ChunkReader.Next
	_ = ref_llm_streaming_ChunkReader.Reset
	_ = ref_llm_streaming_DropPolicy.String
	_ = ref_llm_streaming_RateLimiter.Allow
	_ = ref_llm_streaming_RateLimiter.Wait
	_ = ref_llm_streaming_RingBuffer.Available
	_ = ref_llm_streaming_RingBuffer.Free
	_ = ref_llm_streaming_RingBuffer.Get
	_ = ref_llm_streaming_RingBuffer.Put
	_ = ref_llm_streaming_StreamMultiplexer.AddConsumer
	_ = ref_llm_streaming_StreamMultiplexer.Start
	_ = ref_llm_streaming_StringView.Bytes
	_ = ref_llm_streaming_StringView.Len
	_ = ref_llm_streaming_StringView.String
	_ = ref_llm_streaming_ZeroCopyBuffer.Bytes
	_ = ref_llm_streaming_ZeroCopyBuffer.BytesUnsafe
	_ = ref_llm_streaming_ZeroCopyBuffer.Len
	_ = ref_llm_streaming_ZeroCopyBuffer.Read
	_ = ref_llm_streaming_ZeroCopyBuffer.Reset
	_ = ref_llm_streaming_ZeroCopyBuffer.Write
	_ = ref_llm_tokenizer_TiktokenTokenizer.CountMessages
	_ = ref_llm_tokenizer_TiktokenTokenizer.CountTokens
	_ = ref_llm_tokenizer_TiktokenTokenizer.Decode
	_ = ref_llm_tokenizer_TiktokenTokenizer.Encode
	_ = ref_llm_tokenizer_TiktokenTokenizer.MaxTokens
	_ = ref_llm_tokenizer_TiktokenTokenizer.Name
	_ = ref_pkg_cache_Manager.Exists
	_ = ref_pkg_cache_Manager.Expire
	_ = ref_pkg_database_PoolManager.DB
	_ = ref_pkg_database_PoolManager.WithTransaction
	_ = ref_pkg_database_PoolManager.WithTransactionRetry
	_ = ref_pkg_metrics_Collector.RecordAgentExecution
	_ = ref_pkg_metrics_Collector.RecordAgentInfo
	_ = ref_pkg_metrics_Collector.RecordAgentStateTransition
	_ = ref_pkg_metrics_Collector.RecordCacheEviction
	_ = ref_pkg_metrics_Collector.RecordCacheHit
	_ = ref_pkg_metrics_Collector.RecordCacheMiss
	_ = ref_pkg_metrics_Collector.RecordCacheSize
	_ = ref_pkg_metrics_Collector.RecordDBConnections
	_ = ref_pkg_metrics_Collector.RecordDBQuery
	_ = ref_pkg_metrics_Collector.RecordLLMRequest
	_ = ref_pkg_migration_CLI.RunInfo
	_ = ref_pkg_migration_CLI.RunSteps
	_ = ref_pkg_migration_CLI.SetOutput
	_ = ref_pkg_openapi_Generator.LoadSpec
	_ = ref_pkg_server_Manager.Addr
	_ = ref_pkg_server_Manager.Errors
	_ = ref_pkg_server_Manager.IsRunning
	_ = ref_pkg_server_Manager.StartTLS
	_ = ref_pkg_service_Registry.HealthChecks
	_ = ref_rag_ContextualRetrieval.CleanExpiredCache
	_ = ref_rag_ContextualRetrieval.IndexDocumentsWithContext
	_ = ref_rag_ContextualRetrieval.Retrieve
	_ = ref_rag_ContextualRetrieval.UpdateIDFStats
	_ = ref_rag_core_RAGError.Error
	_ = ref_rag_core_RAGError.Unwrap
	_ = ref_rag_CrossEncoderReranker.Rerank
	_ = ref_rag_DocumentChunker.ChunkDocument
	_ = ref_rag_EnhancedTokenizer.CountTokens
	_ = ref_rag_EnhancedTokenizer.Encode
	_ = ref_rag_GraphRAG.AddDocument
	_ = ref_rag_GraphRAG.Retrieve
	_ = ref_rag_HNSWIndex.Add
	_ = ref_rag_HNSWIndex.Build
	_ = ref_rag_HNSWIndex.Delete
	_ = ref_rag_HNSWIndex.Search
	_ = ref_rag_HNSWIndex.Size
	_ = ref_rag_KnowledgeGraph.GetNode
	_ = ref_rag_KnowledgeGraph.QueryByType
	_ = ref_rag_LLMContextProvider.GenerateContext
	_ = ref_rag_LLMReranker.Rerank
	_ = ref_rag_LLMTokenizerAdapter.CountTokens
	_ = ref_rag_LLMTokenizerAdapter.Encode
	_ = ref_rag_loader_ArxivSourceAdapter.Load
	_ = ref_rag_loader_ArxivSourceAdapter.SupportedTypes
	_ = ref_rag_loader_CSVLoader.Load
	_ = ref_rag_loader_CSVLoader.SupportedTypes
	_ = ref_rag_loader_GitHubSourceAdapter.Load
	_ = ref_rag_loader_GitHubSourceAdapter.SupportedTypes
	_ = ref_rag_loader_JSONLoader.Load
	_ = ref_rag_loader_JSONLoader.SupportedTypes
	_ = ref_rag_loader_LoaderRegistry.Load
	_ = ref_rag_loader_LoaderRegistry.Register
	_ = ref_rag_loader_LoaderRegistry.SupportedTypes
	_ = ref_rag_loader_MarkdownLoader.Load
	_ = ref_rag_loader_MarkdownLoader.SupportedTypes
	_ = ref_rag_loader_TextLoader.Load
	_ = ref_rag_loader_TextLoader.SupportedTypes
	_ = ref_rag_MultiHopReasoner.Reason
	_ = ref_rag_MultiHopReasoner.ReasonBatch
	_ = ref_rag_QueryRouter.GetStrategyStats
	_ = ref_rag_QueryRouter.RecordFeedback
	_ = ref_rag_QueryRouter.Route
	_ = ref_rag_QueryRouter.RouteBatch
	_ = ref_rag_QueryRouter.RouteMulti
	_ = ref_rag_QueryTransformer.Expand
	_ = ref_rag_QueryTransformer.ExpandWithMetadata
	_ = ref_rag_QueryTransformer.Transform
	_ = ref_rag_QueryTransformer.TransformBatch
	_ = ref_rag_retrieval_Pipeline.Execute
	_ = ref_rag_retrieval_StrategyRegistry.Build
	_ = ref_rag_retrieval_StrategyRegistry.List
	_ = ref_rag_retrieval_StrategyRegistry.Register
	_ = ref_rag_RoutingDecision.FromJSON
	_ = ref_rag_RoutingDecision.ToJSON
	_ = ref_rag_SemanticCache.Clear
	_ = ref_rag_SimpleContextProvider.GenerateContext
	_ = ref_rag_SimpleGraphEmbedder.Embed
	_ = ref_rag_SimpleReranker.Rerank
	_ = ref_rag_SimpleTokenizer.CountTokens
	_ = ref_rag_SimpleTokenizer.Encode
	_ = ref_rag_sources_ArxivSource.Name
	_ = ref_rag_sources_ArxivSource.Search
	_ = ref_rag_sources_ArxivSource.ToJSON
	_ = ref_rag_sources_GitHubSource.GetReadme
	_ = ref_rag_sources_GitHubSource.Name
	_ = ref_rag_sources_GitHubSource.SearchCode
	_ = ref_rag_sources_GitHubSource.SearchRepos
	_ = ref_rag_TransformedQuery.FromJSON
	_ = ref_rag_TransformedQuery.ToJSON
	_ = ref_rag_WebRetriever.Retrieve

	// exported function references
	_ = llm_capabilities_audio.DefaultDeepgramConfig
	_ = llm_capabilities_audio.DefaultElevenLabsConfig
	_ = llm_capabilities_audio.DefaultOpenAISTTConfig
	_ = llm_capabilities_audio.DefaultOpenAITTSConfig
	_ = llm_capabilities_audio.NewDeepgramProvider
	_ = llm_capabilities_embedding.DefaultCohereConfig
	_ = llm_capabilities_embedding.DefaultGeminiConfig
	_ = llm_capabilities_embedding.DefaultJinaConfig
	_ = llm_capabilities_embedding.DefaultOpenAIConfig
	_ = llm_capabilities_embedding.DefaultVoyageConfig
	_ = llm_capabilities_image.DefaultFluxConfig
	_ = llm_capabilities_image.DefaultGeminiConfig
	_ = llm_capabilities_image.DefaultImagen4Config
	_ = llm_capabilities_image.DefaultOpenAIConfig
	_ = llm_capabilities_image.DefaultStabilityConfig
	_ = llm_capabilities_image.NewFluxProvider
	_ = llm_capabilities_image.NewProviderFromConfig
	_ = llm_capabilities_moderation.DefaultOpenAIConfig
	_ = llm_capabilities_moderation.NewOpenAIProvider
	_ = llm_capabilities_multimodal.DefaultAudioConfig
	_ = llm_capabilities_multimodal.DefaultProcessor
	_ = llm_capabilities_multimodal.DefaultVisionConfig
	_ = llm_capabilities_multimodal.LoadAudioFromFile
	_ = llm_capabilities_multimodal.LoadImageFromFile
	_ = llm_capabilities_multimodal.LoadImageFromURL
	_ = llm_capabilities_multimodal.NewAudioBase64Content
	_ = llm_capabilities_multimodal.NewAudioURLContent
	_ = llm_capabilities_multimodal.NewImageBase64Content
	_ = llm_capabilities_multimodal.NewImageURLContent
	_ = llm_capabilities_multimodal.NewMultimodalProvider
	_ = llm_capabilities_multimodal.NewProcessor
	_ = llm_capabilities_multimodal.NewTextContent
	_ = llm_capabilities_music.DefaultMiniMaxMusicConfig
	_ = llm_capabilities_music.DefaultSunoConfig
	_ = llm_capabilities_music.NewMiniMaxProvider
	_ = llm_capabilities_music.NewSunoProvider
	_ = llm_capabilities_rerank.DefaultCohereConfig
	_ = llm_capabilities_rerank.DefaultJinaConfig
	_ = llm_capabilities_rerank.DefaultVoyageConfig
	_ = llm_capabilities_threed.DefaultMeshyConfig
	_ = llm_capabilities_threed.DefaultTripoConfig
	_ = llm_capabilities_threed.NewMeshyProvider
	_ = llm_capabilities_threed.NewTripoProvider
	_ = llm_capabilities_tools.AuditMiddleware
	_ = llm_capabilities_tools.CostControlMiddleware
	_ = llm_capabilities_tools.CreateAgentBudget
	_ = llm_capabilities_tools.CreateAgentRateLimit
	_ = llm_capabilities_tools.CreateGlobalBudget
	_ = llm_capabilities_tools.CreateGlobalRateLimit
	_ = llm_capabilities_tools.CreateTokenBucketRateLimit
	_ = llm_capabilities_tools.CreateToolCost
	_ = llm_capabilities_tools.CreateToolRateLimit
	_ = llm_capabilities_tools.CreateUserBudget
	_ = llm_capabilities_tools.CreateUserRateLimit
	_ = llm_capabilities_tools.DefaultDuckDuckGoConfig
	_ = llm_capabilities_tools.DefaultFallbackConfig
	_ = llm_capabilities_tools.DefaultFirecrawlConfig
	_ = llm_capabilities_tools.DefaultHTTPScrapeConfig
	_ = llm_capabilities_tools.DefaultJinaReaderConfig
	_ = llm_capabilities_tools.DefaultParallelConfig
	_ = llm_capabilities_tools.DefaultSearXNGConfig
	_ = llm_capabilities_tools.DefaultTavilyConfig
	_ = llm_capabilities_tools.DefaultWebScrapeOptions
	_ = llm_capabilities_tools.DefaultWebScrapeToolConfig
	_ = llm_capabilities_tools.DefaultWebSearchOptions
	_ = llm_capabilities_tools.DefaultWebSearchToolConfig
	_ = llm_capabilities_tools.GetPermissionContext
	_ = llm_capabilities_tools.LogCostAlert
	_ = llm_capabilities_tools.LogPermissionCheck
	_ = llm_capabilities_tools.LogRateLimitHit
	_ = llm_capabilities_tools.LogToolCall
	_ = llm_capabilities_tools.LogToolResult
	_ = llm_capabilities_tools.NewBatchExecutor
	_ = llm_capabilities_tools.NewCostController
	_ = llm_capabilities_tools.NewDefaultExecutorWithConfig
	_ = llm_capabilities_tools.NewDuckDuckGoSearchProvider
	_ = llm_capabilities_tools.NewFileAuditBackend
	_ = llm_capabilities_tools.NewFirecrawlProvider
	_ = llm_capabilities_tools.NewFixedWindowLimiter
	_ = llm_capabilities_tools.NewHTTPScrapeProvider
	_ = llm_capabilities_tools.NewJinaScraperProvider
	_ = llm_capabilities_tools.NewMemoryAuditBackend
	_ = llm_capabilities_tools.NewParallelExecutor
	_ = llm_capabilities_tools.NewPermissionManager
	_ = llm_capabilities_tools.NewRateLimitManager
	_ = llm_capabilities_tools.NewResilientExecutor
	_ = llm_capabilities_tools.NewSearXNGSearchProvider
	_ = llm_capabilities_tools.NewSlidingWindowLimiter
	_ = llm_capabilities_tools.NewTavilySearchProvider
	_ = llm_capabilities_tools.NewTokenBucketLimiter
	_ = llm_capabilities_tools.NewToolCallChain
	_ = llm_capabilities_tools.NewWebScrapeTool
	_ = llm_capabilities_tools.NewWebSearchTool
	_ = llm_capabilities_tools.PermissionMiddleware
	_ = llm_capabilities_tools.RateLimitMiddleware
	_ = llm_capabilities_tools.RegisterWebScrapeTool
	_ = llm_capabilities_tools.RegisterWebSearchTool
	_ = llm_capabilities_tools.WithPermissionContext
	_ = llm_capabilities_video.DefaultGeminiConfig
	_ = llm_capabilities_video.DefaultKlingConfig
	_ = llm_capabilities_video.DefaultLumaConfig
	_ = llm_capabilities_video.DefaultMiniMaxVideoConfig
	_ = llm_capabilities_video.DefaultRunwayConfig
	_ = llm_capabilities_video.DefaultSoraConfig
	_ = llm_capabilities_video.DefaultVeoConfig
	_ = llm_capabilities_video.NewGeminiProvider
	_ = llm_capabilities_video.NewProvider
	_ = llm_circuitbreaker.DefaultConfig
	_ = llm_circuitbreaker.NewCircuitBreaker
	_ = llm_config.NewPolicyManager
	_ = llm_gateway.NewStateMachine
	_ = llm_idempotency.NewMemoryManager
	_ = llm_idempotency.NewMemoryManagerWithCleanup
	_ = llm_idempotency.NewRedisManager
	_ = llm_middleware.HeadersMiddleware
	_ = llm_middleware.RateLimitMiddleware
	_ = llm_middleware.RetryMiddleware
	_ = llm_middleware.TracingMiddleware
	_ = llm_middleware.ValidatorMiddleware
	_ = llm_providers_base.BearerTokenHeaders
	_ = llm_providers_base.ChooseCapabilityModel
	_ = llm_providers_base.NewBaseCapabilityProvider
	_ = llm_providers_base.SafeCloseBody
	_ = llm_providers_openai.WithPreviousResponseID
	_ = llm_providers_vendor.NewAnthropicProfile
	_ = llm_runtime_policy.DefaultBudgetConfig
	_ = llm_runtime_policy.IsRetryableError
	_ = llm_runtime_policy.NewBackoffRetryer
	_ = llm_runtime_policy.WrapRetryable
	_ = llm_runtime_router.DefaultSemanticRouterConfig
	_ = llm_runtime_router.NewABRouter
	_ = llm_runtime_router.NewAPIKeyPool
	_ = llm_runtime_router.NewDefaultProviderFactory
	_ = llm_runtime_router.NewHealthChecker
	_ = llm_runtime_router.NewHealthCheckerWithProviders
	_ = llm_runtime_router.NewHealthMonitor
	_ = llm_runtime_router.NewMultiProviderRouter
	_ = llm_runtime_router.NewPrefixRouter
	_ = llm_runtime_router.NewRouter
	_ = llm_runtime_router.NewSemanticRouter
	_ = llm_runtime_router.NewWeightedRouter
	_ = llm_streaming.BytesToString
	_ = llm_streaming.DefaultBackpressureConfig
	_ = llm_streaming.NewBackpressureStream
	_ = llm_streaming.NewChunkReader
	_ = llm_streaming.NewRateLimiter
	_ = llm_streaming.NewRingBuffer
	_ = llm_streaming.NewStreamMultiplexer
	_ = llm_streaming.NewStringView
	_ = llm_streaming.NewZeroCopyBuffer
	_ = llm_streaming.StringToBytes
	_ = llm_tokenizer.NewTiktokenTokenizer
	_ = llm_tokenizer.RegisterOpenAITokenizers
	_ = llm_tokenizer.RegisterTokenizer
	_ = pkg_cache.IsCacheMiss
	_ = pkg_middleware.RequestIDFromContext
	_ = pkg_migration.GetMigrationsPath
	_ = pkg_migration.NewMigratorFromConfig
	_ = pkg_mongodb.NewClientFromOptions
	_ = pkg_server.DefaultConfig
	_ = pkg_telemetry.LoggerWithTrace
	_ = rag_core.BuildSharedEvalMetrics
	_ = rag_core.BuildSharedRetrievalRecords
	_ = rag_core.ErrConfig
	_ = rag_core.ErrInternal
	_ = rag_core.ErrTimeout
	_ = rag_core.ErrUpstream
	_ = rag_core.NewRAGError
	_ = rag_loader.NewArxivSourceAdapter
	_ = rag_loader.NewCSVLoader
	_ = rag_loader.NewGitHubSourceAdapter
	_ = rag_loader.NewJSONLoader
	_ = rag_loader.NewLoaderRegistry
	_ = rag_loader.NewMarkdownLoader
	_ = rag_loader.NewTextLoader
	_ = rag_retrieval.DefaultPipelineConfig
	_ = rag_retrieval.NewPipeline
	_ = rag_retrieval.NewStrategyNode
	_ = rag_retrieval.NewStrategyRegistry
	_ = rag_retrieval.RegisterDefaultStrategies
	_ = rag_sources.DefaultArxivConfig
	_ = rag_sources.DefaultGitHubConfig
	_ = rag_sources.FilterByLanguage
	_ = rag_sources.FilterByStars
	_ = rag_sources.NewArxivSource
	_ = rag_sources.NewGitHubSource
	_ = rag.AdaptiveHNSWConfig
	_ = rag.DefaultChunkingConfig
	_ = rag.DefaultContextualRetrievalConfig
	_ = rag.DefaultCrossEncoderConfig
	_ = rag.DefaultGraphRAGConfig
	_ = rag.DefaultHNSWConfig
	_ = rag.DefaultLLMRerankerConfig
	_ = rag.DefaultMultiHopConfig
	_ = rag.DefaultQueryRouterConfig
	_ = rag.DefaultQueryTransformConfig
	_ = rag.DefaultWebRetrieverConfig
	_ = rag.EmbeddingSimilarity
	_ = rag.Float32ToFloat64
	_ = rag.Float64ToFloat32
	_ = rag.NewContextualRetrieval
	_ = rag.NewCrossEncoderReranker
	_ = rag.NewDocumentChunker
	_ = rag.NewEstimatorAdapter
	_ = rag.NewGraphRAG
	_ = rag.NewHNSWIndex
	_ = rag.NewLLMContextProvider
	_ = rag.NewLLMReranker
	_ = rag.NewLLMTokenizerAdapter
	_ = rag.NewMultiHopReasoner
	_ = rag.NewQueryRouter
	_ = rag.NewQueryTransformer
	_ = rag.NewSimpleContextProvider
	_ = rag.NewSimpleGraphEmbedder
	_ = rag.NewSimpleReranker
	_ = rag.NewTiktokenAdapter
	_ = rag.NewWebRetriever

	// runtime-gated real invocations to keep module integrations on the executable chain
	if os.Getenv("AGENTFLOW_REACHABILITY_INTEGRATION") == "1" {
		llm_capabilities_audio.DefaultDeepgramConfig()
		llm_capabilities_audio.DefaultElevenLabsConfig()
		llm_capabilities_audio.DefaultOpenAISTTConfig()
		llm_capabilities_audio.DefaultOpenAITTSConfig()
		llm_capabilities_audio.NewDeepgramProvider(llm_capabilities_audio.DeepgramConfig{})
		llm_capabilities_embedding.DefaultCohereConfig()
		llm_capabilities_embedding.DefaultGeminiConfig()
		llm_capabilities_embedding.DefaultJinaConfig()
		llm_capabilities_embedding.DefaultOpenAIConfig()
		llm_capabilities_embedding.DefaultVoyageConfig()
		llm_capabilities_image.DefaultFluxConfig()
		llm_capabilities_image.DefaultGeminiConfig()
		llm_capabilities_image.DefaultImagen4Config()
		llm_capabilities_image.DefaultOpenAIConfig()
		llm_capabilities_image.DefaultStabilityConfig()
		llm_capabilities_image.NewFluxProvider(llm_capabilities_image.FluxConfig{})
		llm_capabilities_image.NewProviderFromConfig(llm_capabilities_image.FactoryConfig{})
		llm_capabilities_moderation.DefaultOpenAIConfig()
		llm_capabilities_moderation.NewOpenAIProvider(llm_capabilities_moderation.OpenAIConfig{})
		llm_capabilities_multimodal.DefaultAudioConfig()
		llm_capabilities_multimodal.DefaultProcessor()
		llm_capabilities_multimodal.DefaultVisionConfig()
		llm_capabilities_multimodal.LoadAudioFromFile("")
		llm_capabilities_multimodal.LoadImageFromFile("")
		llm_capabilities_multimodal.LoadImageFromURL("")
		llm_capabilities_multimodal.NewAudioBase64Content("", "")
		llm_capabilities_multimodal.NewAudioURLContent("")
		llm_capabilities_multimodal.NewImageBase64Content("", "")
		llm_capabilities_multimodal.NewImageURLContent("")
		llm_capabilities_multimodal.NewMultimodalProvider(nil, nil)
		llm_capabilities_multimodal.NewProcessor(llm_capabilities_multimodal.VisionConfig{}, llm_capabilities_multimodal.AudioConfig{})
		llm_capabilities_multimodal.NewTextContent("")
		llm_capabilities_music.DefaultMiniMaxMusicConfig()
		llm_capabilities_music.DefaultSunoConfig()
		llm_capabilities_music.NewMiniMaxProvider(llm_capabilities_music.MiniMaxMusicConfig{})
		llm_capabilities_music.NewSunoProvider(llm_capabilities_music.SunoConfig{})
		llm_capabilities_rerank.DefaultCohereConfig()
		llm_capabilities_rerank.DefaultJinaConfig()
		llm_capabilities_rerank.DefaultVoyageConfig()
		llm_capabilities_threed.DefaultMeshyConfig()
		llm_capabilities_threed.DefaultTripoConfig()
		llm_capabilities_threed.NewMeshyProvider(llm_capabilities_threed.MeshyConfig{})
		llm_capabilities_threed.NewTripoProvider(llm_capabilities_threed.TripoConfig{})
		llm_capabilities_tools.AuditMiddleware(nil)
		llm_capabilities_tools.CostControlMiddleware(nil, nil)
		llm_capabilities_tools.CreateAgentBudget("", "", "", 0, "")
		llm_capabilities_tools.CreateAgentRateLimit("", "", 0, 0)
		llm_capabilities_tools.CreateGlobalBudget("", "", 0, "")
		llm_capabilities_tools.CreateGlobalRateLimit("", "", 0, 0)
		llm_capabilities_tools.CreateTokenBucketRateLimit("", "", 0, 0)
		llm_capabilities_tools.CreateToolCost("", 0, 0)
		llm_capabilities_tools.CreateToolRateLimit("", "", "", 0, 0)
		llm_capabilities_tools.CreateUserBudget("", "", "", 0, "")
		llm_capabilities_tools.CreateUserRateLimit("", "", 0, 0)
		llm_capabilities_tools.DefaultDuckDuckGoConfig()
		llm_capabilities_tools.DefaultFallbackConfig()
		llm_capabilities_tools.DefaultFirecrawlConfig()
		llm_capabilities_tools.DefaultHTTPScrapeConfig()
		llm_capabilities_tools.DefaultJinaReaderConfig()
		llm_capabilities_tools.DefaultParallelConfig()
		llm_capabilities_tools.DefaultSearXNGConfig()
		llm_capabilities_tools.DefaultTavilyConfig()
		llm_capabilities_tools.DefaultWebScrapeOptions()
		llm_capabilities_tools.DefaultWebScrapeToolConfig()
		llm_capabilities_tools.DefaultWebSearchOptions()
		llm_capabilities_tools.DefaultWebSearchToolConfig()
		llm_capabilities_tools.GetPermissionContext(nil)
		llm_capabilities_tools.LogCostAlert(nil, "", "", 0, "")
		llm_capabilities_tools.LogPermissionCheck(nil, nil, "", "")
		llm_capabilities_tools.LogRateLimitHit(nil, "", "", "", "")
		llm_capabilities_tools.LogToolCall(nil, "", "", "", nil)
		llm_capabilities_tools.LogToolResult(nil, "", "", "", nil, nil, 0)
		llm_capabilities_tools.NewBatchExecutor(nil, nil)
		llm_capabilities_tools.NewCostController(nil)
		llm_capabilities_tools.NewDefaultExecutorWithConfig(nil, nil, llm_capabilities_tools.ExecutorConfig{})
		llm_capabilities_tools.NewDuckDuckGoSearchProvider(llm_capabilities_tools.DuckDuckGoConfig{})
		llm_capabilities_tools.NewFileAuditBackend(nil, nil)
		llm_capabilities_tools.NewFirecrawlProvider(llm_capabilities_tools.FirecrawlConfig{})
		llm_capabilities_tools.NewHTTPScrapeProvider(llm_capabilities_tools.HTTPScrapeConfig{})
		llm_capabilities_tools.NewJinaScraperProvider(llm_capabilities_tools.JinaReaderConfig{})
		llm_capabilities_tools.NewMemoryAuditBackend(0)
		llm_capabilities_tools.NewParallelExecutor(nil, llm_capabilities_tools.ParallelConfig{}, nil)
		llm_capabilities_tools.NewPermissionManager(nil)
		llm_capabilities_tools.NewRateLimitManager(nil)
		llm_capabilities_tools.NewResilientExecutor(nil, nil, nil)
		llm_capabilities_tools.NewSearXNGSearchProvider(llm_capabilities_tools.SearXNGConfig{})
		llm_capabilities_tools.NewTavilySearchProvider(llm_capabilities_tools.TavilyConfig{})
		llm_capabilities_tools.NewToolCallChain(nil, nil)
		llm_capabilities_tools.NewWebScrapeTool(llm_capabilities_tools.WebScrapeToolConfig{}, nil)
		llm_capabilities_tools.NewWebSearchTool(llm_capabilities_tools.WebSearchToolConfig{}, nil)
		llm_capabilities_tools.PermissionMiddleware(nil)
		llm_capabilities_tools.RateLimitMiddleware(nil, nil)
		llm_capabilities_tools.RegisterWebScrapeTool(nil, llm_capabilities_tools.WebScrapeToolConfig{}, nil)
		llm_capabilities_tools.RegisterWebSearchTool(nil, llm_capabilities_tools.WebSearchToolConfig{}, nil)
		llm_capabilities_tools.WithPermissionContext(nil, nil)
		llm_capabilities_video.DefaultGeminiConfig()
		llm_capabilities_video.DefaultKlingConfig()
		llm_capabilities_video.DefaultLumaConfig()
		llm_capabilities_video.DefaultMiniMaxVideoConfig()
		llm_capabilities_video.DefaultRunwayConfig()
		llm_capabilities_video.DefaultSoraConfig()
		llm_capabilities_video.DefaultVeoConfig()
		llm_capabilities_video.NewGeminiProvider(llm_capabilities_video.GeminiConfig{}, nil)
		llm_capabilities_video.NewProvider("", nil, nil)
		llm_circuitbreaker.DefaultConfig()
		llm_circuitbreaker.NewCircuitBreaker(nil, nil)
		llm_config.NewPolicyManager()
		llm_gateway.NewStateMachine()
		llm_idempotency.NewMemoryManager(nil)
		llm_idempotency.NewMemoryManagerWithCleanup(nil, 0)
		llm_idempotency.NewRedisManager(nil, "", nil)
		llm_middleware.HeadersMiddleware(nil)
		llm_middleware.RateLimitMiddleware(nil)
		llm_middleware.RetryMiddleware(0, 0)
		llm_middleware.TracingMiddleware(nil)
		llm_middleware.ValidatorMiddleware()
		llm_providers_base.BearerTokenHeaders(nil, "")
		llm_providers_base.NewBaseCapabilityProvider(llm_providers_base.CapabilityConfig{})
		llm_providers_base.SafeCloseBody(nil)
		llm_providers_openai.WithPreviousResponseID(nil, "")
		llm_providers_vendor.NewAnthropicProfile(llm_providers_vendor.AnthropicConfig{}, nil)
		llm_runtime_policy.DefaultBudgetConfig()
		llm_runtime_policy.IsRetryableError(nil)
		llm_runtime_policy.NewBackoffRetryer(nil, nil)
		llm_runtime_policy.WrapRetryable(nil)
		llm_runtime_router.DefaultSemanticRouterConfig()
		llm_runtime_router.NewABRouter(llm_runtime_router.ABTestConfig{}, nil)
		llm_runtime_router.NewDefaultProviderFactory()
		llm_runtime_router.NewHealthChecker(nil, 0, nil)
		llm_runtime_router.NewHealthCheckerWithProviders(nil, nil, 0, 0, nil)
		llm_runtime_router.NewHealthMonitor(nil)
		llm_runtime_router.NewMultiProviderRouter(nil, nil, llm_runtime_router.RouterOptions{})
		llm_runtime_router.NewPrefixRouter(nil)
		llm_runtime_router.NewRouter(nil, nil, llm_runtime_router.RouterOptions{})
		llm_runtime_router.NewSemanticRouter(nil, nil, llm_runtime_router.SemanticRouterConfig{}, nil)
		llm_runtime_router.NewWeightedRouter(nil, nil)
		llm_streaming.BytesToString(nil)
		llm_streaming.DefaultBackpressureConfig()
		llm_streaming.NewChunkReader(nil, 0)
		llm_streaming.NewRateLimiter(0, 0)
		llm_streaming.NewRingBuffer(0)
		llm_streaming.NewStreamMultiplexer(nil)
		llm_streaming.NewStringView(nil)
		llm_streaming.NewZeroCopyBuffer(0)
		llm_streaming.StringToBytes("")
		llm_tokenizer.NewTiktokenTokenizer("")
		llm_tokenizer.RegisterOpenAITokenizers()
		llm_tokenizer.RegisterTokenizer("", nil)
		pkg_cache.IsCacheMiss(nil)
		pkg_middleware.RequestIDFromContext(nil)
		pkg_migration.GetMigrationsPath("")
		pkg_migration.NewMigratorFromConfig(nil)
		pkg_mongodb.NewClientFromOptions(nil, "", nil)
		pkg_server.DefaultConfig()
		pkg_telemetry.LoggerWithTrace(nil, nil)
		rag.AdaptiveHNSWConfig(0)
		rag.DefaultChunkingConfig()
		rag.DefaultContextualRetrievalConfig()
		rag.DefaultCrossEncoderConfig()
		rag.DefaultGraphRAGConfig()
		rag.DefaultHNSWConfig()
		rag.DefaultLLMRerankerConfig()
		rag.DefaultMultiHopConfig()
		rag.DefaultQueryRouterConfig()
		rag.DefaultQueryTransformConfig()
		rag.DefaultWebRetrieverConfig()
		rag.EmbeddingSimilarity(nil, nil)
		rag.Float32ToFloat64(nil)
		rag.Float64ToFloat32(nil)
		rag.NewContextualRetrieval(nil, nil, rag.ContextualRetrievalConfig{}, nil)
		rag.NewCrossEncoderReranker(nil, rag.CrossEncoderConfig{}, nil)
		rag.NewDocumentChunker(rag.ChunkingConfig{}, nil, nil)
		rag.NewEstimatorAdapter("", 0, nil)
		rag.NewGraphRAG(nil, nil, nil, rag.GraphRAGConfig{}, nil)
		rag.NewHNSWIndex(rag.HNSWConfig{}, nil)
		rag.NewLLMContextProvider(nil, nil)
		rag.NewLLMReranker(nil, rag.LLMRerankerConfig{}, nil)
		rag.NewLLMTokenizerAdapter(nil, nil)
		rag.NewMultiHopReasoner(rag.MultiHopConfig{}, nil, nil, nil, nil, nil)
		rag.NewQueryRouter(rag.QueryRouterConfig{}, nil, nil, nil)
		rag.NewQueryTransformer(rag.QueryTransformConfig{}, nil, nil)
		rag.NewSimpleContextProvider(nil)
		rag.NewSimpleGraphEmbedder(rag.SimpleGraphEmbedderConfig{}, nil)
		rag.NewSimpleReranker(nil)
		rag.NewTiktokenAdapter("", nil)
		rag.NewWebRetriever(rag.WebRetrieverConfig{}, nil, nil, nil)
		rag_core.BuildSharedEvalMetrics(rag_core.EvalMetrics{})
		rag_core.BuildSharedRetrievalRecords(nil, af_types.RetrievalTrace{})
		rag_core.ErrConfig("", nil)
		rag_core.ErrInternal("", nil)
		rag_core.ErrTimeout("", nil)
		rag_core.ErrUpstream("", nil)
		rag_core.NewRAGError("", "", nil)
		rag_loader.NewArxivSourceAdapter(nil, 0)
		rag_loader.NewCSVLoader(rag_loader.CSVLoaderConfig{})
		rag_loader.NewGitHubSourceAdapter(nil, 0)
		rag_loader.NewJSONLoader(rag_loader.JSONLoaderConfig{})
		rag_loader.NewLoaderRegistry()
		rag_loader.NewMarkdownLoader()
		rag_loader.NewTextLoader()
		rag_retrieval.DefaultPipelineConfig()
		rag_retrieval.NewPipeline(rag_retrieval.PipelineConfig{}, nil, nil, nil, nil)
		rag_retrieval.NewStrategyNode("", rag_retrieval.StrategyNodes{})
		rag_retrieval.NewStrategyRegistry()
		rag_retrieval.RegisterDefaultStrategies(nil, rag_retrieval.StrategyNodes{})
		rag_sources.DefaultArxivConfig()
		rag_sources.DefaultGitHubConfig()
		rag_sources.FilterByLanguage(nil, "")
		rag_sources.FilterByStars(nil, 0)
		rag_sources.NewArxivSource(rag_sources.ArxivConfig{}, nil)
		rag_sources.NewGitHubSource(rag_sources.GitHubConfig{}, nil)

		// generic helpers + test utility modules
		cb := llm_circuitbreaker.NewCircuitBreaker(nil, nil)
		llm_circuitbreaker.CallWithResultTyped[int](cb, context.Background(), func() (int, error) { return 1, nil })

		idm := llm_idempotency.NewMemoryManager(nil)
		llm_idempotency.SetTyped[map[string]any](idm, context.Background(), "k", map[string]any{"ok": true}, time.Second)
		llm_idempotency.GetTyped[map[string]any](idm, context.Background(), "k")

		retryer := llm_runtime_policy.NewBackoffRetryer(nil, nil)
		llm_runtime_policy.DoWithResultTyped[int](retryer, context.Background(), func() (int, error) { return 1, nil })

		testutil.TestContext(nil)
		testutil.WaitFor(func() bool { return true }, time.Millisecond)
		testutil.WaitForChannel(make(chan struct{}), time.Millisecond)
		mockProvider := testutil_mocks.NewSuccessProvider("ok")
		mockProvider.Completion(context.Background(), nil)
		mockProvider.Stream(context.Background(), nil)
		mockProvider.HealthCheck(context.Background())
		mockProvider.Name()
		mockProvider.SupportsNativeFunctionCalling()
		mockProvider.ListModels(context.Background())
		mockProvider.Endpoints()

	}
}

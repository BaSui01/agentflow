// 示例：使用多模态能力（向量、重排、语音合成、语音识别、图像）
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/BaSui01/agentflow/llm/embedding"
	"github.com/BaSui01/agentflow/llm/image"
	"github.com/BaSui01/agentflow/llm/multimodal"
	"github.com/BaSui01/agentflow/llm/rerank"
	"github.com/BaSui01/agentflow/rag"
	"github.com/BaSui01/agentflow/llm/speech"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Example 1: Embedding providers
	fmt.Println("=== Embedding Providers ===")
	demoEmbedding(ctx, logger)

	// Example 2: Rerank providers
	fmt.Println("\n=== Rerank Providers ===")
	demoRerank(ctx, logger)

	// Example 3: Enhanced RAG retrieval
	fmt.Println("\n=== Enhanced RAG Retrieval ===")
	demoEnhancedRetrieval(ctx, logger)

	// Example 4: Multimodal router
	fmt.Println("\n=== Multimodal Router ===")
	demoMultimodalRouter(ctx, logger)
}

func demoEmbedding(ctx context.Context, logger *zap.Logger) {
	// OpenAI embedding
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey != "" {
		provider := embedding.NewOpenAIProvider(embedding.OpenAIConfig{
			APIKey:     openaiKey,
			Model:      "text-embedding-3-large",
			Dimensions: 1024, // Reduced dimensions for efficiency
		})

		resp, err := provider.Embed(ctx, &embedding.EmbeddingRequest{
			Input:     []string{"Hello world", "Machine learning is fascinating"},
			InputType: embedding.InputTypeDocument,
		})
		if err != nil {
			log.Printf("OpenAI embedding error: %v", err)
		} else {
			fmt.Printf("OpenAI: Generated %d embeddings, dims=%d, tokens=%d\n",
				len(resp.Embeddings), len(resp.Embeddings[0].Embedding), resp.Usage.TotalTokens)
		}
	}

	// Voyage AI embedding
	voyageKey := os.Getenv("VOYAGE_API_KEY")
	if voyageKey != "" {
		provider := embedding.NewVoyageProvider(embedding.VoyageConfig{
			APIKey: voyageKey,
			Model:  "voyage-3-large",
		})

		emb, err := provider.EmbedQuery(ctx, "What is machine learning?")
		if err != nil {
			log.Printf("Voyage embedding error: %v", err)
		} else {
			fmt.Printf("Voyage: Query embedding dims=%d\n", len(emb))
		}
	}

	// Jina AI embedding with Matryoshka dimensions
	jinaKey := os.Getenv("JINA_API_KEY")
	if jinaKey != "" {
		provider := embedding.NewJinaProvider(embedding.JinaConfig{
			APIKey: jinaKey,
			Model:  "jina-embeddings-v3",
		})

		resp, err := provider.Embed(ctx, &embedding.EmbeddingRequest{
			Input:      []string{"Jina AI provides excellent embeddings"},
			Dimensions: 512, // Matryoshka: use smaller dimensions
			InputType:  embedding.InputTypeDocument,
		})
		if err != nil {
			log.Printf("Jina embedding error: %v", err)
		} else {
			fmt.Printf("Jina: Embedding dims=%d (Matryoshka)\n", len(resp.Embeddings[0].Embedding))
		}
	}
}

func demoRerank(ctx context.Context, logger *zap.Logger) {
	documents := []string{
		"Machine learning is a subset of artificial intelligence.",
		"Python is a popular programming language.",
		"Deep learning uses neural networks with many layers.",
		"Go is a statically typed language developed by Google.",
		"Natural language processing enables computers to understand text.",
	}
	query := "What is deep learning?"

	// Cohere reranker
	cohereKey := os.Getenv("COHERE_API_KEY")
	if cohereKey != "" {
		provider := rerank.NewCohereProvider(rerank.CohereConfig{
			APIKey: cohereKey,
			Model:  "rerank-v3.5",
		})

		results, err := provider.RerankSimple(ctx, query, documents, 3)
		if err != nil {
			log.Printf("Cohere rerank error: %v", err)
		} else {
			fmt.Println("Cohere rerank results:")
			for _, r := range results {
				fmt.Printf("  [%.3f] %s\n", r.RelevanceScore, documents[r.Index][:50])
			}
		}
	}

	// Jina reranker
	jinaKey := os.Getenv("JINA_API_KEY")
	if jinaKey != "" {
		provider := rerank.NewJinaProvider(rerank.JinaConfig{
			APIKey: jinaKey,
		})

		results, err := provider.RerankSimple(ctx, query, documents, 3)
		if err != nil {
			log.Printf("Jina rerank error: %v", err)
		} else {
			fmt.Println("Jina rerank results:")
			for _, r := range results {
				fmt.Printf("  [%.3f] %s\n", r.RelevanceScore, documents[r.Index][:50])
			}
		}
	}
}

func demoEnhancedRetrieval(ctx context.Context, logger *zap.Logger) {
	cohereKey := os.Getenv("COHERE_API_KEY")
	if cohereKey == "" {
		fmt.Println("Skipping: COHERE_API_KEY not set")
		return
	}

	// Create enhanced retriever with Cohere embedding + reranking
	retriever := rag.NewCohereRetriever(cohereKey, logger)

	// Index documents
	docs := []rag.Document{
		{ID: "1", Content: "Machine learning algorithms learn from data to make predictions."},
		{ID: "2", Content: "Deep learning is a type of machine learning using neural networks."},
		{ID: "3", Content: "Natural language processing helps computers understand human language."},
		{ID: "4", Content: "Computer vision enables machines to interpret visual information."},
		{ID: "5", Content: "Reinforcement learning trains agents through rewards and penalties."},
	}

	if err := retriever.IndexDocumentsWithEmbedding(ctx, docs); err != nil {
		log.Printf("Index error: %v", err)
		return
	}

	// Retrieve with hybrid search + reranking
	results, err := retriever.RetrieveWithProviders(ctx, "How do neural networks learn?")
	if err != nil {
		log.Printf("Retrieval error: %v", err)
		return
	}

	fmt.Println("Enhanced retrieval results:")
	for _, r := range results {
		fmt.Printf("  [%.3f] %s: %s\n", r.FinalScore, r.Document.ID, r.Document.Content[:50])
	}
}

func demoMultimodalRouter(ctx context.Context, logger *zap.Logger) {
	router := multimodal.NewRouter()

	// Register providers based on available API keys
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		router.RegisterEmbedding("openai", embedding.NewOpenAIProvider(embedding.OpenAIConfig{APIKey: key}), true)
		router.RegisterTTS("openai", speech.NewOpenAITTSProvider(speech.OpenAITTSConfig{APIKey: key}), true)
		router.RegisterSTT("openai", speech.NewOpenAISTTProvider(speech.OpenAISTTConfig{APIKey: key}), true)
		router.RegisterImage("openai", image.NewOpenAIProvider(image.OpenAIConfig{APIKey: key}), true)
	}

	if key := os.Getenv("COHERE_API_KEY"); key != "" {
		router.RegisterEmbedding("cohere", embedding.NewCohereProvider(embedding.CohereConfig{APIKey: key}), false)
		router.RegisterRerank("cohere", rerank.NewCohereProvider(rerank.CohereConfig{APIKey: key}), true)
	}

	if key := os.Getenv("ELEVENLABS_API_KEY"); key != "" {
		router.RegisterTTS("elevenlabs", speech.NewElevenLabsProvider(speech.ElevenLabsConfig{APIKey: key}), false)
	}

	// List registered providers
	providers := router.ListProviders()
	fmt.Println("Registered providers:")
	for cap, names := range providers {
		fmt.Printf("  %s: %v\n", cap, names)
	}

	// Use router for operations
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		// Embedding via router
		resp, err := router.Embed(ctx, &embedding.EmbeddingRequest{
			Input: []string{"Router test"},
		}, "") // Empty = use default
		if err != nil {
			log.Printf("Router embed error: %v", err)
		} else {
			fmt.Printf("Router embedding: dims=%d\n", len(resp.Embeddings[0].Embedding))
		}
	}
}

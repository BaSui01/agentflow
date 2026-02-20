package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/BaSui01/agentflow/agent/protocol/a2a"
	"github.com/BaSui01/agentflow/agent/structured"
)

func main() {
	fmt.Println("=== AgentFlow A2A Protocol Example ===")

	// 1. Agent Card Generation
	demonstrateAgentCard()

	// 2. A2A Message Creation
	demonstrateMessages()

	// 3. Client/Server Setup (demonstration only)
	demonstrateClientServer()
}

// demonstrateAgentCard shows how to create and configure Agent Cards.
func demonstrateAgentCard() {
	fmt.Println("--- 1. Agent Card Generation ---")

	// Create an Agent Card manually
	card := a2a.NewAgentCard(
		"assistant-agent",
		"A helpful AI assistant for general tasks",
		"https://api.example.com/agents/assistant",
		"1.0.0",
	)

	// Add capabilities
	card.AddCapability("chat", "Interactive conversation", a2a.CapabilityTypeQuery)
	card.AddCapability("task_execution", "Execute user tasks", a2a.CapabilityTypeTask)
	card.AddCapability("streaming", "Stream responses", a2a.CapabilityTypeStream)

	// Add tool definitions
	searchParams := structured.NewObjectSchema()
	searchParams.AddProperty("query", structured.NewStringSchema().WithMinLength(1))
	searchParams.AddProperty("limit", structured.NewIntegerSchema().WithMinimum(1).WithMaximum(100))
	searchParams.AddRequired("query")

	card.AddTool("search", "Search for information", searchParams)
	card.AddTool("calculate", "Perform calculations", structured.NewObjectSchema().
		AddProperty("expression", structured.NewStringSchema()).
		AddRequired("expression"))

	// Set metadata
	card.SetMetadata("author", "AgentFlow Team")
	card.SetMetadata("category", "general-purpose")

	// Set input/output schemas
	card.SetInputSchema(structured.NewObjectSchema().
		AddProperty("message", structured.NewStringSchema()).
		AddRequired("message"))

	card.SetOutputSchema(structured.NewObjectSchema().
		AddProperty("response", structured.NewStringSchema()).
		AddProperty("confidence", structured.NewNumberSchema()))

	// Print the Agent Card
	cardJSON, _ := json.MarshalIndent(card, "", "  ")
	fmt.Printf("Agent Card:\n%s\n\n", cardJSON)

	// Using the generator with SimpleAgentConfig
	generator := a2a.NewAgentCardGenerator()
	config := &a2a.SimpleAgentConfig{
		AgentID:          "analyzer-001",
		AgentName:        "Data Analyzer",
		AgentType:        "analyzer",
		AgentDescription: "Analyzes data and provides insights",
		AgentMetadata:    map[string]string{"version": "2.0.0"},
	}

	generatedCard := generator.Generate(config, "https://api.example.com")
	fmt.Printf("Generated Card Name: %s\n", generatedCard.Name)
	fmt.Printf("Generated Card URL: %s\n", generatedCard.URL)
	fmt.Printf("Capabilities: %d\n\n", len(generatedCard.Capabilities))
}

// demonstrateMessages shows how to create and work with A2A messages.
func demonstrateMessages() {
	fmt.Println("--- 2. A2A Message Creation ---")

	// Create a task message
	taskMsg := a2a.NewTaskMessage(
		"client-agent",
		"assistant-agent",
		map[string]any{
			"task":    "summarize",
			"content": "This is a long document that needs summarization...",
			"options": map[string]any{
				"max_length": 100,
				"format":     "bullet_points",
			},
		},
	)

	fmt.Printf("Task Message:\n")
	fmt.Printf("  ID: %s\n", taskMsg.ID)
	fmt.Printf("  Type: %s\n", taskMsg.Type)
	fmt.Printf("  From: %s\n", taskMsg.From)
	fmt.Printf("  To: %s\n", taskMsg.To)
	fmt.Printf("  Timestamp: %s\n", taskMsg.Timestamp.Format(time.RFC3339))

	// Validate the message
	if err := taskMsg.Validate(); err != nil {
		fmt.Printf("  Validation: ✗ %v\n", err)
	} else {
		fmt.Printf("  Validation: ✓ Valid\n")
	}

	// Create a reply message
	replyMsg := taskMsg.CreateReply(a2a.A2AMessageTypeResult, map[string]any{
		"summary": []string{
			"Point 1: Key finding",
			"Point 2: Important detail",
			"Point 3: Conclusion",
		},
		"word_count": 15,
	})

	fmt.Printf("\nReply Message:\n")
	fmt.Printf("  ID: %s\n", replyMsg.ID)
	fmt.Printf("  Type: %s\n", replyMsg.Type)
	fmt.Printf("  ReplyTo: %s\n", replyMsg.ReplyTo)
	fmt.Printf("  IsReply: %v\n", replyMsg.IsReply())

	// Serialize to JSON
	msgJSON, _ := taskMsg.ToJSON()
	fmt.Printf("\nSerialized Message:\n%s\n\n", msgJSON)

	// Parse from JSON
	parsed, err := a2a.ParseA2AMessage(msgJSON)
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
	} else {
		fmt.Printf("Parsed Message ID: %s\n\n", parsed.ID)
	}
}

// demonstrateClientServer shows how to set up A2A client and server.
func demonstrateClientServer() {
	fmt.Println("--- 3. Client/Server Setup ---")

	// Create HTTP client
	clientConfig := a2a.DefaultClientConfig()
	clientConfig.Timeout = 30 * time.Second
	clientConfig.AgentID = "my-client-agent"
	clientConfig.Headers["X-Custom-Header"] = "custom-value"

	client := a2a.NewHTTPClient(clientConfig)
	fmt.Printf("Client created with timeout: %v\n", clientConfig.Timeout)

	// Create HTTP server
	serverConfig := a2a.DefaultServerConfig()
	serverConfig.BaseURL = "http://localhost:8080"
	serverConfig.RequestTimeout = 60 * time.Second
	serverConfig.EnableAuth = true
	serverConfig.AuthToken = "secret-token"

	server := a2a.NewHTTPServer(serverConfig)
	fmt.Printf("Server created with base URL: %s\n", serverConfig.BaseURL)

	// The server implements http.Handler
	var _ http.Handler = server

	// Example: Discover a remote agent (would work with a real server)
	fmt.Println("\nExample discovery call (requires running server):")
	fmt.Println("  card, err := client.Discover(ctx, \"https://remote-agent.example.com\")")

	// Example: Send a message
	fmt.Println("\nExample send call:")
	fmt.Println("  msg := a2a.NewTaskMessage(\"from\", \"to\", payload)")
	fmt.Println("  response, err := client.Send(ctx, msg)")

	// Example: Async message
	fmt.Println("\nExample async call:")
	fmt.Println("  taskID, err := client.SendAsync(ctx, msg)")
	fmt.Println("  result, err := client.GetResult(ctx, taskID)")

	// Demonstrate client methods
	fmt.Println("\nClient utility methods:")
	client.SetHeader("Authorization", "Bearer token")
	client.SetTimeout(45 * time.Second)
	fmt.Println("  - SetHeader: Set custom headers")
	fmt.Println("  - SetTimeout: Adjust timeout")
	fmt.Println("  - ClearCache: Clear agent card cache")
	fmt.Println("  - RegisterTask: Track async tasks")

	// Server endpoints
	fmt.Println("\nServer endpoints:")
	fmt.Println("  GET  /.well-known/agent.json  - Agent Card discovery")
	fmt.Println("  POST /a2a/messages            - Sync message handling")
	fmt.Println("  POST /a2a/messages/async      - Async message handling")
	fmt.Println("  GET  /a2a/tasks/{id}/result   - Get async result")

	// Quick integration example
	fmt.Println("\n--- Quick Integration Example ---")
	demonstrateQuickIntegration(client)
}

// demonstrateQuickIntegration shows a minimal integration example.
func demonstrateQuickIntegration(client *a2a.HTTPClient) {
	ctx := context.Background()

	// Create a task message
	msg := a2a.NewTaskMessage(
		"my-agent",
		"http://localhost:8080", // Target agent URL
		map[string]string{
			"query": "What is the weather today?",
		},
	)

	fmt.Printf("Created task message: %s\n", msg.ID)
	fmt.Printf("Message type: %s\n", msg.Type)

	// In a real scenario, you would send the message:
	// response, err := client.Send(ctx, msg)

	// For demo, just show the message is valid
	if err := msg.Validate(); err != nil {
		fmt.Printf("Message validation failed: %v\n", err)
	} else {
		fmt.Println("Message is valid and ready to send")
	}

	_ = ctx    // Used in real scenarios
	_ = client // Used in real scenarios
}

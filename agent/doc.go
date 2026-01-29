// Copyright 2024 AgentFlow Authors. All rights reserved.
// Use of this source code is governed by a MIT license that can be
// found in the LICENSE file.

/*
Package agent provides the core agent framework for AgentFlow.

# Overview

The agent package implements a flexible, extensible agent architecture that supports
various AI agent patterns including ReAct, Chain-of-Thought, and custom workflows.
It provides a unified interface for building intelligent agents that can reason,
plan, and execute tasks using Large Language Models (LLMs).

# Architecture

The agent framework follows a layered architecture:

	┌─────────────────────────────────────────────────────────────┐
	│                      Agent Interface                        │
	│  (ID, Name, Type, State, Init, Teardown, Plan, Execute)    │
	├─────────────────────────────────────────────────────────────┤
	│                      BaseAgent                              │
	│  (Common functionality, lifecycle management, hooks)        │
	├─────────────────────────────────────────────────────────────┤
	│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
	│  │   Memory    │  │    Tools    │  │     Guardrails      │ │
	│  │  Manager    │  │   Manager   │  │   (Validators)      │ │
	│  └─────────────┘  └─────────────┘  └─────────────────────┘ │
	├─────────────────────────────────────────────────────────────┤
	│                    LLM Provider                             │
	└─────────────────────────────────────────────────────────────┘

# Core Components

Agent Interface: Defines the contract for all agent implementations.

	type Agent interface {
	    ID() string
	    Name() string
	    Type() AgentType
	    State() State
	    Init(ctx context.Context) error
	    Teardown(ctx context.Context) error
	    Plan(ctx context.Context, input *Input) (*PlanResult, error)
	    Execute(ctx context.Context, input *Input) (*Output, error)
	    Observe(ctx context.Context, feedback *Feedback) error
	}

BaseAgent: Provides common functionality for all agent types including:
  - Lifecycle management (Init, Teardown)
  - State machine (Idle → Running → Completed/Failed)
  - Hook system (BeforeExecute, AfterExecute, OnError)
  - Checkpoint/recovery support

MemoryManager: Manages agent memory across multiple layers:
  - Working Memory: Short-term context storage
  - Episodic Memory: Event-based experiences
  - Semantic Memory: Factual knowledge
  - Procedural Memory: How-to knowledge

ToolManager: Handles tool registration, selection, and execution.

# Usage

Basic agent creation using the builder pattern:

	agent, err := agent.NewAgentBuilder(agent.Config{
	    Name:        "my-agent",
	    Type:        agent.TypeReAct,
	    MaxIterations: 10,
	}).
	    WithProvider(llmProvider).
	    WithMemory(memoryManager).
	    WithTools(toolManager).
	    Build()

	if err != nil {
	    log.Fatal(err)
	}

	// Execute a task
	output, err := agent.Execute(ctx, &agent.Input{
	    Query: "What is the weather in Beijing?",
	})

# Agent Types

The framework supports multiple agent types:

  - TypeReAct: Reasoning and Acting pattern
  - TypeCoT: Chain-of-Thought reasoning
  - TypePlanAndExecute: Planning then execution
  - TypeReflection: Self-reflection and improvement
  - TypeCustom: User-defined agent logic

# State Machine

Agents follow a well-defined state machine:

	Idle → Running → Completed
	         ↓
	       Failed

State transitions are validated to ensure correct agent behavior.

# Checkpointing

The framework supports checkpointing for long-running tasks:

	// Enable checkpointing
	agent.EnableCheckpointing(checkpointManager)

	// Recover from checkpoint
	agent.RecoverFromCheckpoint(ctx, checkpointID)

# Error Handling

The package defines structured errors with error codes:

	var (
	    ErrProviderNotSet = NewError(ErrCodeProviderNotSet, "LLM provider not configured")
	    ErrAgentNotReady  = NewError(ErrCodeNotReady, "agent not in ready state")
	    ErrAgentBusy      = NewError(ErrCodeBusy, "agent is busy executing another task")
	)

# Thread Safety

All agent implementations are designed to be thread-safe. The BaseAgent uses
appropriate synchronization primitives to protect shared state.

# Extensibility

The framework is designed for extensibility:
  - Custom agent types via AgentFactory registration
  - Custom validators via Validator interface
  - Custom memory stores via MemoryStore interface
  - Custom tools via Tool interface

See the subpackages for additional functionality:
  - agent/guardrails: Input/output validation and security
  - agent/memory: Memory management systems
  - agent/evaluation: Agent evaluation and A/B testing
  - agent/structured: Structured output parsing
  - agent/protocol/a2a: Agent-to-Agent communication
*/
package agent

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

//go:embed prompts/routing.txt
var routingPromptTemplate string

// RoutingAgent analyzes conversation history and determines the appropriate workflow intent
type RoutingAgent struct {
	config *AgentConfig
}

// NewRoutingAgent creates a new routing agent
func NewRoutingAgent(opts ...AgentOption) *RoutingAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	return &RoutingAgent{
		config: config,
	}
}

// RouteMessage analyzes the conversation context and determines the appropriate intent
func (a *RoutingAgent) RouteMessage(ctx context.Context) (*types.RoutingResult, error) {
	// Create a new conversation buffer for routing analysis
	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Copy conversation history to routing buffer
	for _, msg := range a.config.messages {
		conversationBuffer.ChatHistory.AddMessage(ctx, msg)
	}

	// Add plan context if available
	if a.config.plan != nil {
		planJsonBytes, err := json.MarshalIndent(a.config.plan, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current plan to JSON: %w", err)
		}

		err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: fmt.Sprintf("Current plan context: \n```json\n%s```\n", string(planJsonBytes)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add plan context to routing buffer: %w", err)
		}
	} else {
		err := conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: "Current plan context: No active plan exists.",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add no-plan context to routing buffer: %w", err)
		}
	}

	// Add available tools context
	var toolNames []string
	for _, tool := range a.config.tools {
		toolNames = append(toolNames, tool.Name())
	}
	toolsJsonBytes, err := json.MarshalIndent(toolNames, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tools to JSON: %w", err)
	}

	err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
		Content: fmt.Sprintf("Available tools: \n```json\n%s```\n", string(toolsJsonBytes)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add tools context to routing buffer: %w", err)
	}

	// Create prompt builder for routing
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(routingPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversationBuffer(conversationBuffer),
	)

	// Run routing evaluation
	routingResult, err := a.evaluateRouting(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate routing: %w", err)
	}

	return routingResult, nil
}

func (a *RoutingAgent) evaluateRouting(ctx context.Context, promptBuilder *PromptBuilder) (*types.RoutingResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build routing messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate routing response: %w", err)
	}
	a.config.callbacksHandler.HandleLLMGenerateContentEnd(ctx, response)

	// Extract text from response
	var responseText string
	if len(response.Choices) > 0 {
		responseText = response.Choices[0].Content
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from LLM")
	}

	// Parse JSON response using the routing prompt format
	var routingEvalResponse *types.RoutingResult
	if err := unmarshalJSONResponse(responseText, &routingEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse routing response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.RoutingResult{
		Intent:     routingEvalResponse.Intent,
		Confidence: routingEvalResponse.Confidence,
		Reasoning:  routingEvalResponse.Reasoning,
	}, nil
}

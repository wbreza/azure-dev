// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	langchainmemory "github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
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
	conversationBuffer := langchainmemory.NewConversationBuffer(
		langchainmemory.WithChatHistory(a.config.conversation.ChatHistory),
	)

	planContext := "Current Plan: No active plan exists"
	if a.config.plan != nil {
		planJsonBytes, err := json.MarshalIndent(a.config.plan, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current plan to JSON: %w", err)
		}

		planContext = fmt.Sprintf("Current Plan: \n```json\n%s```\n", string(planJsonBytes))
	}

	fullSystemMessage := fmt.Sprintf("%s\n\n%s", routingPromptTemplate, planContext)

	// Create prompt builder for routing
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(fullSystemMessage),
		WithPromptTools(a.config.tools),
		WithConversation(conversationBuffer),
	)

	// Run routing evaluation
	routingResult, err := a.evaluateRouting(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate routing: %w", err)
	}

	a.config.callbacksHandler.HandleAgentFinish(ctx, schema.AgentFinish{
		Log: routingResult.Message,
	})

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
		Message:    routingEvalResponse.Message,
	}, nil
}

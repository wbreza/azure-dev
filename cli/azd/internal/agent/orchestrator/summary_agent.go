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

//go:embed prompts/summary.txt
var summaryPromptTemplate string

// SummaryAgent creates concise summaries of various objects and data structures
type SummaryAgent struct {
	config *AgentConfig
}

// NewSummaryAgent creates a new summary agent
func NewSummaryAgent(opts ...AgentOption) *SummaryAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	return &SummaryAgent{
		config: config,
	}
}

// Summarize analyzes an object and creates a concise summary
func (a *SummaryAgent) Summarize(ctx context.Context, description string, obj interface{}) (*types.SummaryResult, error) {
	// Create a new conversation buffer for summarization
	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Marshal object to JSON for analysis
	objJsonBytes, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object to JSON: %w", err)
	}

	// Add description and object context to conversation
	content := fmt.Sprintf("Summary Instructions: %s\n\nObject to summarize: \n```json\n%s```\n", description, string(objJsonBytes))
	err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add object context to conversation: %w", err)
	}

	// Create prompt builder for summarization
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(summaryPromptTemplate),
		WithConversation(conversationBuffer),
	)

	// Run summarization evaluation
	evalResult, err := a.evaluateSummary(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate summary: %w", err)
	}

	// Create summary result
	summaryResult := &types.SummaryResult{
		Summary: evalResult.Summary,
	}

	return summaryResult, nil
}

func (a *SummaryAgent) evaluateSummary(ctx context.Context, promptBuilder *PromptBuilder) (*types.SummaryResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build summary messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate summary response: %w", err)
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

	// Parse JSON response using the summary prompt format
	var summaryEvalResponse *types.SummaryResult
	if err := unmarshalJSONResponse(responseText, &summaryEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse summary response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.SummaryResult{
		Summary: summaryEvalResponse.Summary,
	}, nil
}

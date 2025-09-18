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

//go:embed prompts/progress.txt
var progressPromptTemplate string

// ProgressAgent analyzes plan progress and generates status summaries
type ProgressAgent struct {
	config *AgentConfig
}

// NewProgressAgent creates a new progress agent
func NewProgressAgent(opts ...AgentOption) *ProgressAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	return &ProgressAgent{
		config: config,
	}
}

// GenerateProgress analyzes a plan and creates a comprehensive progress summary
func (a *ProgressAgent) GenerateProgress(ctx context.Context) (*types.ProgressResult, error) {
	if a.config.plan == nil {
		return &types.ProgressResult{
			Progress: "No active plan to analyze. Ready to start new work when you provide a goal or task.",
		}, nil
	}

	// Create a new conversation buffer for progress analysis
	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Marshal plan to JSON for analysis
	planJsonBytes, err := json.MarshalIndent(a.config.plan, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal plan to JSON: %w", err)
	}

	// Add plan context to conversation
	err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.HumanChatMessage{
		Content: fmt.Sprintf("Plan to analyze for progress: \n```json\n%s```\n", string(planJsonBytes)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add plan context to conversation: %w", err)
	}

	// Create prompt builder for progress analysis
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(progressPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversationBuffer(conversationBuffer),
	)

	// Run progress evaluation
	progressResult, err := a.evaluateProgress(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate progress: %w", err)
	}

	return progressResult, nil
}

func (a *ProgressAgent) evaluateProgress(ctx context.Context, promptBuilder *PromptBuilder) (*types.ProgressResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build progress messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate progress response: %w", err)
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

	// Parse JSON response using the progress prompt format
	var progressEvalResponse *types.ProgressResult
	if err := unmarshalJSONResponse(responseText, &progressEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse progress response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.ProgressResult{
		Progress: progressEvalResponse.Progress,
	}, nil
}

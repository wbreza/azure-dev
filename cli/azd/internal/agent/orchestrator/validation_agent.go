// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

//go:embed prompts/validation.txt
var validationPromptTemplate string

// ValidationAgent validates execution results and provides guidance for next steps
type ValidationAgent struct {
	config *AgentConfig
}

// NewValidationAgentWithSharedComponents creates a new validation agent with shared conversation buffer and working memory
func NewValidationAgent(opts ...AgentOption) *ValidationAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	return &ValidationAgent{
		config: config,
	}
}

// ValidateTask analyzes the results of a task execution and provides validation
func (a *ValidationAgent) ValidateTask(ctx context.Context, taskResult *types.TaskExecutionResult) (*types.TaskValidationResult, error) {
	// Update task status to validating
	taskResult.Task.Status = types.TaskValidating
	taskResult.Task.UpdatedAt = time.Now()

	// Create a new conversation buffer for validation
	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Marshal task to JSON for context
	taskJsonBytes, err := json.MarshalIndent(taskResult.Task, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal task to JSON: %w", err)
	}

	// Add task context to conversation
	err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
		Content: fmt.Sprintf("Task to validate: \n```json\n%s```\n", string(taskJsonBytes)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add task context to conversation: %w", err)
	}

	// Add execution evaluation context if available
	if taskResult.Evaluation != nil {
		evalJsonBytes, err := json.MarshalIndent(taskResult.Evaluation, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal evaluation to JSON: %w", err)
		}

		err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: fmt.Sprintf("Task execution evaluation: \n```json\n%s```\n", string(evalJsonBytes)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add evaluation context to conversation: %w", err)
		}
	}

	// Add tool call results to conversation
	for _, toolResult := range taskResult.ToolCalls {
		err := conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: fmt.Sprintf(
				"Tool call executed: '%s' with input: %s",
				toolResult.ToolCall.Tool,
				toolResult.ToolCall.Input, // TODO: Truncate if too long
			),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add tool call message to conversation: %w", err)
		}

		var toolResultContent string
		if toolResult.Error != "" {
			toolResultContent = fmt.Sprintf("Tool execution error: %s", toolResult.Error)
		} else {
			toolResultContent = toolResult.Output
		}

		err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.ToolChatMessage{
			Content: toolResultContent,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to add tool result message to conversation: %w", err)
		}
	}

	// Create prompt builder for validation
	promptBuilder := NewConversationalPromptBuilder(
		WithSystemPrompt(validationPromptTemplate),
		WithPromptTools(a.config.tools),
		WithWorkingMemory(StandardWorkingMemoryFormatter),
		WithConversationBuffer(conversationBuffer),
	)

	// Run evaluation
	evalResult, err := a.evaluateTaskValidation(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate task validation: %w", err)
	}

	// Create validation result
	validationResult := &types.TaskValidationResult{
		TaskExecutionResult: taskResult,
		Evaluation:          evalResult,
	}

	return validationResult, nil
}

func (a *ValidationAgent) evaluateTaskValidation(ctx context.Context, promptBuilder *ConversationalPromptBuilder) (*types.TaskValidationEvalResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := promptBuilder.BuildMessages(ctx, a.config.workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build validation messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate validation response: %w", err)
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

	// Parse JSON response using the validation prompt format
	var validationEvalResponse *types.TaskValidationEvalResult
	if err := unmarshalJSONResponse(responseText, &validationEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse validation response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.TaskValidationEvalResult{
		Summary:         validationEvalResponse.Summary,
		Status:          validationEvalResponse.Status,
		Insights:        validationEvalResponse.Insights,
		Recommendations: validationEvalResponse.Recommendations,
	}, nil
}

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
)

//go:embed prompts/validation.txt
var validationPromptTemplate string

// ValidationAgent validates execution results and provides guidance for next steps
type ValidationAgent struct {
	config        *AgentConfig
	promptBuilder *ConversationalPromptBuilder
}

// NewValidationAgentWithSharedComponents creates a new validation agent with shared conversation buffer and working memory
func NewValidationAgent(opts ...AgentOption) *ValidationAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	// Use the embedded prompt template as the system prompt (contains JSON schema and core instructions)
	promptBuilder := NewConversationalPromptBuilder(
		WithSystemPrompt(validationPromptTemplate),
		WithWorkingMemory(DetailedWorkingMemoryFormatter),
		WithConversationBuffer(config.conversationBuffer),
	)

	return &ValidationAgent{
		config:        config,
		promptBuilder: promptBuilder,
	}
}

// ValidateExecution analyzes the results of executed actions and provides validation
func (a *ValidationAgent) ValidateExecution(ctx context.Context, workingMemory *memory.WorkingMemory, actionResults []types.ActionResult) (*types.ValidationResult, error) {
	// Get validation response from LLM
	response, err := a.getValidationResponse(ctx, workingMemory, actionResults)
	if err != nil {
		return nil, fmt.Errorf("failed to get validation response: %w", err)
	}

	return response, nil
}

// Private methods

func (a *ValidationAgent) getValidationResponse(ctx context.Context, workingMemory *memory.WorkingMemory, actionResults []types.ActionResult) (*types.ValidationResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := a.promptBuilder.BuildMessages(ctx, workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build validation messages: %w", err)
	}

	// Format action results for the human message
	var resultsBuilder strings.Builder
	resultsBuilder.WriteString("Execution Results to Validate:\n\n")

	for i, result := range actionResults {
		resultsBuilder.WriteString(fmt.Sprintf("%d. Tool: %s\n", i+1, result.Tool))
		resultsBuilder.WriteString(fmt.Sprintf("   Input: %s\n", result.Input))
		if result.Error != "" {
			resultsBuilder.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
		} else {
			resultsBuilder.WriteString(fmt.Sprintf("   Output: %s\n", result.Output))
		}
		resultsBuilder.WriteString(fmt.Sprintf("   Duration: %s\n\n", result.Duration))
	}

	resultsBuilder.WriteString("Please analyze these results and provide a validation assessment with task status updates and next step recommendations.")

	// Add the validation request as human message
	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(resultsBuilder.String())},
	})

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

	// Parse JSON response (handle markdown-formatted JSON)
	var validationResult types.ValidationResult
	if err := unmarshalJSONResponse(responseText, &validationResult); err != nil {
		return nil, fmt.Errorf("failed to parse validation response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &validationResult, nil
}

// ApplyValidationResults updates working memory based on validation results
func (a *ValidationAgent) ApplyValidationResults(workingMemory *memory.WorkingMemory, validation *types.ValidationResult) error {
	// Update task statuses based on validation
	for _, taskUpdate := range validation.ProgressAssessment.TaskUpdates {
		err := workingMemory.UpdateTaskStatus(taskUpdate.TaskID, taskUpdate.NewStatus, taskUpdate.Evidence)
		if err != nil {
			return fmt.Errorf("failed to update task %s status: %w", taskUpdate.TaskID, err)
		}
	}

	// Record validation insights
	for _, insight := range validation.Insights {
		workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "validation_insight",
			Details: insight,
		})
	}

	// Record validation result
	workingMemory.AddEvent(memory.ExecutionEvent{
		Type:    "validation_result",
		Details: fmt.Sprintf("Result: %s, Progress: %s", validation.ValidationResult, validation.ProgressAssessment.OverallProgress),
	})

	return nil
}

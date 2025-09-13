// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
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

// ValidateTask analyzes the results of a task execution and provides validation
func (a *ValidationAgent) ValidateTask(ctx context.Context, task *types.Task, taskResult *types.TaskExecutionResult) (*types.ValidationResult, error) {
	// Get validation response from LLM using the complete task definition and execution result
	response, err := a.getValidationResponse(ctx, task, taskResult)
	if err != nil {
		return nil, fmt.Errorf("failed to get validation response: %w", err)
	}

	return response, nil
}

// Private methods

func (a *ValidationAgent) getValidationResponse(ctx context.Context, task *types.Task, taskResult *types.TaskExecutionResult) (*types.ValidationResult, error) {
	// Build validation context using the complete task definition and execution results
	var contextBuilder strings.Builder
	contextBuilder.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	contextBuilder.WriteString(fmt.Sprintf("Task Description: %s\n", task.Description))

	// Include task validation criteria
	if task.ValidationCriteria != "" {
		contextBuilder.WriteString(fmt.Sprintf("Validation Criteria: %s\n", task.ValidationCriteria))
	}

	// Include task rules if any
	if len(task.Rules) > 0 {
		contextBuilder.WriteString("Task Rules:\n")
		for _, rule := range task.Rules {
			contextBuilder.WriteString(fmt.Sprintf("- %s\n", rule))
		}
	}

	// Include planned tool calls for comparison
	if len(task.ToolCalls) > 0 {
		contextBuilder.WriteString("Planned Tool Calls:\n")
		for i, plannedCall := range task.ToolCalls {
			contextBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, plannedCall.ToolName))
			if plannedCall.Reasoning != "" {
				contextBuilder.WriteString(fmt.Sprintf("   Reasoning: %s\n", plannedCall.Reasoning))
			}
		}
	}

	contextBuilder.WriteString(fmt.Sprintf("\nExecution Status: %s\n\n", taskResult.Status))

	// Include task execution reasoning and evidence
	if taskResult.Reasoning != "" {
		contextBuilder.WriteString(fmt.Sprintf("Execution Reasoning: %s\n\n", taskResult.Reasoning))
	}
	if len(taskResult.Evidence) > 0 {
		contextBuilder.WriteString("Evidence from execution:\n")
		for _, evidence := range taskResult.Evidence {
			contextBuilder.WriteString(fmt.Sprintf("- %s\n", evidence))
		}
		contextBuilder.WriteString("\n")
	}

	// Format tool call results for validation
	contextBuilder.WriteString("Tool Execution Results:\n\n")
	for i, result := range taskResult.ToolCalls {
		contextBuilder.WriteString(fmt.Sprintf("%d. Tool: %s\n", i+1, result.Tool))

		// Format input based on its type
		var inputStr string
		if result.Input == nil {
			inputStr = "null"
		} else {
			inputBytes, err := json.MarshalIndent(result.Input, "   ", "  ")
			if err != nil {
				inputStr = fmt.Sprintf("%v", result.Input)
			} else {
				inputStr = string(inputBytes)
			}
		}
		contextBuilder.WriteString(fmt.Sprintf("   Input: %s\n", inputStr))

		if result.Error != "" {
			contextBuilder.WriteString(fmt.Sprintf("   Error: %s\n", result.Error))
		} else {
			contextBuilder.WriteString(fmt.Sprintf("   Output: %s\n", result.Output))
		}
		contextBuilder.WriteString(fmt.Sprintf("   Duration: %s\n\n", result.Duration))
	}

	contextBuilder.WriteString("Please analyze these results and validate whether the task goal has been achieved. Provide a detailed validation assessment with task status updates and next step recommendations.")

	// Create messages for the LLM call
	messages := []llms.MessageContent{
		{
			Role:  llms.ChatMessageTypeSystem,
			Parts: []llms.ContentPart{llms.TextPart(validationPromptTemplate)},
		},
	}

	// If the task has a conversation buffer, include recent messages for context
	if taskResult.ConversationBuffer != nil {
		// Extract recent conversation history for context
		chatHistory := taskResult.ConversationBuffer.ChatHistory
		if bufferMemory, ok := chatHistory.(*langchainmemory.ChatMessageHistory); ok {
			recentMessages, err := bufferMemory.Messages(ctx)
			if err == nil && len(recentMessages) > 0 {
				// Include last few messages for context (limit to avoid token overflow)
				maxMessages := 10
				startIdx := 0
				if len(recentMessages) > maxMessages {
					startIdx = len(recentMessages) - maxMessages
				}

				for _, msg := range recentMessages[startIdx:] {
					var role llms.ChatMessageType
					switch msg.GetType() {
					case "human":
						role = llms.ChatMessageTypeHuman
					case "ai":
						role = llms.ChatMessageTypeAI
					default:
						continue
					}

					messages = append(messages, llms.MessageContent{
						Role:  role,
						Parts: []llms.ContentPart{llms.TextPart(msg.GetContent())},
					})
				}
			}
		}
	}

	// Add the validation request as the final human message
	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(contextBuilder.String())},
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

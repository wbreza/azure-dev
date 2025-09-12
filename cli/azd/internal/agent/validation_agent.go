// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package agent

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/prompts"
)

//go:embed prompts/validation.txt
var validationPromptTemplate string

// ValidationAgent validates execution results and provides guidance for next steps
type ValidationAgent struct {
	llm            llms.Model
	promptTemplate prompts.PromptTemplate
}

// NewValidationAgent creates a new validation agent
func NewValidationAgent(llm llms.Model) *ValidationAgent {
	return &ValidationAgent{
		llm: llm,
		promptTemplate: prompts.PromptTemplate{
			Template:       validationPromptTemplate,
			TemplateFormat: prompts.TemplateFormatGoTemplate,
			InputVariables: []string{"goal", "taskStatus", "actionResults", "workingMemoryContext"},
		},
	}
}

// ValidateExecution analyzes the results of executed actions and provides validation
func (v *ValidationAgent) ValidateExecution(ctx context.Context, workingMemory *memory.WorkingMemory, actionResults []types.ActionResult) (*types.ValidationResult, error) {
	// Build validation context
	context := v.buildValidationContext(workingMemory, actionResults)

	// Get validation response from LLM
	response, err := v.getValidationResponse(ctx, context)
	if err != nil {
		return nil, fmt.Errorf("failed to get validation response: %w", err)
	}

	return response, nil
}

// Private methods

func (v *ValidationAgent) buildValidationContext(workingMemory *memory.WorkingMemory, actionResults []types.ActionResult) map[string]any {
	taskStatus := workingMemory.GetTaskStatusSummary()

	// Format action results for prompt
	var resultsBuilder strings.Builder
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

	// Get working memory context
	recentHistory := workingMemory.GetRecentHistory(10)
	var contextBuilder strings.Builder
	contextBuilder.WriteString("Recent execution history:\n")
	for _, event := range recentHistory {
		contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", event.Type, event.Details))
	}

	return map[string]any{
		"goal":                 workingMemory.GetGoal(),
		"taskStatus":           taskStatus.ToPromptFormat(),
		"actionResults":        resultsBuilder.String(),
		"workingMemoryContext": contextBuilder.String(),
	}
}

func (v *ValidationAgent) getValidationResponse(ctx context.Context, context map[string]any) (*types.ValidationResult, error) {
	// Generate prompt
	prompt, err := v.promptTemplate.Format(context)
	if err != nil {
		return nil, fmt.Errorf("failed to format validation prompt: %w", err)
	}

	// Get response from LLM
	response, err := llms.GenerateFromSinglePrompt(ctx, v.llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate validation response: %w", err)
	}

	// Parse JSON response (handle markdown-formatted JSON)
	var validationResult types.ValidationResult
	if err := unmarshalJSONResponse(response, &validationResult); err != nil {
		return nil, fmt.Errorf("failed to parse validation response as JSON: %w\nResponse: %s", err, response)
	}

	return &validationResult, nil
}

// ApplyValidationResults updates working memory based on validation results
func (v *ValidationAgent) ApplyValidationResults(workingMemory *memory.WorkingMemory, validation *types.ValidationResult) error {
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

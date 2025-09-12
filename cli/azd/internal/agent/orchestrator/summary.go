// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
)

// summarizeExecution creates a natural, conversational summary of task execution results
func summarizeExecution(ctx context.Context, llm llms.Model, result *types.ExecutionResult) (string, error) {
	// For simple message responses, return as-is since they're already conversational
	if result.Status == "message" {
		return result.Reason, nil
	}

	// Build execution summary for task-based responses
	var summaryBuilder strings.Builder

	// Basic execution info
	summaryBuilder.WriteString(fmt.Sprintf("Goal: %s\n", result.Goal))
	summaryBuilder.WriteString(fmt.Sprintf("Status: %s\n", result.Status))
	summaryBuilder.WriteString(fmt.Sprintf("Iterations: %d\n", result.TotalIterations))

	if result.Reason != "" {
		summaryBuilder.WriteString(fmt.Sprintf("Outcome: %s\n", result.Reason))
	}

	// Add task details if available
	if taskSummary, ok := result.TasksSummary.(memory.TaskStatusDisplay); ok {
		if len(taskSummary.CompletedTasks) > 0 {
			summaryBuilder.WriteString("\nCompleted Tasks:\n")
			for _, task := range taskSummary.CompletedTasks {
				summaryBuilder.WriteString(fmt.Sprintf("- %s: %s\n", task.ID, task.Brief))
			}
		}

		if len(taskSummary.PendingTasks) > 0 {
			summaryBuilder.WriteString("\nPending Tasks:\n")
			for _, task := range taskSummary.PendingTasks {
				summaryBuilder.WriteString(fmt.Sprintf("- %s: %s\n", task.ID, task.Brief))
			}
		}
	}

	// Create prompt for natural summary
	prompt := fmt.Sprintf(`You are an Azure Developer CLI assistant. Please convert this execution summary into a natural, conversational response for the user.

EXECUTION SUMMARY:
%s

Please provide a friendly, conversational summary that:
- Sounds natural and human
- Focuses on what was accomplished for the user
- Avoids technical jargon like "iterations" or "task IDs"
- Is encouraging and helpful
- If something failed, be empathetic and suggest next steps

Respond in a conversational tone as if you're talking directly to the user.`, summaryBuilder.String())

	// Get natural response from LLM
	response, err := llms.GenerateFromSinglePrompt(ctx, llm, prompt)
	if err != nil {
		// If summary fails, fall back to basic formatting
		return fmt.Sprintf("Completed your request: %s", result.Goal), nil
	}

	return strings.TrimSpace(response), nil
}

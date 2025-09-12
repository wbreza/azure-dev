// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	uxlib "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
	"github.com/tmc/langchaingo/llms"
)

// EnhancedAzdAiAgent represents an enhanced agent that uses the sophisticated
// ReAct orchestrator with planning, execution, and validation loops
type EnhancedAzdAiAgent struct {
	*agentBase
	orchestrator *EnhancedReActOrchestrator
}

// NewEnhancedAzdAiAgent creates a new enhanced agent that uses the sophisticated
// ReAct orchestrator instead of the standard langchain executor. This provides
// advanced capabilities like task planning, validation loops, and persistent working memory.
func NewEnhancedAzdAiAgent(llm llms.Model, opts ...AgentCreateOption) (Agent, error) {
	azdAgent := &EnhancedAzdAiAgent{
		agentBase: &agentBase{
			defaultModel: llm,
			tools:        []common.AnnotatedTool{},
		},
	}

	for _, opt := range opts {
		opt(azdAgent.agentBase)
	}

	// Default max iterations for the orchestrator
	if azdAgent.maxIterations <= 0 {
		azdAgent.maxIterations = 20 // More conservative than conversational agent
	}

	// Create orchestrator configuration
	config := OrchestratorConfig{
		MaxIterations:     azdAgent.maxIterations,
		MaxFailedCycles:   3,
		EnableDeepThought: true,
	}

	// Create the enhanced orchestrator
	azdAgent.orchestrator = NewEnhancedReActOrchestrator(llm, azdAgent.tools, config)

	return azdAgent, nil
}

// SendMessage processes a single message through the enhanced agent and returns the response
func (eai *EnhancedAzdAiAgent) SendMessage(ctx context.Context, args ...string) (string, error) {
	goal := strings.Join(args, "\n")

	// Start thinking UI
	thoughtsCtx, cancelCtx := context.WithCancel(ctx)
	cleanup, err := eai.renderThoughts(thoughtsCtx)
	if err != nil {
		cancelCtx()
		return "", err
	}

	defer func() {
		cleanup()
		cancelCtx()
	}()

	// Execute using the enhanced orchestrator
	result, err := eai.orchestrator.Execute(ctx, goal)
	if err != nil {
		return "", fmt.Errorf("enhanced agent execution failed: %w", err)
	}

	// Generate natural summary of the execution
	summary, err := summarizeExecution(ctx, eai.defaultModel, result)
	if err != nil {
		// If summary fails, fall back to the basic formatted result
		return eai.formatExecutionResult(result), nil
	}

	return summary, nil
}

// formatExecutionResult converts the execution result into a user-friendly response
func (eai *EnhancedAzdAiAgent) formatExecutionResult(result *types.ExecutionResult) string {
	var response strings.Builder

	switch result.Status {
	case "message":
		// Simple message response - just return the message
		return result.Reason

	case "success":
		response.WriteString("✅ **Goal Achieved Successfully**\n\n")
		response.WriteString(fmt.Sprintf("**Goal:** %s\n\n", result.Goal))

		// Show task summary
		if taskSummary, ok := result.TasksSummary.(memory.TaskStatusDisplay); ok {
			if len(taskSummary.CompletedTasks) > 0 {
				response.WriteString("**Completed Tasks:**\n")
				for _, task := range taskSummary.CompletedTasks {
					response.WriteString(fmt.Sprintf("- ✓ %s: %s\n", task.ID, task.Brief))
				}
				response.WriteString("\n")
			}
		}

	case "failed":
		response.WriteString("❌ **Goal Failed**\n\n")
		response.WriteString(fmt.Sprintf("**Goal:** %s\n", result.Goal))
		response.WriteString(fmt.Sprintf("**Reason:** %s\n\n", result.Reason))

	case "timeout":
		response.WriteString("⏱️ **Goal Timed Out**\n\n")
		response.WriteString(fmt.Sprintf("**Goal:** %s\n", result.Goal))
		response.WriteString(fmt.Sprintf("**Iterations:** %d\n", result.TotalIterations))
		response.WriteString("The agent reached the maximum number of iterations before completing the goal.\n\n")

	default:
		response.WriteString(fmt.Sprintf("**Status:** %s\n", result.Status))
		response.WriteString(fmt.Sprintf("**Goal:** %s\n", result.Goal))
		response.WriteString(fmt.Sprintf("**Reason:** %s\n\n", result.Reason))
	}

	// Add execution summary for task-based responses
	response.WriteString(fmt.Sprintf("**Total Iterations:** %d\n", result.TotalIterations))

	return response.String()
}

// renderThoughts provides visual feedback during agent execution
func (eai *EnhancedAzdAiAgent) renderThoughts(ctx context.Context) (func(), error) {
	var currentPhase string
	var currentTask string

	spinner := uxlib.NewSpinner(&uxlib.SpinnerOptions{
		Text: "Initializing enhanced agent...",
	})

	canvas := uxlib.NewCanvas(
		spinner,
		uxlib.NewVisualElement(func(printer uxlib.Printer) error {
			printer.Fprintln()
			printer.Fprintln()

			if currentPhase != "" {
				printer.Fprintln(color.HiBlueString("Phase: %s", currentPhase))
			}
			if currentTask != "" {
				printer.Fprintln(color.HiBlackString("Task: %s", currentTask))
			}
			if currentPhase != "" || currentTask != "" {
				printer.Fprintln()
				printer.Fprintln()
			}

			return nil
		}))

	go func() {
		defer canvas.Clear()

		phaseIndex := 0
		phases := []string{
			"Creating execution plan",
			"Executing planned actions",
			"Validating progress",
			"Replanning if needed",
			"Finalizing results",
		}

		for {
			select {
			case thought := <-eai.thoughtChan:
				if thought.Action != "" {
					currentTask = fmt.Sprintf("Running %s", color.GreenString(thought.Action))
					if thought.ActionInput != "" {
						currentTask += fmt.Sprintf(" with %s", color.GreenString(thought.ActionInput))
					}
				}
				if thought.Thought != "" {
					currentTask = thought.Thought
				}
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
				// Cycle through phases to show progress
				if phaseIndex < len(phases) {
					currentPhase = phases[phaseIndex]
					phaseIndex = (phaseIndex + 1) % len(phases)
				}
			}

			// Update spinner text based on current activity
			var spinnerText string
			if currentTask != "" {
				spinnerText = currentTask
			} else if currentPhase != "" {
				spinnerText = currentPhase + "..."
			} else {
				spinnerText = "Processing with enhanced agent..."
			}

			spinner.UpdateText(spinnerText)
			canvas.Update()
		}
	}()

	cleanup := func() {
		canvas.Clear()
		canvas.Close()
	}

	return cleanup, canvas.Run()
}

// GetWorkingMemory returns the orchestrator's working memory for debugging/inspection
func (eai *EnhancedAzdAiAgent) GetWorkingMemory() *memory.WorkingMemory {
	return eai.orchestrator.GetWorkingMemory()
}

// Reset clears the orchestrator's working memory for a new conversation
func (eai *EnhancedAzdAiAgent) Reset() {
	eai.orchestrator.Reset()
}

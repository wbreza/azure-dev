// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package agent

import (
	"context"
	"fmt"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
)

// EnhancedReActOrchestrator manages the enhanced ReAct loop with planning, execution, and validation
type EnhancedReActOrchestrator struct {
	planningAgent   *PlanningAgent
	executionAgent  *ExecutionAgent
	validationAgent *ValidationAgent
	workingMemory   *memory.WorkingMemory

	// Configuration
	maxIterations     int
	maxFailedCycles   int
	enableDeepThought bool
}

// OrchestratorConfig contains configuration for the orchestrator
type OrchestratorConfig struct {
	MaxIterations     int  // Maximum number of ReAct cycles
	MaxFailedCycles   int  // Maximum consecutive failed cycles before giving up
	EnableDeepThought bool // Whether to enable deep thought mode for complex problems
}

// DefaultOrchestratorConfig returns sensible default configuration
func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		MaxIterations:     20,
		MaxFailedCycles:   3,
		EnableDeepThought: true,
	}
}

// NewEnhancedReActOrchestrator creates a new orchestrator
func NewEnhancedReActOrchestrator(llm llms.Model, tools []common.AnnotatedTool, config OrchestratorConfig) *EnhancedReActOrchestrator {
	return &EnhancedReActOrchestrator{
		planningAgent:     NewPlanningAgent(llm, tools),
		executionAgent:    NewExecutionAgent(llm, tools),
		validationAgent:   NewValidationAgent(llm),
		workingMemory:     memory.NewWorkingMemory(),
		maxIterations:     config.MaxIterations,
		maxFailedCycles:   config.MaxFailedCycles,
		enableDeepThought: config.EnableDeepThought,
	}
}

// Execute runs the enhanced ReAct loop to achieve the given goal
func (o *EnhancedReActOrchestrator) Execute(ctx context.Context, goal string) (*types.ExecutionResult, error) {
	// Initialize working memory with the goal
	o.workingMemory.SetGoal(goal)

	// Create initial plan
	planningResult, err := o.planningAgent.CreatePlan(ctx, goal, o.workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to create initial plan: %w", err)
	}

	// Handle different response types
	switch planningResult.ResponseType {
	case "message":
		// Simple message response - return immediately without task execution
		return &types.ExecutionResult{
			Status:          "message",
			Goal:            goal,
			Reason:          planningResult.Message,
			TotalIterations: 0,
			TasksSummary:    nil,
		}, nil

	case "tasks":
		// Task-based response - proceed with execution
		if planningResult.Plan == nil {
			return nil, fmt.Errorf("planning result has tasks response type but no execution plan")
		}

		o.workingMemory.SetExecutionPlan(planningResult.Plan)
		o.workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "initial_plan_created",
			Details: fmt.Sprintf("Created plan with %d tasks", len(planningResult.Plan.Tasks)),
		})

		// Continue with task execution
		return o.executeTaskBasedPlan(ctx, goal)

	default:
		return nil, fmt.Errorf("unknown planning response type: %s", planningResult.ResponseType)
	}
}

// executeTaskBasedPlan handles the main ReAct loop for task-based execution
func (o *EnhancedReActOrchestrator) executeTaskBasedPlan(ctx context.Context, goal string) (*types.ExecutionResult, error) {
	var (
		iteration      = 0
		failedCycles   = 0
		lastValidation *types.ValidationResult
	)

	for iteration < o.maxIterations {
		iteration++

		// Check if goal is achieved
		if o.isGoalAchieved() {
			break
		}

		// Plan/Replan if needed
		needsReplanning := o.needsReplanning(lastValidation, iteration)
		if needsReplanning {
			updatedPlan, err := o.planningAgent.UpdatePlan(ctx, o.workingMemory)
			if err != nil {
				return nil, fmt.Errorf("failed to update plan on iteration %d: %w", iteration, err)
			}
			o.workingMemory.SetExecutionPlan(updatedPlan)
			o.workingMemory.AddEvent(memory.ExecutionEvent{
				Type:    "plan_updated",
				Details: fmt.Sprintf("Updated plan on iteration %d", iteration),
			})
		}

		// Execute actions
		_, actionResults, err := o.executionAgent.ExecuteStep(ctx, o.workingMemory)
		if err != nil {
			failedCycles++
			o.workingMemory.AddEvent(memory.ExecutionEvent{
				Type:    "execution_failed",
				Details: fmt.Sprintf("Execution failed on iteration %d: %v", iteration, err),
			})

			if failedCycles >= o.maxFailedCycles {
				return o.buildResult("failed", fmt.Sprintf("Too many consecutive failures (%d)", failedCycles)), nil
			}
			continue
		}

		// Record execution
		o.workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "execution_completed",
			Details: fmt.Sprintf("Executed %d actions on iteration %d", len(actionResults), iteration),
		})

		// Validate results
		validation, err := o.validationAgent.ValidateExecution(ctx, o.workingMemory, actionResults)
		if err != nil {
			return nil, fmt.Errorf("failed to validate execution on iteration %d: %w", iteration, err)
		}

		// Apply validation results to working memory
		err = o.validationAgent.ApplyValidationResults(o.workingMemory, validation)
		if err != nil {
			return nil, fmt.Errorf("failed to apply validation results on iteration %d: %w", iteration, err)
		}

		lastValidation = validation

		// Handle validation results
		if validation.ValidationResult == "failure" {
			failedCycles++
			if failedCycles >= o.maxFailedCycles {
				return o.buildResult("failed", "Validation indicated failure too many times"), nil
			}
		} else {
			failedCycles = 0 // Reset failure counter on success
		}
	}

	// Determine final result
	if iteration >= o.maxIterations {
		return o.buildResult("timeout", "Maximum iterations reached"), nil
	}

	return o.buildResult("success", "Goal achieved"), nil
}

// Private methods

func (o *EnhancedReActOrchestrator) isGoalAchieved() bool {
	taskSummary := o.workingMemory.GetTaskStatusSummary()

	// Check if all tasks are completed - for now use simple completed vs pending count
	totalTasks := len(taskSummary.CompletedTasks) + len(taskSummary.PendingTasks)

	// Goal is achieved if we have tasks and all are completed
	return totalTasks > 0 && len(taskSummary.CompletedTasks) == totalTasks
}

func (o *EnhancedReActOrchestrator) needsReplanning(lastValidation *types.ValidationResult, iteration int) bool {
	// Always replan on first iteration if we don't have validation yet
	if lastValidation == nil && iteration == 1 {
		return false // Initial plan is already created
	}

	// Replan if validation explicitly recommends it
	if lastValidation != nil {
		return lastValidation.Recommendations.NextAction == "replan" ||
			lastValidation.ValidationResult == "needs_replanning"
	}

	// Replan every few iterations as a safety mechanism
	return iteration%5 == 0
}

func (o *EnhancedReActOrchestrator) buildResult(status, reason string) *types.ExecutionResult {
	taskSummary := o.workingMemory.GetTaskStatusSummary()
	history := o.workingMemory.GetRecentHistory(50)

	// Convert history to interface slice
	historyInterface := make([]interface{}, len(history))
	for i, event := range history {
		historyInterface[i] = event
	}

	return &types.ExecutionResult{
		Status:           status,
		Reason:           reason,
		Goal:             o.workingMemory.GetGoal(),
		TasksSummary:     taskSummary,
		TotalIterations:  len(o.workingMemory.GetRecentHistory(1000)), // Get all events as proxy for iterations
		ExecutionHistory: historyInterface,
	}
}

// GetWorkingMemory returns the current working memory (useful for debugging)
func (o *EnhancedReActOrchestrator) GetWorkingMemory() *memory.WorkingMemory {
	return o.workingMemory
}

// Reset clears the working memory for a new goal
func (o *EnhancedReActOrchestrator) Reset() {
	o.workingMemory = memory.NewWorkingMemory()
}

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	uxlib "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

// OrchestratorAgent manages the enhanced ReAct loop with planning, execution, and validation
type OrchestratorAgent struct {
	// Agent instances
	planningAgent   *PlanningAgent
	executionAgent  *ExecutionAgent
	validationAgent *ValidationAgent

	// Configuration
	config *AgentConfig
}

// NewOrchestratorAgent creates a new orchestrator
func NewOrchestratorAgent(opts ...AgentOption) *OrchestratorAgent {
	config := &AgentConfig{}

	for _, option := range opts {
		option(config)
	}

	if config.workingMemory == nil {
		config.workingMemory = memory.NewWorkingMemory()
	}

	if config.conversationBuffer == nil {
		config.conversationBuffer = langchainmemory.NewConversationBuffer()
	}

	return &OrchestratorAgent{
		config:          config,
		planningAgent:   NewPlanningAgent(WithConfig(config)),
		executionAgent:  NewExecutionAgent(WithConfig(config)),
		validationAgent: NewValidationAgent(WithConfig(config)),
	}
}

func (a *OrchestratorAgent) SendMessage(ctx context.Context, args ...string) (string, error) {
	userMessage := strings.Join(args, "\n")

	// Start thinking UI
	thoughtsCtx, cancelCtx := context.WithCancel(ctx)
	cleanup, err := a.renderThoughts(thoughtsCtx)
	if err != nil {
		cancelCtx()
		return "", err
	}

	defer func() {
		cleanup()
		cancelCtx()
	}()

	// Execute using the enhanced orchestrator
	result, err := a.execute(ctx, userMessage)
	if err != nil {
		return "", fmt.Errorf("enhanced agent execution failed: %w", err)
	}

	// Generate natural summary of the execution
	summary, err := summarizeExecution(ctx, a.config.model, result)
	if err != nil {
		// If summary fails, fall back to the basic formatted result
		return a.formatExecutionResult(result), nil
	}

	return summary, nil
}

// Stop terminates the agent and performs any necessary cleanup
func (a *OrchestratorAgent) Stop() error {
	if a.config.cleanupFunc != nil {
		return a.config.cleanupFunc()
	}

	return nil
}

// Execute runs the enhanced ReAct loop to achieve the given goal
func (a *OrchestratorAgent) execute(ctx context.Context, userMessage string) (*types.ExecutionResult, error) {
	// Append user message to conversation buffer
	if err := a.config.conversationBuffer.ChatHistory.AddUserMessage(ctx, userMessage); err != nil {
		return nil, fmt.Errorf("failed to add user message to chat history")
	}

	// Initialize working memory with the goal if not already set
	if a.config.workingMemory.GetGoal() == "" {
		a.config.workingMemory.SetGoal(userMessage)
	}

	// Create initial plan
	planningResult, err := a.planningAgent.CreatePlan(ctx, userMessage)
	if err != nil {
		return nil, fmt.Errorf("failed to create initial plan: %w", err)
	}

	if planningResult.Goal != "" {
		a.config.workingMemory.SetGoal(planningResult.Goal)
	}

	// Handle different response types
	switch planningResult.ResponseType {
	case "message":
		// Simple message response - return immediately without task execution
		result := &types.ExecutionResult{
			Status:          "message",
			Goal:            userMessage,
			Reason:          planningResult.Message,
			TotalIterations: 0,
			TasksSummary:    nil,
		}

		// Save response to conversation buffer
		if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, planningResult.Message); err != nil {
			// Log error but don't fail the request
			fmt.Printf("Warning: failed to save response to conversation: %v\n", err)
		}

		return result, nil

	case "tasks":
		// Task-based response - proceed with execution
		if planningResult.Plan == nil {
			return nil, fmt.Errorf("planning result has tasks response type but no execution plan")
		}

		a.config.workingMemory.SetExecutionPlan(planningResult.Plan)
		a.config.workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "initial_plan_created",
			Details: fmt.Sprintf("Created plan with %d tasks", len(planningResult.Plan.Tasks)),
		})

		// Continue with task execution
		return a.executeTaskBasedPlan(ctx)

	default:
		return nil, fmt.Errorf("unknown planning response type: %s", planningResult.ResponseType)
	}
}

// executeTaskBasedPlan handles the main ReAct loop for task-based execution
func (a *OrchestratorAgent) executeTaskBasedPlan(ctx context.Context) (*types.ExecutionResult, error) {
	plan := a.config.workingMemory.GetExecutionPlan()
	if plan == nil {
		return nil, fmt.Errorf("no execution plan available")
	}

	// Execute tasks in sequence
	for _, task := range plan.Tasks {
		if task.Status == types.TaskComplete {
			continue // Skip already completed tasks
		}

		// Delegate complete task execution to ExecutionAgent
		taskResult, err := a.executeTaskWithValidation(ctx, task)
		if err != nil {
			// Task execution failed, build failed result
			result := a.buildResult("failed", fmt.Sprintf("Task %s failed: %v", task.ID, err))
			if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, result.Reason); err != nil {
				fmt.Printf("Warning: failed to save failure result to conversation: %v\n", err)
			}
			return result, err
		}

		// Add task summary to high-level conversation
		taskSummary := fmt.Sprintf("Task %s completed: %s. Evidence: %s",
			taskResult.TaskID, taskResult.Reasoning, strings.Join(taskResult.Evidence, "; "))
		if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, taskSummary); err != nil {
			fmt.Printf("Warning: failed to add task summary to conversation: %v\n", err)
		}
	}

	// All tasks completed successfully
	result := a.buildResult("success", "All tasks completed successfully")
	if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, result.Reason); err != nil {
		fmt.Printf("Warning: failed to save success result to conversation: %v\n", err)
	}

	return result, nil
}

// executeTaskWithValidation executes a task using ExecutionAgent and validates the result
func (a *OrchestratorAgent) executeTaskWithValidation(ctx context.Context, task *types.Task) (*types.TaskExecutionResult, error) {
	maxValidationRetries := 3

	for validationRetry := 0; validationRetry < maxValidationRetries; validationRetry++ {
		// Mark task as in progress
		if err := a.config.workingMemory.UpdateTaskStatus(task.ID, "in_progress", "Starting task execution"); err != nil {
			return nil, fmt.Errorf("failed to update task status to in_progress: %w", err)
		}

		// Execute task using ExecutionAgent
		taskResult, err := a.executionAgent.ExecuteTask(ctx, task)
		if err != nil {
			return nil, fmt.Errorf("execution agent failed for task %s: %w", task.ID, err)
		}

		// Handle different execution results
		switch taskResult.Status {
		case "completed":
			// Validate the completion using complete task definition and execution result
			validation, err := a.validationAgent.ValidateTask(ctx, task, taskResult)
			if err != nil {
				return nil, fmt.Errorf("failed to validate task completion: %w", err)
			}

			if validation.ValidationResult == "success" {
				// Task is validated as complete
				err := a.config.workingMemory.UpdateTaskStatus(task.ID, "completed", strings.Join(taskResult.Evidence, "; "))
				if err != nil {
					return nil, fmt.Errorf("failed to mark task as completed: %w", err)
				}

				a.config.workingMemory.AddEvent(memory.ExecutionEvent{
					Type:    "task_completed",
					TaskID:  task.ID,
					Details: fmt.Sprintf("Task %s completed and validated", task.ID),
				})
				return taskResult, nil
			} else {
				// Validation failed, add context and retry
				validationContext := fmt.Sprintf("Task validation failed: %s. Retry %d/%d. Additional context: %s",
					validation.Reasoning, validationRetry+1, maxValidationRetries, strings.Join(validation.Insights, "; "))

				// Mark task as in progress again for retry
				if err := a.config.workingMemory.UpdateTaskStatus(task.ID, "in_progress", "Validation failed, retrying"); err != nil {
					return nil, fmt.Errorf("failed to update task status for retry: %w", err)
				}

				// If we have specific replanning recommendations, trigger replanning for this task
				if validation.Recommendations.NextAction == "replan" {
					err := a.replanTask(ctx, task, validation.Reasoning, validation.Recommendations.SpecificActions)
					if err != nil {
						return nil, fmt.Errorf("failed to replan task %s: %w", task.ID, err)
					}
				}

				// Add validation failure context to high-level conversation
				if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, validationContext); err != nil {
					fmt.Printf("Warning: failed to add validation context to conversation: %v\n", err)
				}

				continue // Retry the task
			}

		case "needs_replanning":
			// ExecutionAgent requests replanning
			err := a.replanTask(ctx, task, taskResult.ReplanReason, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to replan task %s: %w", task.ID, err)
			}

			// Add replanning context to high-level conversation
			replanContext := fmt.Sprintf("Task %s needs replanning: %s", task.ID, taskResult.ReplanReason)
			if err := a.config.conversationBuffer.ChatHistory.AddAIMessage(ctx, replanContext); err != nil {
				fmt.Printf("Warning: failed to add replan context to conversation: %v\n", err)
			}

			continue // Retry with replanned task

		case "failed":
			// ExecutionAgent failed the task
			if err := a.config.workingMemory.UpdateTaskStatus(task.ID, "failed", taskResult.Reasoning); err != nil {
				return nil, fmt.Errorf("failed to mark task as failed: %w", err)
			}
			return nil, fmt.Errorf("task %s failed: %s", task.ID, taskResult.Reasoning)

		default:
			return nil, fmt.Errorf("unknown task execution status: %s", taskResult.Status)
		}
	}

	// If we reach here, validation failed too many times
	if err := a.config.workingMemory.UpdateTaskStatus(task.ID, "failed", fmt.Sprintf("Validation failed %d times", maxValidationRetries)); err != nil {
		return nil, fmt.Errorf("failed to mark task as failed: %w", err)
	}

	return nil, fmt.Errorf("task %s failed validation %d times", task.ID, maxValidationRetries)
}

// replanTask replans a single task based on validation feedback
func (a *OrchestratorAgent) replanTask(ctx context.Context, task *types.Task, validationReasoning string, specificActions []string) error {
	// Build replanning context with specific actions if available
	replanContext := fmt.Sprintf("Replanning task %s due to validation failure: %s", task.ID, validationReasoning)
	if len(specificActions) > 0 {
		replanContext += "\n\nSpecific recommendations for replanning:"
		for _, action := range specificActions {
			replanContext += fmt.Sprintf("\n- %s", action)
		}
	}

	if err := a.config.conversationBuffer.ChatHistory.AddUserMessage(ctx, replanContext); err != nil {
		return fmt.Errorf("failed to add replan context to conversation: %w", err)
	}

	// Use planning agent to update just this task
	updatedPlan, err := a.planningAgent.UpdatePlan(ctx)
	if err != nil {
		return fmt.Errorf("failed to replan task: %w", err)
	}

	// Find the updated task in the new plan
	updatedTask := updatedPlan.GetTaskByID(task.ID)
	if updatedTask != nil {
		// Update the task in the current plan
		*task = *updatedTask

		// Build detailed replanning event message
		eventDetails := fmt.Sprintf("Task %s replanned due to validation failure", task.ID)
		if len(specificActions) > 0 {
			eventDetails += ". Specific actions incorporated:"
			for _, action := range specificActions {
				eventDetails += fmt.Sprintf(" • %s", action)
			}
		}

		a.config.workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "task_replanned",
			TaskID:  task.ID,
			Details: eventDetails,
		})
	}

	return nil
}

// Private methods
func (a *OrchestratorAgent) buildResult(status, reason string) *types.ExecutionResult {
	taskSummary := a.config.workingMemory.GetTaskStatusSummary()
	history := a.config.workingMemory.GetRecentHistory(50)

	// Convert history to interface slice
	historyInterface := make([]interface{}, len(history))
	for i, event := range history {
		historyInterface[i] = event
	}

	return &types.ExecutionResult{
		Status:           status,
		Reason:           reason,
		Goal:             a.config.workingMemory.GetGoal(),
		TasksSummary:     taskSummary,
		TotalIterations:  len(a.config.workingMemory.GetRecentHistory(1000)), // Get all events as proxy for iterations
		ExecutionHistory: historyInterface,
	}
}

// formatExecutionResult converts the execution result into a user-friendly response
func (a *OrchestratorAgent) formatExecutionResult(result *types.ExecutionResult) string {
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

func (a *OrchestratorAgent) renderThoughts(ctx context.Context) (func(), error) {
	var latestThought string

	spinner := uxlib.NewSpinner(&uxlib.SpinnerOptions{
		Text: "Thinking...",
	})

	canvas := uxlib.NewCanvas(
		spinner,
		uxlib.NewVisualElement(func(printer uxlib.Printer) error {
			printer.Fprintln()
			printer.Fprintln()

			if latestThought != "" {
				printer.Fprintln(color.HiBlackString(latestThought))
				printer.Fprintln()
				printer.Fprintln()
			}

			return nil
		}))

	go func() {
		defer canvas.Clear()

		var latestAction string
		var latestActionInput string
		var spinnerText string

		for {

			select {
			case thought := <-a.config.thoughtChan:
				if thought.Action != "" {
					latestAction = thought.Action
					latestActionInput = thought.ActionInput
				}
				if thought.Thought != "" {
					latestThought = thought.Thought
				}
			case <-ctx.Done():
				return
			case <-time.After(200 * time.Millisecond):
			}

			// Update spinner text
			if latestAction == "" {
				spinnerText = "Thinking..."
			} else {
				spinnerText = fmt.Sprintf("Running %s tool", color.GreenString(latestAction))
				if latestActionInput != "" {
					spinnerText += " with " + color.GreenString(latestActionInput)
				}

				spinnerText += "..."
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

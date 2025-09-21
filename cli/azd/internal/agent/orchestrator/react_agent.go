// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	"crypto/md5"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	uxlib "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
)

//go:embed prompts/react.txt
var reactPromptTemplate string

// ReactAgent implements a unified planning and execution agent using the React pattern
type ReactAgent struct {
	// State management
	currentPlan *types.Plan

	// Configuration
	config   *AgentConfig
	toolsMap map[string]common.AnnotatedTool
}

// NewReactAgent creates a new React agent
func NewReactAgent(opts ...AgentOption) *ReactAgent {
	config := &AgentConfig{}

	for _, option := range opts {
		option(config)
	}

	// Set defaults similar to orchestrator agent
	if config.conversation == nil {
		config.conversation = memory.NewConversationBuffer()
	}

	if config.maxIterations == 0 {
		config.maxIterations = 100
	}

	// Build tools map for quick lookup
	toolsMap := make(map[string]common.AnnotatedTool)
	for _, tool := range config.tools {
		toolsMap[tool.Name()] = tool
	}

	return &ReactAgent{
		config:      config,
		currentPlan: config.plan,
		toolsMap:    toolsMap,
	}
}

// SendMessage is the main entry point for the React agent
func (a *ReactAgent) SendMessage(ctx context.Context, args ...string) (string, error) {
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

	userMessage := strings.Join(args, "\n")

	// Add user message to conversation history
	if err := a.config.conversation.ChatHistory.AddUserMessage(ctx, userMessage); err != nil {
		return "", fmt.Errorf("failed to add user message to conversation: %w", err)
	}

	if a.currentPlan == nil {
		a.currentPlan = &types.Plan{
			Status:    types.PlanPending,
			Tasks:     []*types.Task{},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}

	// Iterate until plan is complete or we have a message for the user
	for i := 0; i < a.config.maxIterations; i++ {
		// Auto-advance to next pending task if no task is currently in progress
		a.autoStartNextPendingTask()

		// Generate response using React pattern
		result, err := a.generateReactResponse(ctx)
		if err != nil {
			feedback := fmt.Sprintf(
				"JSON parsing failed: %s.\nRetry with correct JSON format specified in the prompt.",
				err.Error(),
			)
			a.config.conversation.ChatHistory.AddAIMessage(ctx, feedback)
		}

		a.config.callbacksHandler.HandleChainEnd(ctx, map[string]any{
			"observation": result.Observation,
			"reasoning":   result.Reasoning,
		})

		// Apply plan updates
		if err := a.applyPlanUpdates(ctx, result.PlanUpdates); err != nil {
			return "", fmt.Errorf("failed to apply plan updates: %w", err)
		}

		// Auto-advance to next pending task if no task is currently in progress
		// This can happen when the LLM creates its initial tasks and starts actions in the same turn
		a.autoStartNextPendingTask()

		// Execute actions
		if err := a.executeActions(ctx, result.Actions); err != nil {
			return "", fmt.Errorf("failed to execute actions: %w", err)
		}

		if a.currentPlan.IsComplete() && result.Message != "" {
			// Summarize the completed plan before clearing it
			if err := a.summarizeCompletedPlan(ctx, a.currentPlan); err != nil {
				return "", fmt.Errorf("failed to summarize completed plan: %w", err)
			}
			a.currentPlan = nil
		}

		// If there's a message for the user, return it (this pauses the flow)
		if result.Message != "" {
			a.config.callbacksHandler.HandleAgentFinish(ctx, schema.AgentFinish{
				Log: result.Message,
			})

			a.config.conversation.ChatHistory.AddAIMessage(ctx, result.Message)
			return result.Message, nil
		}
	}

	return "It's taking me a while, should I continue?", nil
}

// Stop terminates the agent and performs any necessary cleanup
func (a *ReactAgent) Stop() error {
	if a.config.cleanupFunc != nil {
		return a.config.cleanupFunc()
	}

	return nil
}

// generateReactResponse calls the LLM to get a React response
func (a *ReactAgent) generateReactResponse(ctx context.Context) (*types.ReactPromptResult, error) {
	// Build prompt builder options
	options := []ConversationalPromptOption{
		WithSystemPrompt(reactPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversation(a.config.conversation),
	}

	// Get tool history for current inprogress task
	if currentTask := a.currentPlan.GetCurrentTask(); currentTask != nil && len(currentTask.ToolHistory) > 0 {
		options = append(options, WithContext("Tool History", currentTask.ToolHistory))
	}

	options = append(options, WithContext("Current Plan", a.currentPlan))

	// Build the prompt using the accumulated options
	promptBuilder := NewPromptBuilder(options...)

	// Build messages
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build messages: %w", err)
	}

	// Call LLM
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		return &types.ReactPromptResult{
				Message: err.Error(),
			},
			fmt.Errorf("failed to generate LLM response: %w", err)
	}

	// Extract response text
	var responseText string
	if len(response.Choices) > 0 {
		responseText = response.Choices[0].Content
	}

	if responseText == "" {
		return nil, fmt.Errorf("empty response from LLM")
	}

	// Parse JSON response (handles markdown code blocks)
	var result types.ReactPromptResult
	if err := unmarshalJSONResponse(responseText, &result); err != nil {
		return &types.ReactPromptResult{
				Message: responseText,
			},
			fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &result, nil
}

// applyPlanUpdates applies the plan updates from the React response
func (a *ReactAgent) applyPlanUpdates(ctx context.Context, planUpdates []*types.PlanUpdates) error {
	for _, update := range planUpdates {
		if update.Goal != "" {
			a.currentPlan.Goal = update.Goal
		}

		if update.OverallStatus != "" && a.currentPlan.CanTransitionTo(update.OverallStatus) {
			a.currentPlan.Status = update.OverallStatus
		}

		a.currentPlan.UpdatedAt = time.Now()

		// Ensure we have a plan to work with
		if a.currentPlan == nil {
			a.currentPlan = &types.Plan{
				Goal:      "",
				Status:    types.PlanPending,
				Tasks:     []*types.Task{},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}

		// Add new tasks
		for _, taskUpdate := range update.AddTasks {
			newTask := &types.Task{
				ID:           generateTaskID(taskUpdate),
				Description:  taskUpdate.Description,
				Status:       types.TaskPending,
				Requirements: taskUpdate.Requirements,
				Rules:        taskUpdate.Rules,
				UpdatedAt:    time.Now(),
				CreatedAt:    time.Now(),
			}
			a.currentPlan.Tasks = append(a.currentPlan.Tasks, newTask)
		}

		// Modify existing tasks
		for _, taskUpdate := range update.ModifyTasks {
			for _, task := range a.currentPlan.Tasks {
				if task.ID == taskUpdate.ID {
					if task.IsTerminal() {
						continue
					}
					if taskUpdate.Description != "" {
						task.Description = taskUpdate.Description
					}
					if taskUpdate.Status != "" {
						// Special handling for InProgress status - ensure only one task can be in progress
						if taskUpdate.Status == types.TaskInProgress {
							// Check if there's already a task in progress
							currentInProgress := a.currentPlan.GetCurrentTask()
							if currentInProgress != nil && currentInProgress.ID != task.ID {
								// Silently ignore - only one task can be in progress at a time
								continue
							}
						}

						// Validate status transition before applying
						if task.CanTransitionTo(taskUpdate.Status) {
							task.Status = taskUpdate.Status
						}
						// Silently ignore invalid status transitions
					}
					if len(taskUpdate.Evidence) > 0 {
						task.Evidence = appendDistinct(task.Evidence, taskUpdate.Evidence)
					}
					if len(taskUpdate.Requirements) > 0 {
						task.Requirements = appendDistinct(task.Requirements, taskUpdate.Requirements)
					}
					if len(taskUpdate.Rules) > 0 {
						task.Rules = appendDistinct(task.Rules, taskUpdate.Rules)
					}
					break
				}
			}
		}

		// Complete tasks (set status to complete)
		for _, taskUpdate := range update.CompleteTasks {
			for _, task := range a.currentPlan.Tasks {
				if task.ID == taskUpdate.ID {
					if task.Status != types.TaskInProgress {
						continue
					}

					if len(taskUpdate.Evidence) > 0 {
						task.Evidence = appendDistinct(task.Evidence, taskUpdate.Evidence)
					}

					// Summarize tool history before marking complete
					if len(task.ToolHistory) > 0 {
						if err := a.summarizeCompletedTask(ctx, task); err != nil {
							return fmt.Errorf("failed to summarize completed task %s: %w", task.ID, err)
						}
					}

					task.Status = types.TaskComplete
					break
				}
			}
		}

		a.currentPlan.UpdatedAt = time.Now()
	}

	return nil
}

// executeActions executes the actions from the React response
func (a *ReactAgent) executeActions(ctx context.Context, actions []*types.Action) error {
	// Find the current inprogress task to store tool history
	currentTask := a.currentPlan.GetCurrentTask()

	for _, action := range actions {
		// If the input is not already a string, then attempt to marshall it using JSON
		inputValue, ok := action.Input.(string)
		if !ok {
			jsonBytes, err := json.Marshal(action.Input)
			if err != nil {
				return fmt.Errorf("failed to marshal tool input: %w", err)
			}

			inputValue = string(jsonBytes)
		}

		a.config.callbacksHandler.HandleAgentAction(ctx, schema.AgentAction{
			Tool:      action.Tool,
			ToolID:    action.Tool,
			ToolInput: inputValue,
		})

		// Find the tool
		tool, exists := a.toolsMap[action.Tool]
		if !exists {
			return fmt.Errorf("tool not found: %s", action.Tool)
		}

		// Execute the tool
		a.config.callbacksHandler.HandleToolStart(ctx, inputValue)

		toolOutput, err := tool.Call(ctx, inputValue)

		// Create tool execution record
		toolExecution := &types.ToolExecution{
			Tool:      action.Tool,
			Input:     inputValue,
			Output:    toolOutput,
			Timestamp: time.Now(),
			Success:   err == nil,
		}

		if err != nil {
			toolExecution.Error = err.Error()
			a.config.callbacksHandler.HandleToolError(ctx, err)

			// Still store failed tool calls for context
			if currentTask != nil {
				currentTask.ToolHistory = append(currentTask.ToolHistory, toolExecution)
			}

			return fmt.Errorf("tool execution failed for %s: %w", action.Tool, err)
		}

		a.config.callbacksHandler.HandleToolEnd(ctx, toolOutput)

		// Store tool execution in current task instead of conversation history
		if currentTask != nil {
			currentTask.ToolHistory = append(currentTask.ToolHistory, toolExecution)
		}
	}

	return nil
}

func (a *ReactAgent) renderThoughts(ctx context.Context) (func(), error) {
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

// appendDistinct appends new items to existing slice and removes duplicates
func appendDistinct(existing []string, newItems []string) []string {
	// Create a map to track existing items
	seen := make(map[string]bool)
	result := make([]string, 0, len(existing)+len(newItems))

	// Add existing items
	for _, item := range existing {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	// Add new items if not already present
	for _, item := range newItems {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// summarizeCompletedTask creates a summary of the task's tool history and adds it to conversation
func (a *ReactAgent) summarizeCompletedTask(ctx context.Context, task *types.Task) error {
	// Use prompt builder with task context for summarization
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt("Provide a concise summary of key insights, discoveries, and outcomes from this completed task."),
		WithContext("Completed Task", task),
		WithContext("Tool History", task.ToolHistory),
	)

	// Build messages
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to build summarization messages: %w", err)
	}

	// Make LLM call for summarization
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to generate task summary: %w", err)
	}

	// Extract summary text
	var summary string
	if len(response.Choices) > 0 {
		summary = response.Choices[0].Content
	}

	if summary == "" {
		return fmt.Errorf("empty task summary returned from LLM")
	}

	// Add summary to conversation history
	taskSummaryMessage := fmt.Sprintf("Task '%s' completed. Summary: %s", task.Description, summary)
	if err := a.config.conversation.ChatHistory.AddAIMessage(ctx, taskSummaryMessage); err != nil {
		return fmt.Errorf("failed to add task summary to conversation: %w", err)
	}

	return nil
}

// summarizeCompletedPlan creates a summary of the entire plan and adds it to conversation history
func (a *ReactAgent) summarizeCompletedPlan(ctx context.Context, plan *types.Plan) error {
	// Use prompt builder with plan context for summarization
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt("Provide a comprehensive summary of this completed plan, highlighting what was accomplished, key deliverables created, and overall success."),
		WithContext("Completed Plan", plan),
	)

	// Build messages
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return fmt.Errorf("failed to build summarization messages: %w", err)
	}

	// Make LLM call for summarization
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		return fmt.Errorf("failed to generate plan summary: %w", err)
	}

	// Extract summary text
	var summary string
	if len(response.Choices) > 0 {
		summary = response.Choices[0].Content
	}

	if summary == "" {
		return fmt.Errorf("empty plan summary returned from LLM")
	}

	// Add summary to conversation history
	planSummaryMessage := fmt.Sprintf("Plan '%s' completed successfully. \nSummary: %s\n", plan.Goal, summary)
	if err := a.config.conversation.ChatHistory.AddAIMessage(ctx, planSummaryMessage); err != nil {
		return fmt.Errorf("failed to add plan summary to conversation: %w", err)
	}

	return nil
}

// generateTaskID creates a unique task identifier based on the description hash
func generateTaskID(task *types.TaskUpdate) string {
	hasher := md5.New()
	hasher.Write([]byte(task.Description))
	hash := hex.EncodeToString(hasher.Sum(nil))[:8]
	return fmt.Sprintf("task_%s", hash)
}

// autoStartNextPendingTask automatically sets the first pending task to inprogress
// if there is no current task in progress
func (a *ReactAgent) autoStartNextPendingTask() {
	if a.currentPlan == nil {
		return
	}

	// Check if there's already a task in progress
	if a.currentPlan.GetCurrentTask() != nil {
		return
	}

	// Find the first pending task and set it to inprogress
	for _, task := range a.currentPlan.Tasks {
		if task.Status == types.TaskPending {
			task.Status = types.TaskInProgress
			task.UpdatedAt = time.Now()
			return
		}
	}
}

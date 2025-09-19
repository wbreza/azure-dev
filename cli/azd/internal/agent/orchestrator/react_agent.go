// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
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

	// Iterate until plan is complete or we have a message for the user
	for i := 0; i < a.config.maxIterations; i++ {
		// Generate response using React pattern
		result, err := a.generateReactResponse(ctx)
		if err != nil {
			feedback := fmt.Sprintf(
				"JSON parsing failed: %s.\nRetry with correct JSON format specified in the prompt.",
				err.Error(),
			)
			a.config.conversation.ChatHistory.AddAIMessage(ctx, feedback)
			continue
		}

		a.config.callbacksHandler.HandleChainEnd(ctx, map[string]any{
			"observation": result.Observation,
			"reasoning":   result.Reasoning,
		})

		// Apply plan updates
		if err := a.applyPlanUpdates(result.PlanUpdates); err != nil {
			return "", fmt.Errorf("failed to apply plan updates: %w", err)
		}

		// Execute actions
		if err := a.executeActions(ctx, result.Actions); err != nil {
			return "", fmt.Errorf("failed to execute actions: %w", err)
		}

		if a.currentPlan != nil && a.isPlanComplete() {
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
	// Build the prompt using the prompt builder
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(reactPromptTemplate),
		WithPromptTools(a.config.tools),
		WithContext("Current Plan", a.currentPlan),
		WithConversation(a.config.conversation),
	)

	// Build messages
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build messages: %w", err)
	}

	// Call LLM
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate LLM response: %w", err)
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
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w", err)
	}

	return &result, nil
}

// applyPlanUpdates applies the plan updates from the React response
func (a *ReactAgent) applyPlanUpdates(planUpdates []*types.PlanUpdates) error {
	for _, update := range planUpdates {
		// Update goal if provided
		if update.Goal != "" {
			if a.currentPlan == nil {
				a.currentPlan = &types.Plan{
					Goal:      update.Goal,
					Tasks:     []*types.Task{},
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
			} else {
				a.currentPlan.Goal = update.Goal
				a.currentPlan.UpdatedAt = time.Now()
			}
		}

		// Ensure we have a plan to work with
		if a.currentPlan == nil {
			a.currentPlan = &types.Plan{
				Goal:      "",
				Tasks:     []*types.Task{},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
		}

		// Add new tasks
		for _, taskUpdate := range update.AddTasks {
			newTask := &types.Task{
				ID:           taskUpdate.ID,
				Description:  taskUpdate.Description,
				Status:       types.TaskPending,
				Requirements: taskUpdate.Requirements,
				UpdatedAt:    time.Now(),
				CreatedAt:    time.Now(),
			}
			a.currentPlan.Tasks = append(a.currentPlan.Tasks, newTask)
		}

		// Modify existing tasks
		for _, taskUpdate := range update.ModifyTasks {
			for _, task := range a.currentPlan.Tasks {
				if task.ID == taskUpdate.ID {
					if taskUpdate.Description != "" {
						task.Description = taskUpdate.Description
					}
					if taskUpdate.Status != "" {
						task.Status = taskUpdate.Status
					}
					if len(taskUpdate.Requirements) > 0 {
						task.Requirements = taskUpdate.Requirements
					}
					break
				}
			}
		}

		// Complete tasks (set status to complete)
		for _, taskUpdate := range update.CompleteTasks {
			for _, task := range a.currentPlan.Tasks {
				if task.ID == taskUpdate.ID {
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
		if err != nil {
			a.config.callbacksHandler.HandleToolError(ctx, err)
			return fmt.Errorf("tool execution failed for %s: %w", action.Tool, err)
		}

		a.config.callbacksHandler.HandleToolEnd(ctx, toolOutput)

		// Add tool result to conversation history
		toolMessage := fmt.Sprintf("Tool: %s\nInput: %v\nResult: %s", action.Tool, action.Input, toolOutput)
		if err := a.config.conversation.ChatHistory.AddAIMessage(ctx, toolMessage); err != nil {
			return fmt.Errorf("failed to add tool result to conversation: %w", err)
		}
	}

	return nil
}

// isPlanComplete checks if all tasks in the plan are complete or obsolete
func (a *ReactAgent) isPlanComplete() bool {
	if a.currentPlan == nil || len(a.currentPlan.Tasks) == 0 {
		return false
	}

	for _, task := range a.currentPlan.Tasks {
		if task.Status != types.TaskComplete && task.Status != types.TaskObsolete {
			return false
		}
	}

	return true
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

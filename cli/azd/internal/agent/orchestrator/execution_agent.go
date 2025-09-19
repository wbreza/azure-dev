// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
)

//go:embed prompts/execution.txt
var executionPromptTemplate string

// ExecutionAgent executes tasks using available tools in an enhanced ReAct loop
type ExecutionAgent struct {
	config   *AgentConfig
	toolsMap map[string]common.AnnotatedTool
}

// NewExecutionAgent creates a new execution agent with its own conversation buffer and validation agent
func NewExecutionAgent(opts ...AgentOption) *ExecutionAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	toolsMap := make(map[string]common.AnnotatedTool)
	for _, tool := range config.tools {
		toolsMap[tool.Name()] = tool
	}

	return &ExecutionAgent{
		toolsMap: toolsMap,
		config:   config,
	}
}

// ExecutePlan executes an entire execution plan
func (a *ExecutionAgent) ExecutePlan(ctx context.Context, plan *types.Plan) (*types.ExecutePlanResult, error) {
	// Iterate through each task in the plan and execute it.
	// Create a consolidated plan result with task results.

	for _, task := range plan.Tasks {
		// Execute each task in sequence
		taskResult, err := a.executeTask(ctx, task)
		if err != nil {
			// If an actual error is returned, stop and propagate it up
			return nil, fmt.Errorf("failed to execute task %s: %w", task.ID, err)
		}

		// Check if task execution paused for user message
		if taskResult.Message != "" {
			// Return plan result with message for user
			return &types.ExecutePlanResult{
				Plan:    plan,
				Message: taskResult.Message,
			}, nil
		}
	}

	planResult := &types.ExecutePlanResult{
		Plan: plan,
	}

	if plan.IsComplete() {
		summaryAgent := NewSummaryAgent(WithConfig(a.config))
		executionDescription := "Summarize the executed plan results. Include task level results as bullet points."
		summaryResult, err := summaryAgent.Summarize(ctx, executionDescription, planResult)
		if err != nil {
			return nil, err
		}

		planResult.Summary = summaryResult.Summary
	}

	agentLog := planResult.Summary
	if planResult.Message != "" {
		agentLog = planResult.Message
	}

	a.config.callbacksHandler.HandleAgentFinish(ctx, schema.AgentFinish{
		Log: agentLog,
	})

	return planResult, nil
}

// executeTask executes a complete task with its own conversation context using ReAct loop
func (a *ExecutionAgent) executeTask(ctx context.Context, task *types.Task) (*types.TaskExecutionResult, error) {
	// Create a new conversation buffer for this task
	conversationBuffer := langchainmemory.NewConversationBuffer(
		langchainmemory.WithChatHistory(a.config.conversation.ChatHistory),
	)

	a.config.callbacksHandler.HandleChainStart(ctx, map[string]any{
		"description": task.Description,
	})

	// Update task status to in progress at the beginning
	task.Status = types.TaskInProgress
	task.UpdatedAt = time.Now()

	// ReAct loop: Execute tools -> Evaluate -> Repeat until complete or max iterations
	iteration := 0

	var lastEvaluation *types.TaskExecutionEvalResult

	for iteration < a.config.maxIterations {
		iteration++

		// Execute tool calls for the task
		if err := a.invokeTools(ctx, task); err != nil {
			// Update task metadata before returning error
			a.updateTaskMetadata(task, nil)
			task.Status = types.TaskFailed
			task.UpdatedAt = time.Now()

			return nil, fmt.Errorf("failed to execute tools for task %s (iteration %d): %w", task.ID, iteration, err)
		}

		taskJsonBytes, err := json.MarshalIndent(task, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal task %s to JSON: %w", task.ID, err)
		}

		// Add the Tasks to evaluate against to the conversation buffer
		conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: fmt.Sprintf("Task to evaluate against: \n```json\n%s```\n", string(taskJsonBytes)),
		})

		// Add tool call interactions to the conversation buffer
		for _, toolCall := range task.ToolCalls {
			// Only add tool calls that have progress (were executed this iteration)
			if toolCall.Progress != nil {
				err := conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
					Content: fmt.Sprintf(
						"Called tool '%s' with input: %s",
						toolCall.Tool,
						toolCall.Input, // TODO: Truncate if too long
					),
				})
				if err != nil {
					return nil, fmt.Errorf("failed to add tool call message to conversation: %w", err)
				}

				err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
					Content: toolCall.Progress.Output,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to add tool result message to conversation: %w", err)
				}
			}
		}

		// Create a new prompt builder with the task-specific conversation buffer
		taskPromptBuilder := NewPromptBuilder(
			WithSystemPrompt(executionPromptTemplate),
			WithPromptTools(a.config.tools),
			WithConversation(conversationBuffer),
		)

		// Evaluate the task execution to get evidence and observations
		evalResult, err := a.evaluateTaskExecution(ctx, taskPromptBuilder)
		if err != nil {
			// Update task metadata before returning error
			a.updateTaskMetadata(task, nil)
			task.Status = types.TaskFailed
			task.UpdatedAt = time.Now()
			return nil, fmt.Errorf("failed to evaluate task execution for task %s (iteration %d): %w", task.ID, iteration, err)
		}

		lastEvaluation = evalResult

		// Update task metadata with new discoveries
		a.updateTaskMetadata(task, evalResult)

		a.config.callbacksHandler.HandleChainEnd(ctx, map[string]any{
			"summary":      task.Progress.Summary,
			"observations": task.Progress.Observations,
			"evidence":     task.Progress.Evidence,
		})

		// Check if we need to pause for user message
		if evalResult.Message != "" {
			task.Status = types.TaskInProgress // Keep as in progress since we're pausing
			task.UpdatedAt = time.Now()
			return &types.TaskExecutionResult{
				Task:    task,
				Message: evalResult.Message,
			}, nil
		}

		// Check if we have more actions to execute
		if len(evalResult.Actions) == 0 {
			// No more actions, task execution is complete
			break
		}
	}

	// Check if we hit max iterations with pending actions
	if iteration >= a.config.maxIterations {
		if lastEvaluation != nil && len(lastEvaluation.Actions) > 0 {
			task.Status = types.TaskInProgress
			task.UpdatedAt = time.Now()

			return &types.TaskExecutionResult{
				Task:    task,
				Message: fmt.Sprintf("Maximum iterations (%d) reached but more actions are pending. Do you want to continue?", a.config.maxIterations),
			}, nil
		}
	}

	// Move to validation phase only if no message was returned
	task.Status = types.TaskAwaitingValidation
	task.UpdatedAt = time.Now()

	// Send to validation agent for validation
	validationAgent := NewValidationAgent(WithConfig(a.config))
	validationResult, err := validationAgent.ValidateTask(ctx, task)
	if err != nil {
		task.Status = types.TaskValidationFailed
		task.UpdatedAt = time.Now()
		return nil, fmt.Errorf("failed to validate task %s: %w", task.ID, err)
	}

	// Update task status based on validation result
	if validationResult.Evaluation != nil {
		task.Status = validationResult.Evaluation.Status
		task.UpdatedAt = time.Now()
	}

	return &types.TaskExecutionResult{
		Task:    task,
		Message: "", // No message needed, task completed normally
	}, nil
}

// updateTaskMetadata updates task with new rules, validation criteria, and actions
func (a *ExecutionAgent) updateTaskMetadata(task *types.Task, evalResult *types.TaskExecutionEvalResult) {
	if evalResult == nil {
		return
	}

	// New tool calls required for task
	task.ToolCalls = append(task.ToolCalls, evalResult.Actions...)

	// Append new rules (avoid duplicates)
	for _, newRule := range evalResult.Rules {
		if newRule != "" && !slices.Contains(task.Rules, newRule) {
			task.Rules = append(task.Rules, newRule)
		}
	}

	// Append new validation criteria (avoid duplicates)
	for _, newCriteria := range evalResult.ValidationCriteria {
		if newCriteria != "" && !slices.Contains(task.ValidationCriteria, newCriteria) {
			task.ValidationCriteria = append(task.ValidationCriteria, newCriteria)
		}
	}

	if task.Progress == nil {
		task.Progress = &types.TaskProgress{}
	}

	// Update observations from last evaluation result
	task.Progress.Summary = evalResult.Summary
	task.Progress.Evidence = append(task.Progress.Evidence, evalResult.Evidence...)
	task.Progress.Observations = append(task.Progress.Observations, evalResult.Observations...)
}

func (a *ExecutionAgent) invokeTools(ctx context.Context, task *types.Task) error {
	// Iterate through each tool call
	// return a slice of tool call results

	for _, toolCall := range task.ToolCalls {
		// Skip tools that have already been executed (have Progress)
		if toolCall.Progress != nil {
			continue
		}

		a.config.callbacksHandler.HandleAgentAction(ctx, schema.AgentAction{
			Tool:      toolCall.Tool,
			ToolID:    toolCall.Tool,
			ToolInput: toolCall.Input,
			Log:       toolCall.Reasoning,
		})

		// Find the tool in our tools map
		tool, exists := a.toolsMap[toolCall.Tool]
		if !exists {
			return fmt.Errorf("tool '%s' not found in available tools", toolCall.Tool)
		}

		startTime := time.Now()

		// Marshal the input to JSON string for the tool
		inputValue := ""
		if toolCall.Input != "" {
			// Parse the input as JSON to validate it, then re-marshal
			var parsedInput interface{}
			if err := json.Unmarshal([]byte(toolCall.Input), &parsedInput); err != nil {
				// Not JSON use raw string
				inputValue = toolCall.Input
			} else {
				jsonBytes, err := json.Marshal(parsedInput)
				if err != nil {
					return fmt.Errorf("failed to marshal input for tool '%s': %w", toolCall.Tool, err)
				}
				inputValue = string(jsonBytes)
			}
		}

		// Execute the tool
		a.config.callbacksHandler.HandleToolStart(ctx, inputValue)
		toolOutput, err := tool.Call(ctx, inputValue)
		if err != nil {
			a.config.callbacksHandler.HandleToolError(ctx, err)
		}
		endTime := time.Now()

		// Create the tool call result
		toolCall.Progress = &types.ToolCallProgress{
			Output:    toolOutput,
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  endTime.Sub(startTime),
		}

		if err != nil {
			toolCall.Progress.Error = err.Error()
			return fmt.Errorf("tool '%s' execution failed: %w", toolCall.Tool, err)
		}

		a.config.callbacksHandler.HandleToolEnd(ctx, toolOutput)
	}

	return nil
}

func (a *ExecutionAgent) evaluateTaskExecution(ctx context.Context, prompt *PromptBuilder) (*types.TaskExecutionEvalResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := prompt.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build evaluation messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate evaluation response: %w", err)
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

	// Parse JSON response using the execution prompt format
	var evalResponse *types.TaskExecutionEvalResult
	if err := unmarshalJSONResponse(responseText, &evalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation response as JSON: %w\nResponse: %s", err, responseText)
	}

	return evalResponse, nil
}

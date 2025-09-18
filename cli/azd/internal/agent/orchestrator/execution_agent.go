// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

//go:embed prompts/execution.txt
var executionPromptTemplate string

// ExecutionAgent executes tasks using available tools in an enhanced ReAct loop
type ExecutionAgent struct {
	config          *AgentConfig
	toolsMap        map[string]common.AnnotatedTool
	promptBuilder   *PromptBuilder
	validationAgent *ValidationAgent
}

// NewExecutionAgent creates a new execution agent with its own conversation buffer and validation agent
func NewExecutionAgent(opts ...AgentOption) *ExecutionAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Use the embedded prompt template as the system prompt (contains JSON schema and core instructions)
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(executionPromptTemplate),
		WithPromptTools(config.tools),
		WithConversationBuffer(conversationBuffer),
	)

	toolsMap := make(map[string]common.AnnotatedTool)
	for _, tool := range config.tools {
		toolsMap[tool.Name()] = tool
	}

	// Create validation agent for task validation
	validationAgent := NewValidationAgent(WithConfig(config))

	return &ExecutionAgent{
		config:          config,
		toolsMap:        toolsMap,
		promptBuilder:   promptBuilder,
		validationAgent: validationAgent,
	}
}

// ExecutePlan executes an entire execution plan
func (a *ExecutionAgent) ExecutePlan(ctx context.Context, plan *types.Plan) (*types.ExecutePlanResult, error) {
	// Iterate through each task in the plan and execute it.
	// Create a consolidated plan result with task results.

	for _, task := range plan.Tasks {
		// Execute each task in sequence
		if err := a.ExecuteTask(ctx, task); err != nil {
			// If an actual error is returned, stop and propagate it up
			return nil, fmt.Errorf("failed to execute task %s: %w", task.ID, err)
		}
	}

	planResult := &types.ExecutePlanResult{
		Plan: plan,
	}

	if plan.IsComplete() {
		summaryAgent := NewSummaryAgent(WithConfig(a.config))
		summaryResult, err := summaryAgent.Summarize(ctx, planResult)
		if err != nil {
			return nil, err
		}

		planResult.Summary = summaryResult.Summary
	}

	return planResult, nil
}

// ExecuteTask executes a complete task with its own conversation context
func (a *ExecutionAgent) ExecuteTask(ctx context.Context, task *types.Task) error {
	// Update task status to in progress at the beginning
	task.Status = types.TaskInProgress
	task.UpdatedAt = time.Now()

	// Create a new conversation buffer for this task
	conversationBuffer := langchainmemory.NewConversationBuffer()

	// Execute tool calls for the task
	toolCallResults, err := a.invokeTools(ctx, task)
	if err != nil {
		// Actual error occurred during tool execution
		task.Status = types.TaskFailed
		task.UpdatedAt = time.Now()
		return fmt.Errorf("failed to execute tools for task %s: %w", task.ID, err)
	}

	taskJsonBytes, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal task %s to JSON: %w", task.ID, err)
	}

	// Add the Tasks to evaluate against to the conversation buffer
	conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
		Content: fmt.Sprintf("Task to evaluate against: \n```json\n%s```\n", string(taskJsonBytes)),
	})

	// Add tool call interactions to the conversation buffer
	for _, toolResult := range toolCallResults {
		err := conversationBuffer.ChatHistory.AddMessage(ctx, llms.AIChatMessage{
			Content: fmt.Sprintf(
				"Called tool '%s' with input: %s",
				toolResult.ToolCall.Tool,
				toolResult.ToolCall.Input, // TODO: Truncate if too long
			),
		})
		if err != nil {
			return fmt.Errorf("failed to add tool call message to conversation: %w", err)
		}

		err = conversationBuffer.ChatHistory.AddMessage(ctx, llms.ToolChatMessage{
			Content: toolResult.Output,
		})
		if err != nil {
			return fmt.Errorf("failed to add tool result message to conversation: %w", err)
		}
	}

	// Create a new prompt builder with the task-specific conversation buffer
	taskPromptBuilder := NewPromptBuilder(
		WithSystemPrompt(executionPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversationBuffer(conversationBuffer),
	)

	// Evaluate the task execution to get evidence and insights
	evalResult, err := a.evaluateTaskExecution(ctx, taskPromptBuilder)
	if err != nil {
		task.Status = types.TaskFailed
		task.UpdatedAt = time.Now()
		return fmt.Errorf("failed to evaluate task execution for task %s: %w", task.ID, err)
	}

	task.Status = types.TaskAwaitingValidation
	task.UpdatedAt = time.Now()
	task.Progress = &types.TaskProgress{
		ToolCalls:  toolCallResults,
		Evaluation: evalResult,
	}

	// Send to validation agent for validation
	validationResult, err := a.validationAgent.ValidateTask(ctx, task)
	if err != nil {
		task.Status = types.TaskValidationFailed
		task.UpdatedAt = time.Now()
		return fmt.Errorf("failed to validate task %s: %w", task.ID, err)
	}

	// Update task status based on validation result
	if validationResult.Evaluation != nil {
		task.Status = validationResult.Evaluation.Status
		task.UpdatedAt = time.Now()
	}

	return nil
}

func (a *ExecutionAgent) invokeTools(ctx context.Context, task *types.Task) ([]*types.ToolCallResult, error) {
	// Iterate through each tool call
	// return a slice of tool call results

	results := make([]*types.ToolCallResult, 0, len(task.ToolCalls))

	for _, toolCall := range task.ToolCalls {
		// Find the tool in our tools map
		tool, exists := a.toolsMap[toolCall.Tool]
		if !exists {
			return nil, fmt.Errorf("tool '%s' not found in available tools", toolCall.Tool)
		}

		startTime := time.Now()

		// Marshal the input to JSON string for the tool
		var inputJSON string
		if toolCall.Input != "" {
			// Parse the input as JSON to validate it, then re-marshal
			var parsedInput interface{}
			if err := json.Unmarshal([]byte(toolCall.Input), &parsedInput); err != nil {
				return nil, fmt.Errorf("invalid JSON input for tool '%s': %w", toolCall.Tool, err)
			}

			inputBytes, err := json.Marshal(parsedInput)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal input for tool '%s': %w", toolCall.Tool, err)
			}
			inputJSON = string(inputBytes)
		} else {
			inputJSON = "{}" // Default empty JSON object
		}

		// Execute the tool
		output, err := tool.Call(ctx, inputJSON)
		endTime := time.Now()

		// Create the tool call result
		result := &types.ToolCallResult{
			ToolCall:  toolCall,
			Output:    output,
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  endTime.Sub(startTime),
		}

		if err != nil {
			result.Error = err.Error()
		}

		results = append(results, result)

		// If error occurred and we want to fail immediately, return the error
		if err != nil {
			return nil, fmt.Errorf("tool '%s' execution failed: %w", toolCall.Tool, err)
		}
	}

	return results, nil
}

func (a *ExecutionAgent) evaluateTaskExecution(ctx context.Context, prompt *PromptBuilder) (*types.TaskExecutionEvalResult, error) {
	// Construct execution prompt with builder
	// Include messages of each tool call and its results
	// Generate evidence and insights for the task
	// Invoke prompt message to LLM for evaluation

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
	var agentEvalResponse *types.TaskExecutionEvalResult
	if err := unmarshalJSONResponse(responseText, &agentEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse evaluation response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.TaskExecutionEvalResult{
		Summary:  agentEvalResponse.Summary,
		Insights: agentEvalResponse.Insights,
		Evidence: agentEvalResponse.Evidence,
	}, nil
}

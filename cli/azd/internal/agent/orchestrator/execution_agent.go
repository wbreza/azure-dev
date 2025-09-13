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
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/tools"
)

//go:embed prompts/execution.txt
var executionPromptTemplate string

// ExecutionAgent executes tasks using available tools in an enhanced ReAct loop
type ExecutionAgent struct {
	config        *AgentConfig
	toolsMap      map[string]common.AnnotatedTool
	promptBuilder *ConversationalPromptBuilder
}

// NewExecutionAgent creates a new execution agent with shared conversation buffer and working memory
func NewExecutionAgent(opts ...AgentOption) *ExecutionAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	// Use the embedded prompt template as the system prompt (contains JSON schema and core instructions)
	promptBuilder := NewConversationalPromptBuilder(
		WithSystemPrompt(executionPromptTemplate),
		WithPromptTools(config.tools),
		WithWorkingMemory(StandardWorkingMemoryFormatter),
		WithConversationBuffer(config.conversationBuffer),
	)

	toolsMap := make(map[string]common.AnnotatedTool)
	for _, tool := range config.tools {
		toolsMap[tool.Name()] = tool
	}

	return &ExecutionAgent{
		config:        config,
		toolsMap:      toolsMap,
		promptBuilder: promptBuilder,
	}
}

// ExecuteStep performs one step of the enhanced ReAct loop
func (a *ExecutionAgent) ExecuteStep(ctx context.Context) (*types.AgentResponse, []types.ActionResult, error) {
	// Get agent response
	response, err := a.getAgentResponse(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get agent response: %w", err)
	}

	// Execute actions
	actionResults, err := a.executeActions(ctx, response.Actions)
	if err != nil {
		return response, nil, fmt.Errorf("failed to execute actions: %w", err)
	}

	// Process completed tasks and update working memory
	for _, completedTask := range response.CompleteTasks {
		err := a.config.workingMemory.UpdateTaskStatus(completedTask.TaskID, "completed", completedTask.Evidence)
		if err != nil {
			// Log warning but don't fail the entire step
			fmt.Printf("Warning: failed to update task status for %s: %v\n", completedTask.TaskID, err)
		}
	}

	return response, actionResults, nil
}

// ExecuteSingleAction executes a single action and returns the result
func (a *ExecutionAgent) ExecuteSingleAction(ctx context.Context, action types.ActionRequest) (types.ActionResult, error) {
	return a.executeAction(ctx, action)
}

// ExecuteTask executes a complete task with its own conversation context
func (a *ExecutionAgent) ExecuteTask(ctx context.Context, task *types.Task) (*types.TaskExecutionResult, error) {
	// Create task-specific conversation buffer
	taskConversation := langchainmemory.NewConversationBuffer()

	// Add task context to conversation
	taskContext := fmt.Sprintf("Executing task: %s\nDescription: %s\nValidation Criteria: %s",
		task.ID, task.Description, task.ValidationCriteria)
	if err := taskConversation.ChatHistory.AddUserMessage(ctx, taskContext); err != nil {
		return nil, fmt.Errorf("failed to add task context to conversation: %w", err)
	}

	// Create task-specific execution agent with its own conversation
	taskAgent := NewExecutionAgent(
		WithConfig(&AgentConfig{
			model:              a.config.model,
			tools:              a.config.tools,
			conversationBuffer: taskConversation,
			workingMemory:      a.config.workingMemory,
			callbacksHandler:   a.config.callbacksHandler,
			thoughtChan:        a.config.thoughtChan,
		}),
	)

	var allToolCalls []types.ActionResult
	var evidence []string
	maxExecutionIterations := 10

	// Execute planned tool calls first
	for _, plannedCall := range task.ToolCalls {
		// Add tool call intent to conversation
		toolCallMsg := fmt.Sprintf("Executing planned tool: %s\nReasoning: %s\nInput: %v",
			plannedCall.ToolName, plannedCall.Reasoning, plannedCall.Input)
		if err := taskConversation.ChatHistory.AddUserMessage(ctx, toolCallMsg); err != nil {
			return nil, fmt.Errorf("failed to add tool call message to conversation: %w", err)
		}

		// Execute the tool call
		actionRequest := types.ActionRequest{
			Tool:      plannedCall.ToolName,
			Input:     plannedCall.Input,
			Reasoning: plannedCall.Reasoning,
		}

		result, err := taskAgent.executeAction(ctx, actionRequest)
		if err != nil {
			// Add error result to conversation and continue
			errorMsg := fmt.Sprintf("Tool %s failed: %s", plannedCall.ToolName, err.Error())
			if err := taskConversation.ChatHistory.AddAIMessage(ctx, errorMsg); err != nil {
				return nil, fmt.Errorf("failed to add tool error to conversation: %w", err)
			}
		} else {
			// Add successful tool result to conversation
			resultMsg := fmt.Sprintf("Tool %s completed successfully. Output: %s", plannedCall.ToolName, result.Output)
			if err := taskConversation.ChatHistory.AddAIMessage(ctx, resultMsg); err != nil {
				return nil, fmt.Errorf("failed to add tool result to conversation: %w", err)
			}
		}
		allToolCalls = append(allToolCalls, result)
	}

	// ReAct loop for additional analysis and actions
	for iteration := 0; iteration < maxExecutionIterations; iteration++ {
		// Get agent response using task-specific conversation
		response, actionResults, err := taskAgent.ExecuteStep(ctx)
		if err != nil {
			return &types.TaskExecutionResult{
				TaskID:             task.ID,
				Status:             "failed",
				Evidence:           evidence,
				ToolCalls:          allToolCalls,
				Reasoning:          fmt.Sprintf("Execution failed after %d iterations: %v", iteration, err),
				ConversationBuffer: taskConversation,
				ReplanReason:       "",
			}, nil
		}

		// Add action results to our collection
		allToolCalls = append(allToolCalls, actionResults...)

		// Process any additional tool calls suggested by the agent
		if len(response.Actions) > 0 {
			additionalResults, err := taskAgent.executeActions(ctx, response.Actions)
			if err != nil {
				return &types.TaskExecutionResult{
					TaskID:             task.ID,
					Status:             "failed",
					Evidence:           evidence,
					ToolCalls:          allToolCalls,
					Reasoning:          fmt.Sprintf("Additional action execution failed: %v", err),
					ConversationBuffer: taskConversation,
					ReplanReason:       "",
				}, nil
			}
			allToolCalls = append(allToolCalls, additionalResults...)

			// Add tool results to conversation for next iteration
			for _, result := range additionalResults {
				var resultMsg string
				if result.Error != "" {
					resultMsg = fmt.Sprintf("Additional tool %s failed: %s", result.Tool, result.Error)
				} else {
					resultMsg = fmt.Sprintf("Additional tool %s completed successfully. Output: %s", result.Tool, result.Output)
				}
				if err := taskConversation.ChatHistory.AddAIMessage(ctx, resultMsg); err != nil {
					return nil, fmt.Errorf("failed to add additional tool result to conversation: %w", err)
				}
			}

			// Continue the loop to re-analyze with new tool results
			continue
		}

		// Check if agent marked this task as complete
		taskCompleted := false
		for _, completedTask := range response.CompleteTasks {
			if completedTask.TaskID == task.ID {
				taskCompleted = true
				evidence = append(evidence, completedTask.Evidence)
				break
			}
		}

		if taskCompleted {
			// Task is marked complete
			return &types.TaskExecutionResult{
				TaskID:             task.ID,
				Status:             "completed",
				Evidence:           evidence,
				ToolCalls:          allToolCalls,
				Reasoning:          response.Thought,
				ConversationBuffer: taskConversation,
				ReplanReason:       "",
			}, nil
		}

		// Agent doesn't think task is complete and suggests no additional actions
		// Check if we should request replanning
		if response.Thought != "" && (strings.Contains(strings.ToLower(response.Thought), "replan") ||
			strings.Contains(strings.ToLower(response.Thought), "stuck") ||
			strings.Contains(strings.ToLower(response.Thought), "unable")) {
			return &types.TaskExecutionResult{
				TaskID:             task.ID,
				Status:             "needs_replanning",
				Evidence:           evidence,
				ToolCalls:          allToolCalls,
				Reasoning:          response.Thought,
				ConversationBuffer: taskConversation,
				ReplanReason:       response.Thought,
			}, nil
		}

		// Add agent's reasoning to conversation for next iteration
		reasoningContext := fmt.Sprintf("Agent analysis (iteration %d): %s. Observation: %s",
			iteration+1, response.Thought, response.Observation)
		if err := taskConversation.ChatHistory.AddAIMessage(ctx, reasoningContext); err != nil {
			return nil, fmt.Errorf("failed to add agent reasoning to conversation: %w", err)
		}
	}

	// If we reach here, we've exceeded max iterations
	return &types.TaskExecutionResult{
		TaskID:             task.ID,
		Status:             "needs_replanning",
		Evidence:           evidence,
		ToolCalls:          allToolCalls,
		Reasoning:          fmt.Sprintf("Exceeded maximum execution iterations (%d)", maxExecutionIterations),
		ConversationBuffer: taskConversation,
		ReplanReason:       "Task execution exceeded maximum iterations without completion",
	}, nil
}

// Private methods

func (a *ExecutionAgent) getAgentResponse(ctx context.Context) (*types.AgentResponse, error) {
	// Build messages using the conversational prompt builder
	messages, err := a.promptBuilder.BuildMessages(ctx, a.config.workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution messages: %w", err)
	}

	// Add current task status as human message for context
	taskStatus := a.config.workingMemory.GetTaskStatusSummary()
	recentHistory := a.config.workingMemory.GetRecentHistory(5)

	var contextBuilder strings.Builder
	contextBuilder.WriteString("Current Execution Status:\n")
	contextBuilder.WriteString(taskStatus.ToPromptFormat())
	contextBuilder.WriteString("\n\nRecent Activity:\n")
	for _, event := range recentHistory {
		contextBuilder.WriteString(fmt.Sprintf("- %s: %s\n", event.Type, event.Details))
	}
	contextBuilder.WriteString("\nPlease analyze the current situation and determine the next action to take.")

	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(contextBuilder.String())},
	})

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate execution response: %w", err)
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
	var agentResponse types.AgentResponse
	if err := unmarshalJSONResponse(responseText, &agentResponse); err != nil {
		return nil, fmt.Errorf("failed to parse agent response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &agentResponse, nil
}

func (a *ExecutionAgent) executeActions(ctx context.Context, actions []types.ActionRequest) ([]types.ActionResult, error) {
	results := make([]types.ActionResult, 0, len(actions))

	for _, action := range actions {
		result, err := a.executeAction(ctx, action)
		if err != nil {
			// Don't fail entirely if one action fails - record the error and continue
			result = types.ActionResult{
				Tool:      action.Tool,
				Input:     action.Input,
				Error:     err.Error(),
				Timestamp: time.Now(),
				Duration:  "0s",
			}
		}
		results = append(results, result)
	}

	return results, nil
}

func (a *ExecutionAgent) executeAction(ctx context.Context, action types.ActionRequest) (types.ActionResult, error) {
	startTime := time.Now()

	// Convert input to string (tools expect string input)
	inputJSON, err := json.Marshal(action.Input)
	if err != nil {
		return types.ActionResult{}, fmt.Errorf("failed to marshal tool input: %w", err)
	}

	a.config.callbacksHandler.HandleAgentAction(ctx, schema.AgentAction{
		ToolID:    action.Tool,
		Tool:      action.Tool,
		ToolInput: string(inputJSON),
		Log:       action.Reasoning,
	})

	// Find the tool
	tool, exists := a.toolsMap[action.Tool]
	if !exists {
		return types.ActionResult{}, fmt.Errorf("tool %s not found", action.Tool)
	}

	// Execute the tool
	a.config.callbacksHandler.HandleToolStart(ctx, string(inputJSON))

	output, err := tool.Call(ctx, string(inputJSON))
	if err != nil {
		a.config.callbacksHandler.HandleToolError(ctx, err)

		return types.ActionResult{
			Tool:      action.Tool,
			Input:     action.Input,
			Error:     err.Error(),
			Timestamp: startTime,
			Duration:  time.Since(startTime).String(),
		}, err
	}

	a.config.callbacksHandler.HandleToolEnd(ctx, output)

	return types.ActionResult{
		Tool:      action.Tool,
		Input:     action.Input,
		Output:    output,
		Timestamp: startTime,
		Duration:  time.Since(startTime).String(),
	}, nil
}

// GetTools returns the tools available to this agent (for langchain compatibility)
func (a *ExecutionAgent) GetTools() []tools.Tool {
	return common.ToLangChainTools(a.config.tools)
}

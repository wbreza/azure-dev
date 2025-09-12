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

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/tools"
)

//go:embed prompts/execution.txt
var executionPromptTemplate string

// ExecutionAgent executes tasks using available tools in an enhanced ReAct loop
type ExecutionAgent struct {
	model         llms.Model
	tools         []common.AnnotatedTool
	toolsMap      map[string]common.AnnotatedTool
	promptBuilder *ConversationalPromptBuilder
}

// NewExecutionAgent creates a new execution agent with shared conversation buffer and working memory
func NewExecutionAgent(opts ...OrchestratorAgentOption) *ExecutionAgent {
	config := &OrchestratorAgentConfig{}
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
		model:         config.model,
		tools:         config.tools,
		toolsMap:      toolsMap,
		promptBuilder: promptBuilder,
	}
}

// ExecuteStep performs one step of the enhanced ReAct loop
func (e *ExecutionAgent) ExecuteStep(ctx context.Context, workingMemory *memory.WorkingMemory) (*types.AgentResponse, []types.ActionResult, error) {
	// Get agent response
	response, err := e.getAgentResponse(ctx, workingMemory)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get agent response: %w", err)
	}

	// Execute actions
	actionResults, err := e.executeActions(ctx, response.Actions)
	if err != nil {
		return response, nil, fmt.Errorf("failed to execute actions: %w", err)
	}

	return response, actionResults, nil
}

// Private methods

func (e *ExecutionAgent) getAgentResponse(ctx context.Context, workingMemory *memory.WorkingMemory) (*types.AgentResponse, error) {
	// Build messages using the conversational prompt builder
	messages, err := e.promptBuilder.BuildMessages(ctx, workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build execution messages: %w", err)
	}

	// Add current task status as human message for context
	taskStatus := workingMemory.GetTaskStatusSummary()
	recentHistory := workingMemory.GetRecentHistory(5)

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
	response, err := e.model.GenerateContent(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to generate execution response: %w", err)
	}

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

func (e *ExecutionAgent) executeActions(ctx context.Context, actions []types.ActionRequest) ([]types.ActionResult, error) {
	results := make([]types.ActionResult, 0, len(actions))

	for _, action := range actions {
		result, err := e.executeAction(ctx, action)
		if err != nil {
			// Don't fail entirely if one action fails - record the error and continue
			result = types.ActionResult{
				Tool:      action.Tool,
				Input:     fmt.Sprintf("%v", action.Input),
				Error:     err.Error(),
				Timestamp: time.Now(),
				Duration:  "0s",
			}
		}
		results = append(results, result)
	}

	return results, nil
}

func (e *ExecutionAgent) executeAction(ctx context.Context, action types.ActionRequest) (types.ActionResult, error) {
	startTime := time.Now()

	// Find the tool
	tool, exists := e.toolsMap[action.Tool]
	if !exists {
		return types.ActionResult{}, fmt.Errorf("tool %s not found", action.Tool)
	}

	// Convert input to string (tools expect string input)
	inputJSON, err := json.Marshal(action.Input)
	if err != nil {
		return types.ActionResult{}, fmt.Errorf("failed to marshal tool input: %w", err)
	}

	// Execute the tool
	output, err := tool.Call(ctx, string(inputJSON))
	if err != nil {
		return types.ActionResult{
			Tool:      action.Tool,
			Input:     string(inputJSON),
			Error:     err.Error(),
			Timestamp: startTime,
			Duration:  time.Since(startTime).String(),
		}, err
	}

	return types.ActionResult{
		Tool:      action.Tool,
		Input:     string(inputJSON),
		Output:    output,
		Timestamp: startTime,
		Duration:  time.Since(startTime).String(),
	}, nil
}

// GetTools returns the tools available to this agent (for langchain compatibility)
func (e *ExecutionAgent) GetTools() []tools.Tool {
	return common.ToLangChainTools(e.tools)
}

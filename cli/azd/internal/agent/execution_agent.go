// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package agent

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
	"github.com/tmc/langchaingo/prompts"
	"github.com/tmc/langchaingo/tools"
)

//go:embed prompts/execution.txt
var executionPromptTemplate string

// ExecutionAgent executes tasks using available tools in an enhanced ReAct loop
type ExecutionAgent struct {
	llm            llms.Model
	tools          []common.AnnotatedTool
	toolsMap       map[string]common.AnnotatedTool
	promptTemplate prompts.PromptTemplate
}

// NewExecutionAgent creates a new execution agent
func NewExecutionAgent(llm llms.Model, tools []common.AnnotatedTool) *ExecutionAgent {
	// Create tools map for quick lookup
	toolsMap := make(map[string]common.AnnotatedTool)
	for _, tool := range tools {
		toolsMap[tool.Name()] = tool
	}

	return &ExecutionAgent{
		llm:      llm,
		tools:    tools,
		toolsMap: toolsMap,
		promptTemplate: prompts.PromptTemplate{
			Template:       executionPromptTemplate,
			TemplateFormat: prompts.TemplateFormatGoTemplate,
			InputVariables: []string{"goal", "taskStatus", "toolDescriptions", "recentHistory"},
		},
	}
}

// ExecuteStep performs one step of the enhanced ReAct loop
func (e *ExecutionAgent) ExecuteStep(ctx context.Context, workingMemory *memory.WorkingMemory) (*types.AgentResponse, []types.ActionResult, error) {
	// Build context for the agent
	context := e.buildExecutionContext(workingMemory)

	// Get agent response
	response, err := e.getAgentResponse(ctx, context)
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

func (e *ExecutionAgent) buildExecutionContext(workingMemory *memory.WorkingMemory) map[string]any {
	taskStatus := workingMemory.GetTaskStatusSummary()
	recentHistory := workingMemory.GetRecentHistory(5)

	// Format recent history for prompt
	var historyBuilder strings.Builder
	for _, event := range recentHistory {
		historyBuilder.WriteString(fmt.Sprintf("- %s: %s\n", event.Type, event.Details))
	}

	return map[string]any{
		"goal":             workingMemory.GetGoal(),
		"taskStatus":       taskStatus.ToPromptFormat(),
		"toolDescriptions": toolDescriptions(e.tools),
		"recentHistory":    historyBuilder.String(),
	}
}

func (e *ExecutionAgent) getAgentResponse(ctx context.Context, context map[string]any) (*types.AgentResponse, error) {
	// Generate prompt
	prompt, err := e.promptTemplate.Format(context)
	if err != nil {
		return nil, fmt.Errorf("failed to format execution prompt: %w", err)
	}

	// Get response from LLM
	response, err := llms.GenerateFromSinglePrompt(ctx, e.llm, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate execution response: %w", err)
	}

	// Parse JSON response (handle markdown-formatted JSON)
	var agentResponse types.AgentResponse
	if err := unmarshalJSONResponse(response, &agentResponse); err != nil {
		return nil, fmt.Errorf("failed to parse agent response as JSON: %w\nResponse: %s", err, response)
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

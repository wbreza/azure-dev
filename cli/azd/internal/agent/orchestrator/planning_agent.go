// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
)

//go:embed prompts/planning.txt
var planningPromptTemplate string

// PlanningAgent creates structured execution plans using LLM reasoning
type PlanningAgent struct {
	config        *AgentConfig
	promptBuilder *ConversationalPromptBuilder
}

// NewPlanningAgent creates a new planning agent
func NewPlanningAgent(opts ...AgentOption) *PlanningAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	// Use the embedded prompt template as the system prompt (contains JSON schema and core instructions)
	promptBuilder := NewConversationalPromptBuilder(
		WithSystemPrompt(planningPromptTemplate),
		WithPromptTools(config.tools),
		WithWorkingMemory(StandardWorkingMemoryFormatter),
	)

	return &PlanningAgent{
		config:        config,
		promptBuilder: promptBuilder,
	}
}

// CreatePlan generates a new execution plan for the given goal
func (a *PlanningAgent) CreatePlan(ctx context.Context, goal string) (*types.PlanningResult, error) {
	// Build context from working memory if available
	context := ""
	if a.config.workingMemory != nil {
		taskSummary := a.config.workingMemory.GetTaskStatusSummary()
		context = taskSummary.ToPromptFormat()
	}

	return a.createPlanWithContext(ctx, goal, context)
}

// UpdatePlan modifies an existing execution plan based on working memory state
func (a *PlanningAgent) UpdatePlan(ctx context.Context) (*types.ExecutionPlan, error) {
	goal := a.config.workingMemory.GetGoal()

	// Build context for replanning
	currentPlan := a.config.workingMemory.GetExecutionPlan()

	contextBuilder := fmt.Sprintf(`
CURRENT PLAN STATUS:
Goal: %s
Total Tasks: %d

EXISTING TASKS:
`, goal, len(currentPlan.Tasks))

	for _, task := range currentPlan.Tasks {
		status := "pending"
		switch task.Status {
		case types.TaskComplete:
			status = "✓ COMPLETE"
		case types.TaskFailed:
			status = "✗ FAILED"
		case types.TaskInProgress:
			status = "→ IN PROGRESS"
		case types.TaskBlocked:
			status = "🚫 BLOCKED"
		}
		contextBuilder += fmt.Sprintf("- %s: %s (%s)\n", task.ID, task.Description, status)
	}

	recentHistory := a.config.workingMemory.GetRecentHistory(5)
	contextBuilder += "\nRECENT EXECUTION HISTORY:\n"
	for _, event := range recentHistory {
		contextBuilder += fmt.Sprintf("- %s: %s\n", event.Type, event.Details)
	}

	contextBuilder += "\nPlease update the plan as needed. Keep completed tasks unchanged unless they need to be redone."

	planningResult, err := a.createPlanWithContext(ctx, goal, contextBuilder)
	if err != nil {
		return nil, err
	}

	// For replanning, we always expect tasks (not messages)
	if planningResult.ResponseType != "tasks" || planningResult.Plan == nil {
		return nil, fmt.Errorf("expected task-based planning result for replanning, got %s", planningResult.ResponseType)
	}

	return planningResult.Plan, nil
}

// createPlanWithContext generates a new execution plan for the given goal and context
func (a *PlanningAgent) createPlanWithContext(ctx context.Context, goal string, context string) (*types.PlanningResult, error) {
	// Create working memory for this planning session
	workingMemory := memory.NewWorkingMemory()
	workingMemory.SetGoal(goal)

	// Add context as an event if provided
	if context != "" {
		workingMemory.AddEvent(memory.ExecutionEvent{
			Type:    "planning_context",
			Details: context,
		})
	}

	// Build messages using the conversational prompt builder
	messages, err := a.promptBuilder.BuildMessages(ctx, workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build planning messages: %w", err)
	}

	// Add the user goal as the human message
	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeHuman,
		Parts: []llms.ContentPart{llms.TextPart(fmt.Sprintf("User Request: %s", goal))},
	})

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return nil, fmt.Errorf("failed to generate planning response: %w", err)
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
	var planningResponse PlanningResponse
	if err := unmarshalJSONResponse(responseText, &planningResponse); err != nil {
		return nil, fmt.Errorf("failed to parse planning response as JSON: %w\nResponse: %s", err, responseText)
	}

	// Create PlanningResult based on response type
	result := &types.PlanningResult{
		ResponseType: planningResponse.ResponseType,
		Goal:         planningResponse.Goal,
	}

	// Handle different response types
	switch planningResponse.ResponseType {
	case "message":
		// Simple message response - no task planning needed
		result.Message = planningResponse.Message
		return result, nil

	case "tasks":
		// Task-based response - convert to ExecutionPlan
		executionPlan := &types.Plan{
			Goal:      planningResponse.Goal,
			Tasks:     make([]*types.Task, 0, len(planningResponse.Tasks)),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		// Convert planning tasks to execution tasks
		for _, planTask := range planningResponse.Tasks {
			task := &types.Task{
				ID:                 planTask.ID,
				Description:        planTask.Description,
				Status:             types.TaskPending,
				Rules:              planTask.Rules,
				ToolCalls:          planTask.ToolCalls,
				ValidationCriteria: planTask.ValidationCriteria,
				CreatedAt:          time.Now(),
				UpdatedAt:          time.Now(),
			}
			executionPlan.Tasks = append(executionPlan.Tasks, task)
		}

		result.Plan = executionPlan
		return result, nil

	default:
		return nil, fmt.Errorf("unknown response type: %s", planningResponse.ResponseType)
	}
}

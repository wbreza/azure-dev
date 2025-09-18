// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

//go:embed prompts/planning.txt
var planningPromptTemplate string

// PlanningAgent creates structured execution plans using LLM reasoning
type PlanningAgent struct {
	config *AgentConfig
}

// NewPlanningAgent creates a new planning agent
func NewPlanningAgent(opts ...AgentOption) *PlanningAgent {
	config := &AgentConfig{}
	for _, option := range opts {
		option(config)
	}

	return &PlanningAgent{
		config: config,
	}
}

// Plan generates a new execution plan for the given goal
func (a *PlanningAgent) Plan(ctx context.Context, goal string) (*types.PlanningResult, error) {
	// Create a new conversation buffer for planning
	conversationBuffer := langchainmemory.NewConversationBuffer()

	if a.config.plan != nil {
		existingPlanJson, err := json.MarshalIndent(a.config.plan, "", "  ")
		if err != nil {
			return nil, err
		}

		existingPlan := fmt.Sprintf("Existing Plan:\n```json\n%s\n```", string(existingPlanJson))
		conversationBuffer.ChatHistory.AddAIMessage(ctx, existingPlan)
	}

	// Add the user goal as context
	err := conversationBuffer.ChatHistory.AddMessage(ctx, llms.HumanChatMessage{
		Content: goal,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to add goal to conversation: %w", err)
	}

	// Create prompt builder for planning
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(planningPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversationBuffer(conversationBuffer),
	)

	// Evaluate the plan
	evalResult, err := a.evaluatePlan(ctx, promptBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate plan: %w", err)
	}

	// Convert PlanEvalResult to types.Plan
	plan := &types.Plan{
		Goal:      evalResult.Goal,
		Tasks:     evalResult.Tasks, // Direct assignment since they're already *Task
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Set status and timestamps for all tasks
	for _, task := range plan.Tasks {
		task.Status = types.TaskPending
		task.UpdatedAt = time.Now()
	}

	summaryAgent := NewSummaryAgent(WithConfig(a.config))
	summaryResult, err := summaryAgent.Summarize(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("failed to summarize plan: %w", err)
	}

	return &types.PlanningResult{
		Summary: summaryResult.Summary,
		Plan:    plan,
	}, nil
}

func (a *PlanningAgent) evaluatePlan(ctx context.Context, promptBuilder *PromptBuilder) (*types.PlanEvalResult, error) {
	// Build messages using the conversational prompt builder
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build planning messages: %w", err)
	}

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

	// Parse JSON response using the planning prompt format
	var planEvalResponse *types.PlanEvalResult
	if err := unmarshalJSONResponse(responseText, &planEvalResponse); err != nil {
		return nil, fmt.Errorf("failed to parse planning response as JSON: %w\nResponse: %s", err, responseText)
	}

	return &types.PlanEvalResult{
		Summary:  planEvalResponse.Summary,
		Goal:     planEvalResponse.Goal,
		Tasks:    planEvalResponse.Tasks,
		Insights: planEvalResponse.Insights,
	}, nil
}

// TODO: UpdatePlan and other methods to be redesigned following the new patterns

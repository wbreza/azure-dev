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
	langchainmemory "github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
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
func (a *PlanningAgent) Plan(ctx context.Context, userMessage string) (*types.PlanningResult, error) {
	// Create a new conversation buffer for planning
	conversationBuffer := langchainmemory.NewConversationBuffer(
		langchainmemory.WithChatHistory(a.config.conversation.ChatHistory),
	)

	planContext := "Current Plan: No active plan exists"
	if a.config.plan != nil {
		planJsonBytes, err := json.MarshalIndent(a.config.plan, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal current plan to JSON: %w", err)
		}

		planContext = fmt.Sprintf("Current Plan: \n```json\n%s```\n", string(planJsonBytes))
	}

	// Add the user goal as context
	if err := conversationBuffer.ChatHistory.AddUserMessage(ctx, userMessage); err != nil {
		return nil, fmt.Errorf("failed to add goal to conversation: %w", err)
	}

	// Create prompt builder for planning
	fullSystemMessage := fmt.Sprintf("%s\n\n%s", planningPromptTemplate, planContext)

	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(fullSystemMessage),
		WithPromptTools(a.config.tools),
		WithConversation(conversationBuffer),
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

	// Use the message from the planning evaluation or fall back to summary
	logMessage := evalResult.Message
	if logMessage == "" {
		logMessage = evalResult.Summary
	}

	a.config.callbacksHandler.HandleAgentFinish(ctx, schema.AgentFinish{
		Log: logMessage,
	})

	return &types.PlanningResult{
		Message: evalResult.Message,
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

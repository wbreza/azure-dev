// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/logging"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	uxlib "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/fatih/color"
	"github.com/tmc/langchaingo/memory"
)

//go:embed prompts/conversation.txt
var conversationPromptTemplate string

// OrchestratorAgent manages the enhanced ReAct loop with planning, execution, and validation
type OrchestratorAgent struct {
	// State management
	currentPlan *types.Plan

	// Configuration
	config *AgentConfig
}

// NewOrchestratorAgent creates a new orchestrator
func NewOrchestratorAgent(opts ...AgentOption) *OrchestratorAgent {
	config := &AgentConfig{}

	for _, option := range opts {
		option(config)
	}

	if config.callbacksHandler == nil {
		fileLogger, _, _ := logging.NewFileLoggerDefault()

		config.callbacksHandler = fileLogger
	}

	if config.conversation == nil {
		config.conversation = memory.NewConversationBuffer()
	}

	if config.maxIterations == 0 {
		config.maxIterations = 100
	}

	if config.maxFailedCycles == 0 {
		config.maxFailedCycles = 3
	}

	if config.thoughtChan == nil {
		thoughtChan := make(chan logging.Thought)
		config.thoughtChan = thoughtChan
	}

	return &OrchestratorAgent{
		config:      config,
		currentPlan: nil,
	}
}

func (a *OrchestratorAgent) SendMessage(ctx context.Context, args ...string) (string, error) {
	// Start thinking UI
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
	if err := a.config.conversation.ChatHistory.AddUserMessage(ctx, userMessage); err != nil {
		return "", err
	}

	// Route the message to determine intent
	routingAgent := NewRoutingAgent(WithConfig(a.config))
	routingResult, err := routingAgent.RouteMessage(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to route message: %w", err)
	}

	// Handle based on routing intent
	switch routingResult.Intent {
	case types.RoutingIntentConversational:
		// If routing has high confidence and includes a direct message, use it
		if routingResult.Confidence >= 0.8 && routingResult.Message != "" {
			// Add the direct response to conversation history
			err = a.config.conversation.ChatHistory.AddAIMessage(ctx, routingResult.Message)
			if err != nil {
				return "", fmt.Errorf("failed to add AI response to conversation: %w", err)
			}
			return routingResult.Message, nil
		}
		// Otherwise, use full conversational agent
		return a.handleConversational(ctx)

	case types.RoutingIntentReplan, types.RoutingIntentPlan:
		return a.handlePlanning(ctx, userMessage)

	case types.RoutingIntentExecute:
		return a.handleExecution(ctx)

	case types.RoutingIntentValidate:
		return a.handleValidation(ctx)

	case types.RoutingIntentProgress:
		return a.handleProgress(ctx)

	default:
		// Fallback to conversational if unknown intent
		return a.handleConversational(ctx)
	}
}

// handleConversational processes conversational messages by directly calling the LLM
func (a *OrchestratorAgent) handleConversational(ctx context.Context) (string, error) {
	// Create prompt builder for conversational response
	promptBuilder := NewPromptBuilder(
		WithSystemPrompt(conversationPromptTemplate),
		WithPromptTools(a.config.tools),
		WithConversation(a.config.conversation),
	)

	// Build messages using the prompt builder
	messages, err := promptBuilder.BuildMessages(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to build conversational messages: %w", err)
	}

	// Get response from LLM
	a.config.callbacksHandler.HandleLLMGenerateContentStart(ctx, messages)
	response, err := a.config.model.GenerateContent(ctx, messages)
	if err != nil {
		a.config.callbacksHandler.HandleLLMError(ctx, err)
		return "", fmt.Errorf("failed to generate conversational response: %w", err)
	}
	a.config.callbacksHandler.HandleLLMGenerateContentEnd(ctx, response)

	// Extract response text
	var responseText string
	if len(response.Choices) > 0 {
		responseText = response.Choices[0].Content
	}

	if responseText == "" {
		responseText = "I'm here to help! Could you please let me know what you'd like to do?"
	}

	// Add response to conversation history
	err = a.config.conversation.ChatHistory.AddAIMessage(ctx, responseText)
	if err != nil {
		return "", fmt.Errorf("failed to add AI response to conversation: %w", err)
	}

	return responseText, nil
}

// handlePlanning creates and executes a new plan
func (a *OrchestratorAgent) handlePlanning(ctx context.Context, userMessage string) (string, error) {
	// Create new plan
	planningAgent := NewPlanningAgent(WithConfig(a.config), WithPlan(a.currentPlan))
	planningResult, err := planningAgent.Plan(ctx, userMessage)
	if err != nil {
		return "", fmt.Errorf("failed to create plan: %w", err)
	}

	// Store the current plan
	a.currentPlan = planningResult.Plan

	if planningResult.Message != "" {
		if err := a.config.conversation.ChatHistory.AddAIMessage(ctx, planningResult.Message); err != nil {
			return "", err
		}

		return planningResult.Message, nil
	}

	return a.handleExecution(ctx)
}

// handleExecution executes an existing plan
func (a *OrchestratorAgent) handleExecution(ctx context.Context) (string, error) {
	if a.currentPlan == nil {
		return "There is no active plan to execute. Would you like me to help you create one?", nil
	}

	// Execute the current plan
	executionAgent := NewExecutionAgent(WithConfig(a.config), WithPlan(a.currentPlan))
	execPlanResult, err := executionAgent.ExecutePlan(ctx, a.currentPlan)
	if err != nil {
		return "", fmt.Errorf("failed to execute plan: %w", err)
	}

	// Check if execution paused for user message
	if execPlanResult.Message != "" {
		// Add the message to conversation history and return it to user
		err = a.config.conversation.ChatHistory.AddAIMessage(ctx, execPlanResult.Message)
		if err != nil {
			return "", fmt.Errorf("failed to add execution message to conversation: %w", err)
		}
		return execPlanResult.Message, nil
	}

	// Clear current plan if execution completed successfully
	if a.currentPlan.IsComplete() {
		a.currentPlan = nil
	}

	a.config.conversation.ChatHistory.AddAIMessage(ctx, execPlanResult.Summary)

	return execPlanResult.Summary, nil
}

// handleValidation checks the status of the current plan
func (a *OrchestratorAgent) handleValidation(ctx context.Context) (string, error) {
	if a.currentPlan == nil {
		return "There is no active plan to validate. Would you like me to help you with something else?", nil
	}

	// TODO: Implement plan level validation
	return "", nil
}

// handleProgress generates a detailed progress report of the current plan
func (a *OrchestratorAgent) handleProgress(ctx context.Context) (string, error) {
	if a.currentPlan == nil {
		return "No active plan to report progress on. Ready to start new work when you provide a goal or task.", nil
	}

	// Generate detailed progress report
	progressAgent := NewProgressAgent(WithConfig(a.config), WithPlan(a.currentPlan))
	progressResult, err := progressAgent.GenerateProgress(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to generate progress report: %w", err)
	}

	a.config.conversation.ChatHistory.AddAIMessage(ctx, progressResult.Progress)

	return progressResult.Progress, nil
}

// Stop terminates the agent and performs any necessary cleanup
func (a *OrchestratorAgent) Stop() error {
	if a.config.cleanupFunc != nil {
		return a.config.cleanupFunc()
	}

	return nil
}

func (a *OrchestratorAgent) renderThoughts(ctx context.Context) (func(), error) {
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

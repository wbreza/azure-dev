// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/tmc/langchaingo/llms"
	langchaingo_memory "github.com/tmc/langchaingo/memory"
)

// ConversationalPromptBuilder composes multi-message prompts for conversation-aware agents
type ConversationalPromptBuilder struct {
	systemPrompt           string
	tools                  []common.AnnotatedTool
	conversationBuffer     *langchaingo_memory.ConversationBuffer
	workingMemoryFormatter func(*memory.WorkingMemory) string
	includeWorkingMemory   bool
	includeConversation    bool
}

// ConversationalPromptOption configures the prompt builder
type ConversationalPromptOption func(*ConversationalPromptBuilder)

// NewConversationalPromptBuilder creates a new builder with options
func NewConversationalPromptBuilder(opts ...ConversationalPromptOption) *ConversationalPromptBuilder {
	builder := &ConversationalPromptBuilder{
		systemPrompt:         "You are a helpful AI assistant.",
		includeWorkingMemory: false,
		includeConversation:  false,
	}

	for _, opt := range opts {
		opt(builder)
	}

	return builder
}

// WithSystemPrompt sets the system prompt text
func WithSystemPrompt(prompt string) ConversationalPromptOption {
	return func(cpb *ConversationalPromptBuilder) {
		cpb.systemPrompt = prompt
	}
}

// WithPromptTools adds tools to the system prompt
func WithPromptTools(tools []common.AnnotatedTool) ConversationalPromptOption {
	return func(cpb *ConversationalPromptBuilder) {
		cpb.tools = tools
	}
}

// WithConversationBuffer adds conversation history to messages
func WithConversationBuffer(buffer *langchaingo_memory.ConversationBuffer) ConversationalPromptOption {
	return func(cpb *ConversationalPromptBuilder) {
		cpb.conversationBuffer = buffer
		cpb.includeConversation = true
	}
}

// WithWorkingMemory adds working memory context to system message
func WithWorkingMemory(formatter func(*memory.WorkingMemory) string) ConversationalPromptOption {
	return func(cpb *ConversationalPromptBuilder) {
		cpb.workingMemoryFormatter = formatter
		cpb.includeWorkingMemory = true
	}
}

// BuildMessages constructs the complete message slice for LLM calls
func (cpb *ConversationalPromptBuilder) BuildMessages(ctx context.Context,
	workingMemory *memory.WorkingMemory) ([]llms.MessageContent, error) {

	messages := []llms.MessageContent{}

	// 1. Build system message with working memory context
	systemText, err := cpb.buildSystemMessage(ctx, workingMemory)
	if err != nil {
		return nil, fmt.Errorf("failed to build system message: %w", err)
	}

	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeSystem,
		Parts: []llms.ContentPart{llms.TextPart(systemText)},
	})

	// 2. Add conversation history if configured
	if cpb.includeConversation && cpb.conversationBuffer != nil {
		conversationMessages, err := cpb.buildConversationMessages(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to build conversation messages: %w", err)
		}
		messages = append(messages, conversationMessages...)
	}

	return messages, nil
}

// buildSystemMessage creates the system message with optional working memory context
func (cpb *ConversationalPromptBuilder) buildSystemMessage(ctx context.Context,
	workingMemory *memory.WorkingMemory) (string, error) {

	var systemParts []string

	// 1. Base system prompt (personality/role)
	if cpb.systemPrompt != "" {
		systemParts = append(systemParts, cpb.systemPrompt)
	}

	// 2. Tools section
	if len(cpb.tools) > 0 {
		toolDescriptions := cpb.formatToolsSection()
		if toolDescriptions != "" {
			systemParts = append(systemParts, toolDescriptions)
		}
	}

	// 3. Working memory context (current state)
	if cpb.includeWorkingMemory && cpb.workingMemoryFormatter != nil && workingMemory != nil {
		workingMemoryContext := cpb.workingMemoryFormatter(workingMemory)
		if workingMemoryContext != "" {
			systemParts = append(systemParts, "## CURRENT CONTEXT")
			systemParts = append(systemParts, workingMemoryContext)
		}
	}

	return strings.Join(systemParts, "\n\n"), nil
}

// formatToolsSection creates a formatted tools description section
func (cpb *ConversationalPromptBuilder) formatToolsSection() string {
	if len(cpb.tools) == 0 {
		return ""
	}

	var toolParts []string
	toolParts = append(toolParts, "## Available Tools")
	toolParts = append(toolParts, "You have access to the following tools:")

	for _, tool := range cpb.tools {
		toolDesc := fmt.Sprintf("- **%s**: %s", tool.Name(), tool.Description())
		toolParts = append(toolParts, toolDesc)
	}

	return strings.Join(toolParts, "\n")
}

// buildConversationMessages converts conversation buffer to message format
func (cpb *ConversationalPromptBuilder) buildConversationMessages(ctx context.Context) ([]llms.MessageContent, error) {
	if cpb.conversationBuffer == nil {
		return []llms.MessageContent{}, nil
	}

	// Get conversation history from buffer
	conversationHistory, err := cpb.conversationBuffer.ChatHistory.Messages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get conversation history: %w", err)
	}

	messages := []llms.MessageContent{}

	// Convert each message in the conversation history
	for _, msg := range conversationHistory {
		role := msg.GetType()
		content := msg.GetContent()

		if content != "" {
			messages = append(messages, llms.MessageContent{
				Role:  role,
				Parts: []llms.ContentPart{llms.TextPart(content)},
			})
		}
	}

	return messages, nil
}

// StandardWorkingMemoryFormatter provides a default formatter for working memory context
func StandardWorkingMemoryFormatter(wm *memory.WorkingMemory) string {
	if wm == nil {
		return ""
	}

	var builder strings.Builder

	// Add current goal
	goal := wm.GetGoal()
	if goal != "" {
		builder.WriteString(fmt.Sprintf("CURRENT GOAL: %s\n\n", goal))
	}

	// Add task status summary
	taskSummary := wm.GetTaskStatusSummary()
	taskSummaryText := taskSummary.ToPromptFormat()
	if taskSummaryText != "" {
		builder.WriteString("TASK PROGRESS:\n")
		builder.WriteString(taskSummaryText)
		builder.WriteString("\n")
	}

	// Add recent execution history
	recentHistory := wm.GetRecentHistory(5)
	if len(recentHistory) > 0 {
		builder.WriteString("RECENT ACTIVITY:\n")
		for _, event := range recentHistory {
			builder.WriteString(fmt.Sprintf("- %s: %s\n", event.Type, event.Details))
		}
	}

	return builder.String()
}

// CompactWorkingMemoryFormatter provides a more concise formatter for working memory
func CompactWorkingMemoryFormatter(wm *memory.WorkingMemory) string {
	if wm == nil {
		return ""
	}

	var builder strings.Builder

	// Just goal and active task for compact version
	goal := wm.GetGoal()
	if goal != "" {
		builder.WriteString(fmt.Sprintf("Goal: %s", goal))
	}

	taskSummary := wm.GetTaskStatusSummary()
	if taskSummary.ActiveTask != "" {
		if builder.Len() > 0 {
			builder.WriteString(" | ")
		}
		builder.WriteString(fmt.Sprintf("Active: %s", taskSummary.ActiveTask))
	}

	return builder.String()
}

// DetailedWorkingMemoryFormatter provides comprehensive working memory context
func DetailedWorkingMemoryFormatter(wm *memory.WorkingMemory) string {
	if wm == nil {
		return ""
	}

	var builder strings.Builder

	// Add current goal
	goal := wm.GetGoal()
	if goal != "" {
		builder.WriteString(fmt.Sprintf("CURRENT GOAL: %s\n\n", goal))
	}

	// Add detailed task status
	taskSummary := wm.GetTaskStatusSummary()
	taskSummaryText := taskSummary.ToPromptFormat()
	if taskSummaryText != "" {
		builder.WriteString("DETAILED TASK PROGRESS:\n")
		builder.WriteString(taskSummaryText)
		builder.WriteString("\n")
	}

	// Add comprehensive recent history
	recentHistory := wm.GetRecentHistory(10)
	if len(recentHistory) > 0 {
		builder.WriteString("RECENT EXECUTION HISTORY:\n")
		for i, event := range recentHistory {
			builder.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, event.Type, event.Details))
		}
		builder.WriteString("\n")
	}

	// Add evidence if available
	// Note: This would require a GetAllEvidence method on WorkingMemory
	// For now, we'll include this as a placeholder for future enhancement

	return builder.String()
}

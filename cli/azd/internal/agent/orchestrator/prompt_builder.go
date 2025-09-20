// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/tmc/langchaingo/llms"
	langchaingo_memory "github.com/tmc/langchaingo/memory"
)

// ContextItem represents a labeled context object for the system prompt
type ContextItem struct {
	Label  string
	Object any
}

// PromptBuilder composes multi-message prompts for conversation-aware agents
type PromptBuilder struct {
	systemPrompt        string
	tools               []common.AnnotatedTool
	contexts            []ContextItem
	conversationBuffer  *langchaingo_memory.ConversationBuffer
	includeConversation bool
}

// ConversationalPromptOption configures the prompt builder
type ConversationalPromptOption func(*PromptBuilder)

// NewPromptBuilder creates a new builder with options
func NewPromptBuilder(opts ...ConversationalPromptOption) *PromptBuilder {
	builder := &PromptBuilder{
		systemPrompt:        "You are a helpful AI assistant.",
		includeConversation: false,
	}

	for _, opt := range opts {
		opt(builder)
	}

	return builder
}

// WithSystemPrompt sets the system prompt text
func WithSystemPrompt(prompt string) ConversationalPromptOption {
	return func(cpb *PromptBuilder) {
		cpb.systemPrompt = prompt
	}
}

// WithPromptTools adds tools to the system prompt
func WithPromptTools(tools []common.AnnotatedTool) ConversationalPromptOption {
	return func(cpb *PromptBuilder) {
		cpb.tools = tools
	}
}

// WithContext adds a labeled context object to the system prompt
func WithContext(label string, object any) ConversationalPromptOption {
	return func(cpb *PromptBuilder) {
		cpb.contexts = append(cpb.contexts, ContextItem{
			Label:  label,
			Object: object,
		})
	}
}

// WithConversation adds conversation history to messages
func WithConversation(buffer *langchaingo_memory.ConversationBuffer) ConversationalPromptOption {
	return func(cpb *PromptBuilder) {
		cpb.conversationBuffer = buffer
		cpb.includeConversation = true
	}
}

// BuildMessages constructs the complete message slice for LLM calls
func (pb *PromptBuilder) BuildMessages(ctx context.Context) ([]llms.MessageContent, error) {

	messages := []llms.MessageContent{}

	// 1. Base system prompt (without contexts)
	systemPrompt, err := pb.buildBaseSystemMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build system prompt: %w", err)
	}

	messages = append(messages, llms.MessageContent{
		Role:  llms.ChatMessageTypeSystem,
		Parts: []llms.ContentPart{llms.TextPart(systemPrompt)},
	})

	// 2. Conversation history
	if pb.includeConversation && pb.conversationBuffer != nil {
		conversationMessages, err := pb.buildConversationMessages(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to build conversation messages: %w", err)
		}
		messages = append(messages, conversationMessages...)
	}

	// 3. Additional context as AI messages
	contextMessages, err := pb.buildContextMessages(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build context messages: %w", err)
	}
	messages = append(messages, contextMessages...)

	return messages, nil
}

// buildBaseSystemMessage creates the system message with just the base prompt and tools (no contexts)
func (pb *PromptBuilder) buildBaseSystemMessage(ctx context.Context) (string, error) {

	var systemParts []string

	// 1. Base system prompt (personality/role)
	if pb.systemPrompt != "" {
		systemParts = append(systemParts, pb.systemPrompt)
	}

	// 2. Tools section
	if len(pb.tools) > 0 {
		toolDescriptions := pb.formatToolsSection()
		if toolDescriptions != "" {
			systemParts = append(systemParts, toolDescriptions)
		}
	}

	return strings.Join(systemParts, "\n\n"), nil
}

// buildContextMessages creates AI messages for each context item
func (pb *PromptBuilder) buildContextMessages(ctx context.Context) ([]llms.MessageContent, error) {
	if len(pb.contexts) == 0 {
		return []llms.MessageContent{}, nil
	}

	var messages []llms.MessageContent

	for _, context := range pb.contexts {
		// Skip nil objects
		if context.Object == nil {
			continue
		}

		// Marshal object to indented JSON
		contextJSON, err := json.MarshalIndent(context.Object, "", "  ")
		if err != nil {
			// Skip sections with marshal errors
			continue
		}

		// Create AI message for this context
		contextContent := fmt.Sprintf("## %s\n```json\n%s\n```", context.Label, string(contextJSON))

		messages = append(messages, llms.MessageContent{
			Role:  llms.ChatMessageTypeAI,
			Parts: []llms.ContentPart{llms.TextPart(contextContent)},
		})
	}

	return messages, nil
}

// buildSystemMessage creates the system message with optional working memory context (DEPRECATED - keeping for compatibility)
func (pb *PromptBuilder) buildSystemMessage(ctx context.Context) (string, error) {

	var systemParts []string

	// 1. Base system prompt (personality/role)
	if pb.systemPrompt != "" {
		systemParts = append(systemParts, pb.systemPrompt)
	}

	// 2. Tools section
	if len(pb.tools) > 0 {
		toolDescriptions := pb.formatToolsSection()
		if toolDescriptions != "" {
			systemParts = append(systemParts, toolDescriptions)
		}
	}

	// 3. Contexts section
	contextsSection := pb.formatContextsSection()
	if contextsSection != "" {
		systemParts = append(systemParts, contextsSection)
	}

	return strings.Join(systemParts, "\n\n"), nil
}

// formatToolsSection creates a formatted tools description section
func (pb *PromptBuilder) formatToolsSection() string {
	if len(pb.tools) == 0 {
		return ""
	}

	var toolParts []string
	toolParts = append(toolParts, "## Available Tools")
	toolParts = append(toolParts, "You have access to the following tools:")

	for _, tool := range pb.tools {
		toolDesc := fmt.Sprintf("- **%s**: %s", tool.Name(), tool.Description())
		toolParts = append(toolParts, toolDesc)
	}

	return strings.Join(toolParts, "\n")
}

// formatContextsSection creates formatted context sections with JSON
func (pb *PromptBuilder) formatContextsSection() string {
	if len(pb.contexts) == 0 {
		return ""
	}

	var contextParts []string

	for _, context := range pb.contexts {
		// Skip nil objects
		if context.Object == nil {
			continue
		}

		// Add section header
		contextParts = append(contextParts, fmt.Sprintf("## %s", context.Label))

		// Marshal object to indented JSON
		contextJSON, err := json.MarshalIndent(context.Object, "", "  ")
		if err != nil {
			// Skip sections with marshal errors as requested
			continue
		}

		contextParts = append(contextParts, "```json")
		contextParts = append(contextParts, string(contextJSON))
		contextParts = append(contextParts, "```")
		contextParts = append(contextParts, "") // Add spacing between contexts
	}

	// Remove trailing empty line if present
	if len(contextParts) > 0 && contextParts[len(contextParts)-1] == "" {
		contextParts = contextParts[:len(contextParts)-1]
	}

	return strings.Join(contextParts, "\n")
}

// buildConversationMessages converts conversation buffer to message format
func (pb *PromptBuilder) buildConversationMessages(ctx context.Context) ([]llms.MessageContent, error) {
	if pb.conversationBuffer == nil {
		return []llms.MessageContent{}, nil
	}

	// Get conversation history from buffer
	conversationHistory, err := pb.conversationBuffer.ChatHistory.Messages(ctx)
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

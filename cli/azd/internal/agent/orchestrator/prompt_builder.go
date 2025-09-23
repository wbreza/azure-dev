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
	"github.com/tmc/langchaingo/schema"
)

type promptFormatterFunc func() ([]llms.ChatMessage, error)

// ContextItem represents a labeled context object for the system prompt
type ContextItem struct {
	Label  string
	Object any
}

// PromptBuilder composes multi-message prompts for conversation-aware agents
type PromptBuilder struct {
	systemParts []string
	formatters  []promptFormatterFunc
}

// NewPromptBuilder creates a new builder with options
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{
		systemParts: []string{},
		formatters:  []promptFormatterFunc{},
	}
}

// WithSystemPrompt sets the system prompt text
func (pb *PromptBuilder) WithSystemPrompt(prompt string) *PromptBuilder {
	pb.systemParts = append(pb.systemParts, prompt)
	return pb
}

// WithPromptTools adds tools to the system prompt
func (pb *PromptBuilder) WithPromptTools(tools []common.AnnotatedTool) *PromptBuilder {
	if len(tools) > 0 {
		toolDescriptions := pb.formatToolsSection(tools)
		if toolDescriptions != "" {
			pb.systemParts = append(pb.systemParts, toolDescriptions)
		}
	}
	return pb
}

// WithContext adds a labeled context object to the system prompt
func (pb *PromptBuilder) WithContext(label string, object any) *PromptBuilder {
	contextItem := ContextItem{
		Label:  label,
		Object: object,
	}

	pb.formatters = append(pb.formatters, func() ([]llms.ChatMessage, error) {
		return pb.buildContextMessage(contextItem)
	})

	return pb
}

// WithConversation adds conversation history to messages
func (pb *PromptBuilder) WithConversation(ctx context.Context, chatHistory schema.ChatMessageHistory) *PromptBuilder {
	pb.formatters = append(pb.formatters, func() ([]llms.ChatMessage, error) {
		messages, err := chatHistory.Messages(ctx)
		if err != nil {
			return nil, err
		}

		return messages, nil
	})

	return pb
}

// BuildMessages constructs the complete message slice for LLM calls
func (pb *PromptBuilder) Build(ctx context.Context) ([]llms.MessageContent, error) {
	chatMessages := []llms.ChatMessage{}

	// 1. Base system prompt (without contexts)
	systemPrompt, err := pb.buildBaseSystemMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to build system prompt: %w", err)
	}

	chatMessages = append(chatMessages, systemPrompt)

	// 2. Other sections in order
	for _, formatter := range pb.formatters {
		messages, err := formatter()
		if err != nil {
			return nil, fmt.Errorf("failed to build prompt section: %w", err)
		}

		chatMessages = append(chatMessages, messages...)
	}

	allMessage := make([]llms.MessageContent, len(chatMessages))
	for i, msg := range chatMessages {
		allMessage[i] = llms.MessageContent{
			Role:  msg.GetType(),
			Parts: []llms.ContentPart{llms.TextPart(msg.GetContent())},
		}
	}

	return allMessage, nil
}

// buildBaseSystemMessage creates the system message with just the base prompt and tools (no contexts)
func (pb *PromptBuilder) buildBaseSystemMessage(ctx context.Context) (llms.ChatMessage, error) {
	return llms.SystemChatMessage{
		Content: strings.Join(pb.systemParts, "\n\n"),
	}, nil
}

// buildContextMessage creates AI messages for each context item
func (pb *PromptBuilder) buildContextMessage(item ContextItem) ([]llms.ChatMessage, error) {
	// Marshal object to indented JSON
	contextJSON, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return nil, err
	}

	return []llms.ChatMessage{
		llms.AIChatMessage{
			Content: fmt.Sprintf("## %s\n```json\n%s\n```", item.Label, string(contextJSON)),
		},
	}, nil
}

// formatToolsSection creates a formatted tools description section
func (pb *PromptBuilder) formatToolsSection(tools []common.AnnotatedTool) string {
	if len(tools) == 0 {
		return ""
	}

	var toolParts []string
	toolParts = append(toolParts, "## Available Tools")
	toolParts = append(toolParts, "You have access to the following tools:")

	for _, tool := range tools {
		toolDesc := fmt.Sprintf("- **%s**: %s", tool.Name(), tool.Description())
		toolParts = append(toolParts, toolDesc)
	}

	return strings.Join(toolParts, "\n")
}

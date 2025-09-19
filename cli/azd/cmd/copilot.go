// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/internal/agent"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/azure/azure-dev/cli/azd/pkg/output/ux"
	uxlib "github.com/azure/azure-dev/cli/azd/pkg/ux"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Register copilot commands
func copilotActions(root *actions.ActionDescriptor) *actions.ActionDescriptor {
	group := root.Add("copilot", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{
			Use:   "copilot",
			Short: "Interact with Azure Developer CLI AI assistant.",
		},
		GroupingOptions: actions.CommandGroupOptions{
			RootLevelHelp: actions.CmdGroupManage, // Categorize under manage and show settings
		},
	})

	// azd copilot chat
	group.Add("chat", &actions.ActionDescriptorOptions{
		Command: &cobra.Command{
			Use:   "chat [message]",
			Short: "Start an interactive chat session with the Azure Developer CLI AI assistant.",
			Long: `Start an interactive chat session with the Azure Developer CLI AI assistant.
The assistant can help with Azure development tasks, project setup, troubleshooting, and more.

Examples:
  azd copilot chat                          # Start interactive chat
  azd copilot chat "help me deploy to Azure"  # Send a single message`,
			Args: cobra.MaximumNArgs(1),
		},
		OutputFormats:  []output.Format{output.NoneFormat},
		DefaultFormat:  output.NoneFormat,
		ActionResolver: newCopilotChatAction,
		FlagsResolver:  newCopilotChatFlags,
	})

	return group
}

// Flags for chat command
type copilotChatFlags struct {
	global *internal.GlobalCommandOptions
	internal.EnvFlag
}

func newCopilotChatFlags(cmd *cobra.Command, global *internal.GlobalCommandOptions) *copilotChatFlags {
	flags := &copilotChatFlags{}
	flags.Bind(cmd.Flags(), global)
	return flags
}

func (f *copilotChatFlags) Bind(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	f.EnvFlag.Bind(local, global)
	f.global = global
}

// Action for chat command
type copilotChatAction struct {
	args         []string
	flags        *copilotChatFlags
	console      input.Console
	agentFactory *agent.AgentFactory
}

func newCopilotChatAction(
	args []string,
	flags *copilotChatFlags,
	console input.Console,
	agentFactory *agent.AgentFactory,
) actions.Action {
	return &copilotChatAction{
		args:         args,
		flags:        flags,
		console:      console,
		agentFactory: agentFactory,
	}
}

func (a *copilotChatAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	agent, err := a.agentFactory.Create(ctx,
		agent.WithDebug(a.flags.global.EnableDebugLogging),
	)
	if err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}

	var userInput string
	if len(a.args) > 0 {
		userInput = a.args[0]
	}

	defer agent.Stop()

	for {
		if userInput == "" {
			messagePrompt := uxlib.NewPrompt(&uxlib.PromptOptions{
				Message:        "You",
				Required:       true,
				HelpMessage:    "Type a message to the agent",
				IgnoreHintKeys: true,
			})

			userInput, err = messagePrompt.Ask(ctx)
			if err != nil {
				if errors.Is(err, uxlib.ErrCancelled) {
					break
				}

				return nil, fmt.Errorf("prompting for message: %w", err)
			}
		}

		outputMessage, err := agent.SendMessage(ctx, userInput)
		if err != nil {
			a.console.Message(ctx, "")
			a.console.Message(ctx, output.WithErrorFormat(err.Error()))
			a.console.Message(ctx, "")
			log.Printf("sending message to agent: %v", err.Error())
		}
		if outputMessage != "" {
			a.console.Message(ctx, "")
			a.console.Message(ctx, outputMessage)
			a.console.Message(ctx, "")
		}

		userInput = ""
	}

	return &actions.ActionResult{
		Message: &actions.ResultMessage{
			Header: "AI assistant session completed.",
		},
	}, nil
}

func (a *copilotChatAction) handleSingleMessage(ctx context.Context, message string) (*actions.ActionResult, error) {
	if message == "" {
		// Prompt for message if not provided
		userMessage, err := a.console.Prompt(ctx, input.ConsoleOptions{
			Message: "Enter your message for the AI assistant:",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get user input: %w", err)
		}
		message = userMessage
	}

	a.console.MessageUxItem(ctx, &ux.MultilineMessage{
		Lines: []string{
			"Processing your message...",
			fmt.Sprintf("Message: %s", output.WithHighLightFormat(message)),
		},
	})

	// TODO: Implement actual AI service call
	// response, err := a.aiService.Chat(ctx, message, a.flags.system, a.flags.model)
	// if err != nil {
	//     return nil, fmt.Errorf("failed to get AI response: %w", err)
	// }

	// Placeholder response
	response := fmt.Sprintf("AI Assistant: Thank you for your message: '%s'. This is a placeholder response. The actual AI integration will be implemented here.", message)

	a.console.Message(ctx, response)

	return &actions.ActionResult{
		Message: &actions.ResultMessage{
			Header: "Chat session completed successfully.",
		},
	}, nil
}

func (a *copilotChatAction) handleInteractiveChat(ctx context.Context) (*actions.ActionResult, error) {
	a.console.MessageUxItem(ctx, &ux.MultilineMessage{
		Lines: []string{
			"Starting interactive chat session...",
			"Type 'exit' or 'quit' to end the session.",
			"Type 'help' for available commands.",
			"",
		},
	})

	for {
		// Prompt for user input
		userInput, err := a.console.Prompt(ctx, input.ConsoleOptions{
			Message: "You:",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to get user input: %w", err)
		}

		// Check for exit commands
		switch userInput {
		case "exit", "quit", ":q":
			a.console.Message(ctx, "Goodbye! Chat session ended.")
			return &actions.ActionResult{
				Message: &actions.ResultMessage{
					Header: "Interactive chat session completed.",
				},
			}, nil
		case "help":
			a.console.MessageUxItem(ctx, &ux.MultilineMessage{
				Lines: []string{
					"Available commands:",
					"  exit, quit, :q  - End the chat session",
					"  help           - Show this help message",
					"",
					"Otherwise, type any message to chat with the AI assistant.",
					"",
				},
			})
			continue
		}

		// Skip empty messages
		if userInput == "" {
			continue
		}

		// TODO: Implement actual AI service call
		// response, err := a.aiService.Chat(ctx, userInput, a.flags.system, a.flags.model)
		// if err != nil {
		//     a.console.Message(ctx, output.WithErrorFormat("Error: %s", err.Error()))
		//     continue
		// }

		// Placeholder response
		response := fmt.Sprintf("AI Assistant: You said '%s'. This is a placeholder response. The actual AI integration will be implemented here.", userInput)

		a.console.Message(ctx, response)
		a.console.Message(ctx, "") // Add blank line for readability
	}
}

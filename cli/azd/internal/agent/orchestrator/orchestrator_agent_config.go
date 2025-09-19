package orchestrator

import (
	"github.com/azure/azure-dev/cli/azd/internal/agent/logging"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
	"github.com/tmc/langchaingo/callbacks"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
)

type AgentConfig struct {
	plan             *types.Plan
	conversation     *memory.ConversationBuffer
	tools            []common.AnnotatedTool
	model            llms.Model
	maxIterations    int
	maxFailedCycles  int
	thoughtChan      chan logging.Thought
	cleanupFunc      func() error
	callbacksHandler callbacks.Handler
}

type AgentOption func(config *AgentConfig)

func WithConfig(config *AgentConfig) AgentOption {
	return func(c *AgentConfig) {
		*c = *config
	}
}

func WithConversationBuffer(conversation *memory.ConversationBuffer) AgentOption {
	return func(config *AgentConfig) {
		config.conversation = conversation
	}
}

func WithPlan(plan *types.Plan) AgentOption {
	return func(config *AgentConfig) {
		config.plan = plan
	}
}

func WithTools(tools ...common.AnnotatedTool) AgentOption {
	return func(config *AgentConfig) {
		config.tools = tools
	}
}

func WithModel(model llms.Model) AgentOption {
	return func(config *AgentConfig) {
		config.model = model
	}
}

func WithMaxIterations(max int) AgentOption {
	return func(config *AgentConfig) {
		config.maxIterations = max
	}
}

func WithMaxFailedCycles(max int) AgentOption {
	return func(config *AgentConfig) {
		config.maxFailedCycles = max
	}
}

func WithThoughtChannel(thoughtChan chan logging.Thought) AgentOption {
	return func(config *AgentConfig) {
		config.thoughtChan = thoughtChan
	}
}

func WithCleanup(cleanupFunc func() error) AgentOption {
	return func(config *AgentConfig) {
		config.cleanupFunc = cleanupFunc
	}
}

func WithCallbacksHandler(handler callbacks.Handler) AgentOption {
	return func(config *AgentConfig) {
		config.callbacksHandler = handler
	}
}

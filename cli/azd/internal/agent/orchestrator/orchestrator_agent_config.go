package orchestrator

import (
	"github.com/azure/azure-dev/cli/azd/internal/agent/logging"
	"github.com/azure/azure-dev/cli/azd/internal/agent/memory"
	"github.com/azure/azure-dev/cli/azd/internal/agent/tools/common"
	"github.com/tmc/langchaingo/llms"
	langchainmemory "github.com/tmc/langchaingo/memory"
)

type OrchestratorAgentConfig struct {
	tools              []common.AnnotatedTool
	conversationBuffer *langchainmemory.ConversationBuffer
	workingMemory      *memory.WorkingMemory
	model              llms.Model
	maxIterations      int
	maxFailedCycles    int
	thoughtChan        chan logging.Thought
}

type OrchestratorAgentOption func(config *OrchestratorAgentConfig)

func WithOrchestrationConfig(config *OrchestratorAgentConfig) OrchestratorAgentOption {
	return func(c *OrchestratorAgentConfig) {
		c = config
	}
}

func WithOrchestrationTools(tools ...common.AnnotatedTool) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.tools = tools
	}
}

func WithOrchestrationHistory(buffer *langchainmemory.ConversationBuffer) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.conversationBuffer = buffer
	}
}

func WithOrchestrationMemory(workingMemory *memory.WorkingMemory) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.workingMemory = workingMemory
	}
}

func WithOrchestrationModel(model llms.Model) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.model = model
	}
}

func WithOrchestrationMaxIterations(max int) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.maxIterations = max
	}
}

func WithOrchestrationMaxFailedCycles(max int) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.maxFailedCycles = max
	}
}

func WithOrchestrationThoughtChannel(thoughtChan chan logging.Thought) OrchestratorAgentOption {
	return func(config *OrchestratorAgentConfig) {
		config.thoughtChan = thoughtChan
	}
}

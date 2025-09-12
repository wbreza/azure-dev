# Enhanced ReAct Agent Implementation Plan

## Overview

Implementation of a sophisticated task-oriented agent system that combines langchain constructs with custom orchestration for complex Azure Developer CLI workflows.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Agent Orchestrator                       │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │   Planning  │  │  Execution  │  │ Validation  │         │
│  │    Agent    │  │    Agent    │  │    Agent    │         │
│  │ (langchain) │  │ (langchain) │  │ (langchain) │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
├─────────────────────────────────────────────────────────────┤
│                   Working Memory                            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐         │
│  │ Execution   │  │ Task Status │  │ Evidence    │         │
│  │    Plan     │  │   Tracker   │  │   Store     │         │
│  └─────────────┘  └─────────────┘  └─────────────┘         │
├─────────────────────────────────────────────────────────────┤
│                 Langchain Foundation                        │
│         LLMs • Tools • Chains • Executors • Memory         │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Agent Orchestrator
**Custom implementation** that manages the enhanced ReAct loop

### 2. Specialized Agents (Langchain-based)
- **Planning Agent**: Creates structured task plans
- **Execution Agent**: Executes tasks using available tools  
- **Validation Agent**: Validates task completion with evidence

### 3. Working Memory System
**Custom implementation** for persistent, non-summarized state

---

## Implementation Phases

## Phase 1: Core Data Structures

### 1.1 Task Management Types

```go
// File: internal/agent/types/task.go

type TaskStatus string
const (
    TaskPending     TaskStatus = "pending"
    TaskInProgress  TaskStatus = "inProgress" 
    TaskComplete    TaskStatus = "complete"
    TaskFailed      TaskStatus = "failed"
    TaskBlocked     TaskStatus = "blocked"
)

type Task struct {
    ID                 string            `json:"id"`
    Description        string            `json:"description"`
    Status             TaskStatus        `json:"status"`
    Dependencies       []string          `json:"dependencies,omitempty"`
    Rules              []string          `json:"rules,omitempty"`
    ToolCalls          []PlannedToolCall `json:"toolCalls,omitempty"`
    ValidationCriteria string            `json:"validationCriteria"`
    CreatedAt          time.Time         `json:"createdAt"`
    UpdatedAt          time.Time         `json:"updatedAt"`
}

type PlannedToolCall struct {
    ToolName    string            `json:"toolName"`
    Parameters  map[string]any    `json:"parameters"`
    Reasoning   string            `json:"reasoning"`
}

type ExecutionPlan struct {
    ID        string    `json:"id"`
    Goal      string    `json:"goal"`
    Tasks     []*Task   `json:"tasks"`
    Status    string    `json:"status"`
    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}
```

### 1.2 Agent Response Types

```go
// File: internal/agent/types/response.go

type AgentResponse struct {
    Thought       string              `json:"thought"`
    Observation   string              `json:"observation"`
    CompleteTasks []CompletedTask     `json:"completeTasks,omitempty"`
    Actions       []ActionRequest     `json:"actions,omitempty"`
}

type CompletedTask struct {
    TaskID     string `json:"taskId"`
    Evidence   string `json:"evidence"`
    Confidence string `json:"confidence"` // high|medium|low
}

type ActionRequest struct {
    Tool      string         `json:"tool"`
    Input     map[string]any `json:"input"`
    Reasoning string         `json:"reasoning"`
}
```

### 1.3 Working Memory

```go
// File: internal/agent/memory/working_memory.go

type WorkingMemory struct {
    mu           sync.RWMutex
    executionPlan *ExecutionPlan
    taskStatus   map[string]TaskStatusSummary
    evidence     map[string][]Evidence
    history      []ExecutionEvent
}

type TaskStatusSummary struct {
    ID           string     `json:"id"`
    Status       TaskStatus `json:"status"`
    Brief        string     `json:"brief"`
    BlockedBy    string     `json:"blockedBy,omitempty"`
    EvidenceBrief string    `json:"evidenceBrief,omitempty"`
}

type Evidence struct {
    TaskID      string    `json:"taskId"`
    Description string    `json:"description"`
    Details     string    `json:"details"`
    Confidence  string    `json:"confidence"`
    Timestamp   time.Time `json:"timestamp"`
}

type ExecutionEvent struct {
    Type        string    `json:"type"` // actionExecuted, taskCompleted, planUpdated
    TaskID      string    `json:"taskId,omitempty"`
    Details     string    `json:"details"`
    Timestamp   time.Time `json:"timestamp"`
}
```

## Phase 2: Langchain Agent Implementations

### 2.1 Enhanced Planning Agent
```go
// File: internal/agent/planning_agent_v2.go

type EnhancedPlanningAgent struct {
    llm          llms.Model
    tools        []common.AnnotatedTool
    promptTemplate prompts.PromptTemplate
}

func NewEnhancedPlanningAgent(llm llms.Model, tools []common.AnnotatedTool) *EnhancedPlanningAgent {
    return &EnhancedPlanningAgent{
        llm:   llm,
        tools: tools,
        promptTemplate: prompts.PromptTemplate{
            Template: planning_prompt_template_v2,
            TemplateFormat: prompts.TemplateFormatGoTemplate,
            InputVariables: []string{"goal", "tools", "context"},
        },
    }
}

func (p *EnhancedPlanningAgent) CreatePlan(ctx context.Context, goal string, context string) (*ExecutionPlan, error) {
    // Generate plan using langchain constructs
}

func (p *EnhancedPlanningAgent) UpdatePlan(ctx context.Context, currentPlan *ExecutionPlan, newContext string) (*ExecutionPlan, error) {
    // Update existing plan based on new information
}
```

### 2.2 Execution Agent
```go
// File: internal/agent/execution_agent.go

type ExecutionAgent struct {
    agent    agents.Agent  // Langchain agent
    executor *agents.Executor
    memory   memory.ConversationBuffer
}

func NewExecutionAgent(llm llms.Model, tools []common.AnnotatedTool) (*ExecutionAgent, error) {
    // Use langchain's ConversationalAgent
    conversationalAgent := agents.NewConversationalAgent(
        llm, 
        common.ToLangChainTools(tools),
        agents.WithPrompt(executionPromptTemplate),
        agents.WithMemory(conversationMemory),
    )
    
    executor := agents.NewExecutor(
        conversationalAgent,
        agents.WithMaxIterations(10),
        agents.WithReturnIntermediateSteps(),
    )
    
    return &ExecutionAgent{
        agent:    conversationalAgent,
        executor: executor,
        memory:   conversationMemory,
    }, nil
}

func (e *ExecutionAgent) ExecuteActions(ctx context.Context, actions []ActionRequest, workingMemory *WorkingMemory) ([]ActionResult, error) {
    // Execute actions using langchain executor
}
```

### 2.3 Validation Agent
```go
// File: internal/agent/validation_agent.go

type ValidationAgent struct {
    llm            llms.Model
    promptTemplate prompts.PromptTemplate
}

func (v *ValidationAgent) ValidateCompletedTasks(ctx context.Context, tasks []CompletedTask, workingMemory *WorkingMemory) ([]ValidationResult, error) {
    // Use langchain LLM to validate task completion
}
```

## Phase 3: Orchestrator Implementation

### 3.1 Main Orchestrator
```go
// File: internal/agent/orchestrator.go

type Orchestrator struct {
    planningAgent   *EnhancedPlanningAgent
    executionAgent  *ExecutionAgent
    validationAgent *ValidationAgent
    workingMemory   *WorkingMemory
    config          OrchestratorConfig
}

type OrchestratorConfig struct {
    MaxIterations    int
    ReplanThreshold  int
    ValidationMode   string // strict|lenient
    DebugMode        bool
}

func (o *Orchestrator) ExecuteGoal(ctx context.Context, goal string) (*ExecutionSummary, error) {
    // Main execution loop implementing enhanced ReAct pattern
    
    // 1. Create initial plan
    plan, err := o.planningAgent.CreatePlan(ctx, goal, "")
    if err != nil {
        return nil, err
    }
    o.workingMemory.SetExecutionPlan(plan)
    
    // 2. Enhanced ReAct loop
    for iteration := 0; iteration < o.config.MaxIterations; iteration++ {
        if o.workingMemory.IsComplete() {
            break
        }
        
        // Build context for execution agent
        context := o.buildExecutionContext()
        
        // Get agent response
        response, err := o.getAgentResponse(ctx, context)
        if err != nil {
            return nil, err
        }
        
        // Execute actions
        actionResults, err := o.executionAgent.ExecuteActions(ctx, response.Actions, o.workingMemory)
        if err != nil {
            return nil, err
        }
        
        // Validate completed tasks
        validationResults, err := o.validationAgent.ValidateCompletedTasks(ctx, response.CompleteTasks, o.workingMemory)
        if err != nil {
            return nil, err
        }
        
        // Update working memory
        o.updateWorkingMemory(response, actionResults, validationResults)
        
        // Check if replanning needed
        if o.shouldReplan(actionResults, validationResults) {
            plan, err = o.planningAgent.UpdatePlan(ctx, o.workingMemory.GetPlan(), o.buildReplanContext())
            if err != nil {
                return nil, err
            }
            o.workingMemory.SetExecutionPlan(plan)
        }
    }
    
    // 3. Generate final summary
    return o.generateSummary()
}

func (o *Orchestrator) buildExecutionContext() string {
    // Build lightweight prompt context
    taskSummary := o.workingMemory.GetTaskStatusSummary()
    return fmt.Sprintf(`
CURRENT GOAL: %s

TASK PROGRESS:
%s

RECENT ACTIVITY:
%s
`, 
        o.workingMemory.GetGoal(),
        taskSummary.ToPromptFormat(),
        o.workingMemory.GetRecentHistory(5),
    )
}
```

## Phase 4: Integration with Existing AZD Agent System

### 4.1 Agent Factory Integration
```go
// File: internal/agent/agent_factory.go (modifications)

func (f *AgentFactory) CreateEnhancedAgent(ctx context.Context, opts ...AgentCreateOption) (Agent, error) {
    // Create orchestrator with all the langchain components
    orchestrator := NewOrchestrator(
        planningAgent,
        executionAgent, 
        validationAgent,
        workingMemory,
        config,
    )
    
    return &EnhancedAzdAgent{
        orchestrator: orchestrator,
        // ... existing agent base functionality
    }, nil
}
```

### 4.2 Prompt Templates
```go
// File: internal/agent/prompts/execution_v2.txt

You are an Azure Developer CLI (AZD) execution agent operating in an enhanced ReAct loop.

CURRENT GOAL: {{.goal}}

TASK PROGRESS:
{{.taskStatus}}

AVAILABLE TOOLS:
{{.toolDescriptions}}

RECENT HISTORY:
{{.recentHistory}}

YOUR ROLE:
1. Analyze the current state and determine what needs to be done next
2. Mark any tasks as complete if you have sufficient evidence
3. Execute actions using available tools to progress toward the goal

RESPOND WITH VALID JSON:
{
  "thought": "Your reasoning about the current state and next steps",
  "observation": "What you observe about previous work and current situation",
  "completeTasks": [
    {
      "taskId": "task_xxx",
      "evidence": "Specific evidence why this task is complete",
      "confidence": "high|medium|low"
    }
  ],
  "actions": [
    {
      "tool": "tool_name",
      "input": {"param": "value"},
      "reasoning": "Why you're executing this action"
    }
  ]
}
```

## Phase 5: Testing Strategy

### 5.1 Unit Tests
- Working memory operations
- Individual agent responses
- Task status management
- Evidence validation

### 5.2 Integration Tests  
- Full orchestration cycles
- Replanning scenarios
- Error recovery
- Long-running workflows

### 5.3 End-to-End Tests
- Real Azure deployments
- Complex multi-service scenarios
- Failure and recovery paths

---

## Implementation Priority

1. **Phase 1**: Core data structures and working memory
2. **Phase 2**: Individual agents (start with execution agent)
3. **Phase 3**: Basic orchestrator (without replanning)
4. **Phase 4**: Integration with existing agent factory
5. **Phase 5**: Add replanning and validation
6. **Testing**: Throughout all phases

## Leveraging Langchain Constructs

### ✅ Used Extensively
- `llms.Model` for all LLM interactions
- `agents.Agent` interface for individual agents
- `agents.Executor` for tool execution
- `prompts.PromptTemplate` for prompt management
- `memory.ConversationBuffer` for conversation history
- `tools.Tool` interface for all tools
- `chains` for complex reasoning flows

### 🔧 Custom Implementation
- Orchestrator (enhanced ReAct loop)
- Working memory (non-summarized state)
- Task management (status, dependencies, evidence)
- Validation workflows
- Replanning logic

This approach maximizes reuse of langchain's battle-tested components while building the sophisticated orchestration layer needed for complex AZD workflows.
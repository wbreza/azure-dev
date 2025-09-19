package types

import (
	"encoding/json"
	"time"
)

type Plan struct {
	Goal      string    `json:"goal"`
	Tasks     []*Task   `json:"tasks"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GetTaskByID returns a task by its ID, or nil if not found
func (ep *Plan) GetTaskByID(id string) *Task {
	for _, task := range ep.Tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

// IsComplete returns true if all tasks in the plan are complete
func (ep *Plan) IsComplete() bool {
	for _, task := range ep.Tasks {
		if task.Status != TaskComplete {
			return false
		}
	}
	return len(ep.Tasks) > 0 // Only complete if there are tasks and all are done
}

// GetPendingTasks returns all tasks that are not yet complete or failed
func (ep *Plan) GetPendingTasks() []*Task {
	var pending []*Task
	for _, task := range ep.Tasks {
		if !task.Status.IsTerminal() {
			pending = append(pending, task)
		}
	}
	return pending
}

// UpdateTaskStatus updates a task's status and timestamp
func (ep *Plan) UpdateTaskStatus(taskID string, status TaskStatus) bool {
	task := ep.GetTaskByID(taskID)
	if task == nil {
		return false
	}

	if !task.Status.CanTransitionTo(status) {
		return false
	}

	task.Status = status
	task.UpdatedAt = time.Now()
	ep.UpdatedAt = time.Now()

	return true
}

type ExecutePlanResult struct {
	Summary string `json:"summary"`
	Plan    *Plan  `json:"plan"`
	Message string `json:"message"` // Message to return to user for input/confirmation
}

type TaskExecutionResult struct {
	Task    *Task  `json:"task"`
	Message string `json:"message"` // Optional: when user input is needed, execution pauses
}

type Task struct {
	ID                 string        `json:"id"`
	Status             TaskStatus    `json:"status"`
	Progress           *TaskProgress `json:"progress"`
	Description        string        `json:"description"`
	ToolCalls          []*ToolCall   `json:"toolCalls"`
	Rules              []string      `json:"rules"`
	ValidationCriteria []string      `json:"validationCriteria"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

// TaskStatus represents the status of a task in the execution pipeline
type TaskStatus string

const (
	TaskPending            TaskStatus = "pending"            // A task has not been started yet
	TaskInProgress         TaskStatus = "inProgress"         // A task is in-progress
	TaskAwaitingValidation TaskStatus = "awaitingValidation" // A task is complete pending validation
	TaskValidating         TaskStatus = "validating"         // A task is currently being validated
	TaskInComplete         TaskStatus = "inComplete"         // A task ran but failed validation
	TaskValidationFailed   TaskStatus = "validationFailed"   // A task failed validation
	TaskComplete           TaskStatus = "complete"           // A task is completed
	TaskFailed             TaskStatus = "failed"             // A task failed during execution
	TaskBlocked            TaskStatus = "blocked"            // A task is blocked
)

func (ts TaskStatus) String() string {
	return string(ts)
}

// IsTerminal returns true if the task status represents a final state
func (ts TaskStatus) IsTerminal() bool {
	return ts == TaskComplete || ts == TaskFailed
}

// CanTransitionTo returns true if the task can transition from current status to target status
func (ts TaskStatus) CanTransitionTo(target TaskStatus) bool {
	switch ts {
	case TaskPending:
		return target == TaskInProgress || target == TaskBlocked || target == TaskFailed
	case TaskInProgress:
		return target == TaskAwaitingValidation || target == TaskFailed || target == TaskBlocked
	case TaskAwaitingValidation:
		return target == TaskValidating || target == TaskBlocked
	case TaskValidating:
		return target == TaskComplete || target == TaskInComplete || target == TaskValidationFailed || target == TaskBlocked
	case TaskInComplete:
		return target == TaskInProgress || target == TaskBlocked || target == TaskFailed
	case TaskValidationFailed:
		return target == TaskInProgress || target == TaskBlocked || target == TaskFailed
	case TaskBlocked:
		return target == TaskPending || target == TaskInProgress || target == TaskFailed
	case TaskComplete, TaskFailed:
		return false // Terminal states
	default:
		return false
	}
}

// The response of a task evaluation after all tool calls run for a given task
type TaskExecutionEvalResult struct {
	// Message to send to the user
	Message string `json:"message"`
	// Summary of the tool calls and results
	Summary string `json:"summary"`
	// Observations derived from the tool calls and results
	Observations []string `json:"observations"`
	// Evidence from the tool call responses to support task completion
	Evidence []string `json:"evidence"`
	// Additional tool calls
	Actions []*ToolCall `json:"actions"`
	// Additional rules / constraints that must be applied during task completion
	Rules []string `json:"rules"`
	// Additional validation criteria that will need to be evaluated to mark a task as complete.
	ValidationCriteria []string `json:"validationCriteria"`
}

type TaskProgress struct {
	// Summary of the tool calls and results
	Summary string `json:"summary"`
	// Observations derived from the tool calls and results
	Observations []string `json:"observations"`
	// Evidence from the tool call responses to support task completion
	Evidence []string `json:"evidence"`
}

type ToolCall struct {
	Tool      string            `json:"tool"`
	Input     string            `json:"input"`
	Reasoning string            `json:"reasoning"`
	Progress  *ToolCallProgress `json:"progress"`
}

// MarshalJSON implements custom JSON marshalling for ToolCall
// Excludes the Progress field from the JSON output
func (tc ToolCall) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Tool      string `json:"tool"`
		Input     string `json:"input"`
		Reasoning string `json:"reasoning"`
		// Progress field intentionally omitted
	}{
		Tool:      tc.Tool,
		Input:     tc.Input,
		Reasoning: tc.Reasoning,
	})
}

// UnmarshalJSON implements custom JSON unmarshalling for ToolCall
// Includes all fields including Progress for normal unmarshalling behavior
func (tc *ToolCall) UnmarshalJSON(data []byte) error {
	type Alias ToolCall
	return json.Unmarshal(data, (*Alias)(tc))
}

type ToolCallProgress struct {
	Output    string        `json:"output"`
	Error     string        `json:"error"`
	StartTime time.Time     `json:"startTime"`
	EndTime   time.Time     `json:"endTime"`
	Duration  time.Duration `json:"duration"`
}

type TaskValidationEvalResult struct {
	// Summary of the validation analysis
	Summary string `json:"summary"`
	// Recommended status after validation
	Status TaskStatus `json:"status"`
	// Detailed insights from the validation analysis and why it failed
	Insights []string `json:"insights"`
	// Recommendations for next steps to take to pass validation
	Recommendations []string `json:"recommendations"`
}

type TaskValidationResult struct {
	Task       *Task                     `json:"task"`
	Evaluation *TaskValidationEvalResult `json:"evaluation"`
}

// PlanEvalResult represents the raw LLM response for plan evaluation
type PlanEvalResult struct {
	// Summary of the planning analysis
	Summary string `json:"summary"`
	// The planned goal
	Goal string `json:"goal"`
	// Structured plan with tasks
	Tasks []*Task `json:"tasks"`
	// Insights about the planning approach
	Insights []string `json:"insights"`
	// Message to send to user (summary + confirmation request when review is needed)
	Message string `json:"message"`
}

// PlanningResult represents the result of planning operation
type PlanningResult struct {
	Plan    *Plan  `json:"plan"`
	Message string `json:"message"` // Message from planning agent to user
}

// SummaryResult represents the result of summarizing an object
type SummaryResult struct {
	// Summary of the analyzed object
	Summary string `json:"summary"`
}

// RoutingIntent represents the intent classification for user messages
type RoutingIntent string

const (
	RoutingIntentConversational RoutingIntent = "conversational" // No action required, just conversation
	RoutingIntentPlan           RoutingIntent = "plan"           // Needs to create or execute a plan
	RoutingIntentExecute        RoutingIntent = "execute"        // Execute an existing plan
	RoutingIntentValidate       RoutingIntent = "validate"       // Check if current plan is complete
	RoutingIntentReplan         RoutingIntent = "replan"         // Modify/update existing plan
	RoutingIntentProgress       RoutingIntent = "progress"       // Get detailed progress report
)

// RoutingResult represents the result of intent routing analysis
type RoutingResult struct {
	// Intent classification for the user message
	Intent RoutingIntent `json:"intent"`
	// Confidence score (0.0 to 1.0) for the classification
	Confidence float64 `json:"confidence"`
	// Reasoning for the intent classification
	Reasoning string `json:"reasoning"`
	// Message to reply to user when high confidence
	Message string `json:"message"`
}

// ProgressResult represents the result of analyzing plan progress
type ProgressResult struct {
	// Progress summary of completed and remaining work
	Progress string `json:"progress"`
}

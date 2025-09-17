package types

import "time"

type Plan struct {
	Goal      string
	Tasks     []*Task
	CreatedAt time.Time
	UpdatedAt time.Time
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
	Goal  string
	Tasks []TaskExecutionResult
}

type Task struct {
	ID                 string
	Status             TaskStatus
	Description        string
	ToolCalls          []*ToolCall
	Rules              []string
	ValidationCriteria []string
	UpdatedAt          time.Time
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
	// Summary of the tool calls and results
	Summary string
	// Insights derived from the tool calls and results
	Insights []string
	// Evidence from the tool call responses to support task completion
	Evidence []string
}

type TaskExecutionResult struct {
	Task       *Task
	Status     TaskStatus
	ToolCalls  []*ToolCallResult
	Evaluation *TaskExecutionEvalResult
}

type ToolCall struct {
	Tool      string
	Input     string
	Reasoning string
}

type ToolCallResult struct {
	ToolCall  *ToolCall
	Output    string
	Error     string
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

type TaskValidationEvalResult struct {
	// Summary of the validation analysis
	Summary string
	// Recommended status after validation
	Status TaskStatus
	// Detailed insights from the validation analysis and why it failed
	Insights []string
	// Recommendations for next steps to take to pass validation
	Recommendations []string
}

type TaskValidationResult struct {
	TaskExecutionResult *TaskExecutionResult
	Evaluation          *TaskValidationEvalResult
}

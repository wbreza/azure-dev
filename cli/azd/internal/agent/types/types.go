package types

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/tmc/langchaingo/memory"
)

type PlanStatus string

const (
	PlanPending    PlanStatus = "pending"
	PlanInProgress PlanStatus = "inprogress"
	PlanComplete   PlanStatus = "complete"
)

type Plan struct {
	Goal      string     `json:"goal"`
	Status    PlanStatus `json:"status"`
	Tasks     []*Task    `json:"tasks"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func (p *Plan) CanTransitionTo(newStatus PlanStatus) bool {
	// Define valid transitions
	validTransitions := map[PlanStatus][]PlanStatus{
		PlanPending: {
			PlanInProgress,
		},
		PlanInProgress: {
			PlanComplete,
		},
	}

	allowedTransitions, exists := validTransitions[p.Status]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == newStatus {
			return true
		}
	}

	return false
}

// IsComplete returns true if all tasks in the plan are complete and the overall status has been marked as complete
func (p *Plan) IsComplete() bool {
	return p.Status == PlanComplete && p.TasksComplete()
}

// TasksComplete return true when all tasks in the plan are complete
func (p *Plan) TasksComplete() bool {
	for _, task := range p.Tasks {
		if !task.IsTerminal() {
			return false
		}
	}

	return true
}

// GetCurrentTask returns the first inprogress task in the plan
func (p *Plan) GetCurrentTask() *Task {
	if p == nil {
		return nil
	}

	for _, task := range p.Tasks {
		if task.Status == TaskInProgress {
			return task
		}
	}

	return nil
}

// TaskStatus represents the status of a task in the execution pipeline
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"    // A task has not been started yet
	TaskInProgress TaskStatus = "inprogress" // A task is in-progress
	TaskComplete   TaskStatus = "complete"   // A task is completed
	TaskBlocked    TaskStatus = "blocked"    // A task is blocked
	TaskObsolete   TaskStatus = "obsolete"   // A task is no longer needed
	TaskFailed     TaskStatus = "failed"     // A task failed execution
)

func (ts TaskStatus) String() string {
	return string(ts)
}

type ReactPromptResult struct {
	// Message to send to the user (typically will pause the flow to prompt user for input or confirmation or when complete)
	Message string `json:"message"`
	// Reasoning for the plan updates
	Reasoning string `json:"reasoning"`
	// Observations made from progress on the plan
	Observation string `json:"observation"`
	// Updates make to the plan
	PlanUpdates []*PlanUpdates `json:"planUpdates"`
	// Next set of actions to take to make progress on the plan
	Actions []*Action `json:"actions"`
}

type PlanUpdates struct {
	// Goal is the final product or result of executing this plan to completion
	Goal string `json:"goal"`
	// The status of the plan
	OverallStatus PlanStatus `json:"overallStatus"`
	// Tasks to add to the plan
	AddTasks []*TaskUpdate `json:"addTasks"`
	// Tasks to modify
	ModifyTasks []*TaskUpdate `json:"modifyTasks"`
	// Tasks to complete
	CompleteTasks []*TaskUpdate `json:"completeTasks"`
}

type TaskUpdate struct {
	ID           string     `json:"id"`
	Description  string     `json:"description"`
	Status       TaskStatus `json:"status"`
	Reasoning    string     `json:"reasoning"`
	Evidence     []string   `json:"evidence"`
	Requirements []string   `json:"requirements"`
	Rules        []string   `json:"rules"`
}

type Task struct {
	ID           string     `json:"id"`
	Status       TaskStatus `json:"status"`
	Description  string     `json:"description"`
	Evidence     []string   `json:"evidence"`
	Requirements []string   `json:"requirements"`
	Rules        []string   `json:"rules"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`

	// Tool execution history for this task (omitted from JSON serialization)
	ToolHistory *memory.ChatMessageHistory `json:"-"`
}

// hashString creates a unique task identifier based on the description hash
func hashString(value string) string {
	hasher := md5.New()
	hasher.Write([]byte(value))
	hash := hex.EncodeToString(hasher.Sum(nil))[:8]
	return fmt.Sprintf("task_%s", hash)
}

func NewTask(description string) *Task {
	return &Task{
		ID:           hashString(description),
		Description:  description,
		Evidence:     []string{},
		Requirements: []string{},
		Rules:        []string{},
		Status:       TaskPending,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		ToolHistory:  memory.NewChatMessageHistory(),
	}
}

func (t *Task) IsTerminal() bool {
	if t.Status == TaskComplete || t.Status == TaskObsolete {
		return true
	}

	return false
}

// CanTransitionTo checks if this task can transition to the specified status
func (t *Task) CanTransitionTo(newStatus TaskStatus) bool {
	// Define valid transitions
	validTransitions := map[TaskStatus][]TaskStatus{
		TaskPending: {
			TaskInProgress,
			TaskBlocked,
			TaskObsolete,
		},
		TaskInProgress: {
			TaskComplete,
			TaskBlocked,
			TaskObsolete,
		},
		TaskBlocked: {
			TaskPending,
			TaskInProgress,
			TaskObsolete,
		},
		TaskFailed: {
			TaskInProgress,
			TaskObsolete,
		},
		// Complete and obsolete are terminal states - no transitions allowed
		TaskComplete: {},
		TaskObsolete: {},
	}

	allowedTransitions, exists := validTransitions[t.Status]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == newStatus {
			return true
		}
	}

	return false
}

// ToolExecution represents a single tool call and its result for a task
type ToolExecution struct {
	Tool      string    `json:"tool"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Timestamp time.Time `json:"timestamp"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

type Action struct {
	Tool      string `json:"tool"`
	Input     any    `json:"input"`
	Reasoning string `json:"reasoning"`
}

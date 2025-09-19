package types

import (
	"time"
)

type Plan struct {
	Goal      string    `json:"goal"`
	Tasks     []*Task   `json:"tasks"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IsComplete returns true if all tasks in the plan are complete
func (p *Plan) IsComplete() bool {
	for _, task := range p.Tasks {
		if task.Status != TaskComplete {
			return false
		}
	}
	return true
}

// TaskStatus represents the status of a task in the execution pipeline
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"    // A task has not been started yet
	TaskInProgress TaskStatus = "inprogress" // A task is in-progress
	TaskValidating TaskStatus = "validating" // A task is currently being validated
	TaskInComplete TaskStatus = "incomplete" // A task ran but failed validation
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
	Message string
	// Reasoning for the plan updates
	Reasoning string
	// Observations made from progress on the plan
	Observation string
	// Updates make to the plan
	PlanUpdates []*PlanUpdates
	// Next set of actions to take to make progress on the plan
	Actions []*Action
}

type PlanUpdates struct {
	// Goal is the final product or result of executing this plan to completion
	Goal string
	// Tasks to add to the plan
	AddTasks []*TaskUpdate
	// Tasks to modify
	ModifyTasks []*TaskUpdate
	// Tasks to complete
	CompleteTasks []*TaskUpdate
}

type TaskUpdate struct {
	ID           string
	Description  string
	Status       TaskStatus
	Reasoning    string
	Evidence     []string
	Requirements []string
}

type Task struct {
	ID           string
	Status       TaskStatus
	Description  string
	Requirements []string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Action struct {
	Tool      string
	Input     any
	Reasoning string
}

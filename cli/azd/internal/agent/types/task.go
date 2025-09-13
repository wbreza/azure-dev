// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package types

import (
	"time"
)

// TaskStatus represents the status of a task in the execution pipeline
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskInProgress TaskStatus = "inProgress"
	TaskComplete   TaskStatus = "complete"
	TaskFailed     TaskStatus = "failed"
	TaskBlocked    TaskStatus = "blocked"
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
		return target == TaskComplete || target == TaskFailed || target == TaskBlocked
	case TaskBlocked:
		return target == TaskPending || target == TaskInProgress || target == TaskFailed
	case TaskComplete, TaskFailed:
		return false // Terminal states
	default:
		return false
	}
}

// Task represents a single task in the execution plan
type Task struct {
	ID                 string            `json:"id"`
	Description        string            `json:"description"`
	Status             TaskStatus        `json:"status"`
	Rules              []string          `json:"rules,omitempty"`
	ToolCalls          []PlannedToolCall `json:"toolCalls,omitempty"`
	ValidationCriteria string            `json:"validationCriteria"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
}

// PlannedToolCall represents a tool invocation planned for a task
type PlannedToolCall struct {
	ToolName  string `json:"toolName"`
	Input     any    `json:"input"`
	Reasoning string `json:"reasoning"`
}

// ExecutionPlan represents the complete plan for achieving a goal
type ExecutionPlan struct {
	ID        string    `json:"id"`
	Goal      string    `json:"goal"`
	Tasks     []*Task   `json:"tasks"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// GetTaskByID returns a task by its ID, or nil if not found
func (ep *ExecutionPlan) GetTaskByID(id string) *Task {
	for _, task := range ep.Tasks {
		if task.ID == id {
			return task
		}
	}
	return nil
}

// IsComplete returns true if all tasks in the plan are complete
func (ep *ExecutionPlan) IsComplete() bool {
	for _, task := range ep.Tasks {
		if task.Status != TaskComplete {
			return false
		}
	}
	return len(ep.Tasks) > 0 // Only complete if there are tasks and all are done
}

// GetPendingTasks returns all tasks that are not yet complete or failed
func (ep *ExecutionPlan) GetPendingTasks() []*Task {
	var pending []*Task
	for _, task := range ep.Tasks {
		if !task.Status.IsTerminal() {
			pending = append(pending, task)
		}
	}
	return pending
}

// UpdateTaskStatus updates a task's status and timestamp
func (ep *ExecutionPlan) UpdateTaskStatus(taskID string, status TaskStatus) bool {
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

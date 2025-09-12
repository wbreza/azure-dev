// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/azure/azure-dev/cli/azd/internal/agent/types"
)

// WorkingMemory manages the persistent, non-summarized state of agent execution
type WorkingMemory struct {
	mu            sync.RWMutex
	executionPlan *types.ExecutionPlan
	taskStatus    map[string]TaskStatusSummary
	evidence      map[string][]Evidence
	history       []ExecutionEvent
}

// TaskStatusSummary provides a lightweight view of task status for prompts
type TaskStatusSummary struct {
	ID            string           `json:"id"`
	Status        types.TaskStatus `json:"status"`
	Brief         string           `json:"brief"`
	BlockedBy     string           `json:"blockedBy,omitempty"`
	EvidenceBrief string           `json:"evidenceBrief,omitempty"`
}

// Evidence represents proof that a task has been completed
type Evidence struct {
	TaskID      string    `json:"taskId"`
	Description string    `json:"description"`
	Details     string    `json:"details"`
	Confidence  string    `json:"confidence"`
	Timestamp   time.Time `json:"timestamp"`
}

// ExecutionEvent represents a significant event during execution
type ExecutionEvent struct {
	Type      string    `json:"type"` // actionExecuted, taskCompleted, planUpdated
	TaskID    string    `json:"taskId,omitempty"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// NewWorkingMemory creates a new working memory instance
func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{
		taskStatus: make(map[string]TaskStatusSummary),
		evidence:   make(map[string][]Evidence),
		history:    make([]ExecutionEvent, 0),
	}
}

// SetExecutionPlan sets the execution plan and updates task status summaries
func (wm *WorkingMemory) SetExecutionPlan(plan *types.ExecutionPlan) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.executionPlan = plan
	wm.updateTaskStatusSummaries()

	wm.addEvent(ExecutionEvent{
		Type:      "planUpdated",
		Details:   fmt.Sprintf("Plan updated with %d tasks", len(plan.Tasks)),
		Timestamp: time.Now(),
	})
}

// GetExecutionPlan returns a copy of the current execution plan
func (wm *WorkingMemory) GetExecutionPlan() *types.ExecutionPlan {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if wm.executionPlan == nil {
		return nil
	}

	// Return a copy to prevent external modifications
	planJSON, _ := json.Marshal(wm.executionPlan)
	var planCopy types.ExecutionPlan
	json.Unmarshal(planJSON, &planCopy)
	return &planCopy
}

// SetGoal sets the goal for the working memory
func (wm *WorkingMemory) SetGoal(goal string) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.executionPlan == nil {
		wm.executionPlan = &types.ExecutionPlan{}
	}
	wm.executionPlan.Goal = goal
}

// GetGoal returns the current goal
func (wm *WorkingMemory) GetGoal() string {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if wm.executionPlan == nil {
		return ""
	}
	return wm.executionPlan.Goal
}

// IsComplete returns true if all tasks in the plan are complete
func (wm *WorkingMemory) IsComplete() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if wm.executionPlan == nil {
		return false
	}
	return wm.executionPlan.IsComplete()
}

// UpdateTaskStatus updates the status of a task with evidence
func (wm *WorkingMemory) UpdateTaskStatus(taskID string, status string, evidence string) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.executionPlan == nil {
		return fmt.Errorf("no execution plan set")
	}

	// Find and update the task
	for _, task := range wm.executionPlan.Tasks {
		if task.ID == taskID {
			// Convert string status to TaskStatus
			switch status {
			case "completed":
				task.Status = types.TaskComplete
			case "in_progress":
				task.Status = types.TaskInProgress
			case "blocked":
				task.Status = types.TaskBlocked
			case "failed":
				task.Status = types.TaskFailed
			default:
				task.Status = types.TaskPending
			}
			task.UpdatedAt = time.Now()

			// Add evidence if provided
			if evidence != "" {
				wm.addEvidenceInternal(Evidence{
					TaskID:      taskID,
					Description: "Status update evidence",
					Details:     evidence,
					Confidence:  "medium",
					Timestamp:   time.Now(),
				})
			}

			// Update internal summaries
			wm.updateTaskStatusSummaries()

			// Record the status change
			wm.addEvent(ExecutionEvent{
				Type:      "task_status_updated",
				TaskID:    taskID,
				Details:   fmt.Sprintf("Status changed to %s: %s", status, evidence),
				Timestamp: time.Now(),
			})

			return nil
		}
	}

	return fmt.Errorf("task %s not found", taskID)
}

// AddEvidence adds evidence for a completed task
func (wm *WorkingMemory) AddEvidence(evidence Evidence) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wm.addEvidenceInternal(evidence)
}

// addEvidenceInternal adds evidence without acquiring locks (for internal use)
func (wm *WorkingMemory) addEvidenceInternal(evidence Evidence) {
	if wm.evidence[evidence.TaskID] == nil {
		wm.evidence[evidence.TaskID] = make([]Evidence, 0)
	}

	wm.evidence[evidence.TaskID] = append(wm.evidence[evidence.TaskID], evidence)

	// Update task status summary with evidence
	if summary, exists := wm.taskStatus[evidence.TaskID]; exists {
		summary.EvidenceBrief = evidence.Description
		wm.taskStatus[evidence.TaskID] = summary
	}

	wm.addEvent(ExecutionEvent{
		Type:      "evidenceAdded",
		TaskID:    evidence.TaskID,
		Details:   fmt.Sprintf("Evidence added: %s", evidence.Description),
		Timestamp: time.Now(),
	})
}

// GetEvidence returns all evidence for a task
func (wm *WorkingMemory) GetEvidence(taskID string) []Evidence {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	evidence := wm.evidence[taskID]
	if evidence == nil {
		return []Evidence{}
	}

	// Return a copy
	result := make([]Evidence, len(evidence))
	copy(result, evidence)
	return result
}

// GetTaskStatusSummary returns the lightweight task status for prompts
func (wm *WorkingMemory) GetTaskStatusSummary() TaskStatusDisplay {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	return TaskStatusDisplay{
		CompletedTasks: wm.getCompletedTasksSummary(),
		PendingTasks:   wm.getPendingTasksSummary(),
		ActiveTask:     wm.getActiveTask(),
	}
}

// GetRecentHistory returns the most recent execution events
func (wm *WorkingMemory) GetRecentHistory(count int) []ExecutionEvent {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	if len(wm.history) == 0 {
		return []ExecutionEvent{}
	}

	start := len(wm.history) - count
	if start < 0 {
		start = 0
	}

	result := make([]ExecutionEvent, len(wm.history[start:]))
	copy(result, wm.history[start:])
	return result
}

// TaskStatusDisplay represents the lightweight task status for prompts
type TaskStatusDisplay struct {
	CompletedTasks []TaskStatusSummary `json:"completedTasks"`
	PendingTasks   []TaskStatusSummary `json:"pendingTasks"`
	ActiveTask     string              `json:"activeTask"`
}

// ToPromptFormat converts the task status to a human-readable format for prompts
func (tsd TaskStatusDisplay) ToPromptFormat() string {
	var sb strings.Builder

	// Completed tasks
	for _, task := range tsd.CompletedTasks {
		sb.WriteString(fmt.Sprintf("✓ %s: %s\n", task.ID, task.Brief))
	}

	// Active task
	if tsd.ActiveTask != "" {
		sb.WriteString(fmt.Sprintf("→ %s: (ACTIVE)\n", tsd.ActiveTask))
	}

	// Pending tasks
	for _, task := range tsd.PendingTasks {
		if task.ID == tsd.ActiveTask {
			continue // Already shown as active
		}

		symbol := "⏳"
		if task.Status == types.TaskBlocked {
			symbol = "🚫"
		}

		suffix := ""
		if task.BlockedBy != "" {
			suffix = fmt.Sprintf(" (blocked by %s)", task.BlockedBy)
		}

		sb.WriteString(fmt.Sprintf("%s %s: %s%s\n", symbol, task.ID, task.Brief, suffix))
	}

	return sb.String()
}

// Private helper methods

func (wm *WorkingMemory) updateTaskStatusSummaries() {
	if wm.executionPlan == nil {
		return
	}

	wm.taskStatus = make(map[string]TaskStatusSummary)

	for _, task := range wm.executionPlan.Tasks {
		blockedBy := ""
		if task.Status == types.TaskBlocked {
			// Find which dependency is blocking
			for _, depID := range task.Dependencies {
				depTask := wm.executionPlan.GetTaskByID(depID)
				if depTask != nil && depTask.Status != types.TaskComplete {
					blockedBy = depID
					break
				}
			}
		}

		evidenceBrief := ""
		if evidence := wm.evidence[task.ID]; len(evidence) > 0 {
			evidenceBrief = evidence[len(evidence)-1].Description
		}

		wm.taskStatus[task.ID] = TaskStatusSummary{
			ID:            task.ID,
			Status:        task.Status,
			Brief:         task.Description,
			BlockedBy:     blockedBy,
			EvidenceBrief: evidenceBrief,
		}
	}
}

func (wm *WorkingMemory) getCompletedTasksSummary() []TaskStatusSummary {
	var completed []TaskStatusSummary
	for _, summary := range wm.taskStatus {
		if summary.Status == types.TaskComplete {
			completed = append(completed, summary)
		}
	}
	return completed
}

func (wm *WorkingMemory) getPendingTasksSummary() []TaskStatusSummary {
	var pending []TaskStatusSummary
	for _, summary := range wm.taskStatus {
		if summary.Status == types.TaskPending || summary.Status == types.TaskBlocked {
			pending = append(pending, summary)
		}
	}
	return pending
}

func (wm *WorkingMemory) getActiveTask() string {
	for _, summary := range wm.taskStatus {
		if summary.Status == types.TaskInProgress {
			return summary.ID
		}
	}
	return ""
}

func (wm *WorkingMemory) addEvent(event ExecutionEvent) {
	wm.history = append(wm.history, event)

	// Keep history bounded (last 100 events)
	if len(wm.history) > 100 {
		wm.history = wm.history[len(wm.history)-100:]
	}
}

// AddEvent adds an execution event to the history (public interface)
func (wm *WorkingMemory) AddEvent(event ExecutionEvent) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	event.Timestamp = time.Now()
	wm.addEvent(event)
}

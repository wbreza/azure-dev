// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package types

import (
	"time"
)

// AgentResponse represents the structured response from an execution agent
type AgentResponse struct {
	Thought       string          `json:"thought"`
	Observation   string          `json:"observation"`
	CompleteTasks []CompletedTask `json:"completeTasks,omitempty"`
	Actions       []ActionRequest `json:"actions,omitempty"`
}

// PlanningResult represents the result of planning - either tasks or a direct message
type PlanningResult struct {
	ResponseType string         `json:"responseType"` // "tasks" or "message"
	Goal         string         `json:"goal"`
	Message      string         `json:"message,omitempty"` // For responseType: "message"
	Plan         *ExecutionPlan `json:"plan,omitempty"`    // For responseType: "tasks"
}

// CompletedTask represents a task that the agent believes is complete
type CompletedTask struct {
	TaskID     string `json:"taskId"`
	Evidence   string `json:"evidence"`
	Confidence string `json:"confidence"` // high|medium|low
}

// ActionRequest represents a tool action the agent wants to execute
type ActionRequest struct {
	Tool      string         `json:"tool"`
	Input     map[string]any `json:"input"`
	Reasoning string         `json:"reasoning"`
}

// ActionResult represents the result of executing an action
type ActionResult struct {
	Tool      string    `json:"tool"`
	Input     string    `json:"input"`
	Output    string    `json:"output"`
	Error     string    `json:"error,omitempty"`
	Timestamp time.Time `json:"timestamp"`
	Duration  string    `json:"duration"`
}

// ValidationResult represents the result of validating execution progress
type ValidationResult struct {
	ValidationResult   string             `json:"validationResult"`
	Reasoning          string             `json:"reasoning"`
	ProgressAssessment ProgressAssessment `json:"progressAssessment"`
	Recommendations    Recommendations    `json:"recommendations"`
	Insights           []string           `json:"insights"`
}

// ProgressAssessment represents the assessment of progress made
type ProgressAssessment struct {
	OverallProgress string       `json:"overallProgress"`
	TaskUpdates     []TaskUpdate `json:"taskUpdates"`
}

// TaskUpdate represents an update to a task's status
type TaskUpdate struct {
	TaskID     string  `json:"taskId"`
	NewStatus  string  `json:"newStatus"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
}

// Recommendations represents recommendations for next actions
type Recommendations struct {
	NextAction      string   `json:"nextAction"`
	Priority        string   `json:"priority"`
	SpecificActions []string `json:"specificActions"`
}

// ExecutionResult represents the final result of the enhanced ReAct execution
type ExecutionResult struct {
	Status           string        `json:"status"`
	Reason           string        `json:"reason"`
	Goal             string        `json:"goal"`
	TasksSummary     interface{}   `json:"tasksSummary"` // TaskStatusDisplay from memory package
	TotalIterations  int           `json:"totalIterations"`
	ExecutionHistory []interface{} `json:"executionHistory"` // ExecutionEvent from memory package
}

// ReplanEvent represents a replanning decision and its context
type ReplanEvent struct {
	Trigger   string    `json:"trigger"` // what caused replanning
	Context   string    `json:"context"` // detailed context
	Changes   []string  `json:"changes"` // what changed in the plan
	Timestamp time.Time `json:"timestamp"`
}

// ExecutionSummary represents the final summary of goal execution
type ExecutionSummary struct {
	Goal              string             `json:"goal"`
	Status            string             `json:"status"`
	TotalTasks        int                `json:"totalTasks"`
	CompletedTasks    int                `json:"completedTasks"`
	FailedTasks       int                `json:"failedTasks"`
	TotalIterations   int                `json:"totalIterations"`
	ReplanEvents      []ReplanEvent      `json:"replanEvents,omitempty"`
	ActionResults     []ActionResult     `json:"actionResults"`
	ValidationResults []ValidationResult `json:"validationResults"`
	StartTime         time.Time          `json:"startTime"`
	EndTime           time.Time          `json:"endTime"`
	Duration          string             `json:"duration"`
	Summary           string             `json:"summary"`
}

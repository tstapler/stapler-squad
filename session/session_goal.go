package session

import (
	"encoding/json"
	"fmt"
	"time"
)

// Goal and task status constants.
const (
	GoalStatusIdle    = "idle"
	GoalStatusWorking = "working"
	GoalStatusBlocked = "blocked"
	GoalStatusDone    = "done"

	TaskStatusPending    = "pending"
	TaskStatusInProgress = "in_progress"
	TaskStatusDone       = "done"
	TaskStatusBlocked    = "blocked"

	maxGoalTasks = 50
	maxTaskDepth = 3
)

// TaskNode represents a single task in the goal's task tree.
type TaskNode struct {
	ID       string     `json:"id"`
	Title    string     `json:"title"`
	Status   string     `json:"status"`
	Children []TaskNode `json:"children,omitempty"`
}

// SessionGoalData holds the goal state for a session, including the task tree.
type SessionGoalData struct {
	UUID        string     `json:"uuid"`
	SessionUUID string     `json:"session_uuid"`
	Goal        string     `json:"goal"`
	Status      string     `json:"status"`
	Tasks       []TaskNode `json:"tasks,omitempty"`
	SetBy       string     `json:"set_by,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TasksTotal returns the total count of all tasks (including nested children) in the goal.
func (g *SessionGoalData) TasksTotal() int { return countTasksTotal(g.Tasks) }

// TasksDone returns the count of all tasks with status "done" (including nested children).
func (g *SessionGoalData) TasksDone() int { return countTasksDone(g.Tasks) }

func countTasksTotal(nodes []TaskNode) int {
	n := len(nodes)
	for _, t := range nodes {
		n += countTasksTotal(t.Children)
	}
	return n
}

func countTasksDone(nodes []TaskNode) int {
	n := 0
	for _, t := range nodes {
		if t.Status == TaskStatusDone {
			n++
		}
		n += countTasksDone(t.Children)
	}
	return n
}

// ValidateTaskDepth validates that the task tree does not exceed maxTaskDepth (3)
// or maxGoalTasks (50) total nodes, and that all task statuses are valid enum values.
func ValidateTaskDepth(tasks []TaskNode, depth int) error {
	if depth > maxTaskDepth {
		return fmt.Errorf("task depth exceeds maximum of %d", maxTaskDepth)
	}
	for _, t := range tasks {
		if !isValidTaskStatus(t.Status) {
			return fmt.Errorf("invalid task status %q: must be one of pending, in_progress, done, blocked", t.Status)
		}
		if err := ValidateTaskDepth(t.Children, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// validateTaskCount checks total task count does not exceed maxGoalTasks.
func validateTaskCount(tasks []TaskNode) error {
	if countTasksTotal(tasks) > maxGoalTasks {
		return fmt.Errorf("task count exceeds maximum of %d", maxGoalTasks)
	}
	return nil
}

// validateTasks runs all validation checks on a task tree.
func validateTasks(tasks []TaskNode) error {
	if err := validateTaskCount(tasks); err != nil {
		return err
	}
	return ValidateTaskDepth(tasks, 1)
}

func isValidTaskStatus(s string) bool {
	switch s {
	case TaskStatusPending, TaskStatusInProgress, TaskStatusDone, TaskStatusBlocked:
		return true
	}
	return false
}

// EncodeTasks serializes a task tree to a JSON string.
func EncodeTasks(tasks []TaskNode) (string, error) {
	if len(tasks) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(tasks)
	if err != nil {
		return "", fmt.Errorf("failed to encode tasks: %w", err)
	}
	return string(b), nil
}

// DecodeTasks deserializes a JSON string to a task tree.
func DecodeTasks(s string) ([]TaskNode, error) {
	if s == "" || s == "[]" {
		return []TaskNode{}, nil
	}
	var tasks []TaskNode
	if err := json.Unmarshal([]byte(s), &tasks); err != nil {
		return nil, fmt.Errorf("failed to decode tasks: %w", err)
	}
	return tasks, nil
}

// findAndUpdateTask walks the task tree and updates the status of the task with the given ID.
// Returns true if the task was found and updated.
func findAndUpdateTask(tasks []TaskNode, taskID, newStatus string) ([]TaskNode, bool) {
	for i, t := range tasks {
		if t.ID == taskID {
			tasks[i].Status = newStatus
			return tasks, true
		}
		if len(t.Children) > 0 {
			children, found := findAndUpdateTask(t.Children, taskID, newStatus)
			if found {
				tasks[i].Children = children
				return tasks, true
			}
		}
	}
	return tasks, false
}

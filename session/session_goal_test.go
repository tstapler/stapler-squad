package session

import (
	"testing"
)

// ─── U-GO-08: TestValidateTaskDepth_acceptsValidTree ─────────────────────────

func TestValidateTaskDepth_acceptsValidTree(t *testing.T) {
	tasks := []TaskNode{
		{ID: "1", Title: "Task 1", Status: TaskStatusPending, Children: []TaskNode{
			{ID: "1.1", Title: "Sub 1.1", Status: TaskStatusInProgress},
		}},
		{ID: "2", Title: "Task 2", Status: TaskStatusDone},
	}
	if err := ValidateTaskDepth(tasks, 1); err != nil {
		t.Errorf("ValidateTaskDepth: unexpected error for valid tree: %v", err)
	}
}

// ─── U-GO-09: TestValidateTaskDepth_rejectsDepthExceeded ──────────────────────

func TestValidateTaskDepth_rejectsDepthExceeded(t *testing.T) {
	tasks := []TaskNode{
		{ID: "1", Title: "L1", Status: TaskStatusPending, Children: []TaskNode{
			{ID: "2", Title: "L2", Status: TaskStatusPending, Children: []TaskNode{
				{ID: "3", Title: "L3", Status: TaskStatusPending, Children: []TaskNode{
					{ID: "4", Title: "L4", Status: TaskStatusPending}, // depth 4 > maxTaskDepth (3)
				}},
			}},
		}},
	}
	if err := ValidateTaskDepth(tasks, 1); err == nil {
		t.Error("ValidateTaskDepth: expected error for depth 4, got nil")
	}
}

// ─── U-GO-10: TestValidateTaskDepth_rejectsCountExceeded ──────────────────────

func TestValidateTaskDepth_rejectsCountExceeded(t *testing.T) {
	tasks := make([]TaskNode, 51)
	for i := range tasks {
		tasks[i] = TaskNode{ID: "t", Title: "task", Status: TaskStatusPending}
	}
	if err := validateTasks(tasks); err == nil {
		t.Error("validateTasks: expected error for 51 tasks, got nil")
	}
}

// ─── U-GO-11: TestValidateTaskDepth_rejectsInvalidTaskStatus ──────────────────

func TestValidateTaskDepth_rejectsInvalidTaskStatus(t *testing.T) {
	tasks := []TaskNode{
		{ID: "1", Title: "Task 1", Status: "invalid_status"},
	}
	if err := ValidateTaskDepth(tasks, 1); err == nil {
		t.Error("ValidateTaskDepth: expected error for invalid status, got nil")
	}
}

// ─── U-GO-12: TestEncodeDecodeTasks_roundTrip ──────────────────────────────────

func TestEncodeDecodeTasks_roundTrip(t *testing.T) {
	tasks := []TaskNode{
		{ID: "a", Title: "Alpha", Status: TaskStatusPending, Children: []TaskNode{
			{ID: "a1", Title: "Alpha Child", Status: TaskStatusInProgress},
		}},
		{ID: "b", Title: "Beta", Status: TaskStatusDone},
	}
	encoded, err := EncodeTasks(tasks)
	if err != nil {
		t.Fatalf("EncodeTasks: %v", err)
	}
	decoded, err := DecodeTasks(encoded)
	if err != nil {
		t.Fatalf("DecodeTasks: %v", err)
	}
	if len(decoded) != 2 {
		t.Errorf("decoded task count = %d, want 2", len(decoded))
	}
	if decoded[0].ID != "a" || decoded[0].Title != "Alpha" {
		t.Errorf("decoded[0] = %+v, want {ID:a, Title:Alpha}", decoded[0])
	}
	if len(decoded[0].Children) != 1 || decoded[0].Children[0].ID != "a1" {
		t.Errorf("decoded[0].Children = %+v, want [{ID:a1}]", decoded[0].Children)
	}
}

// ─── U-GO-13: TestEncodeDecodeTasks_emptySlice ────────────────────────────────

func TestEncodeDecodeTasks_emptySlice(t *testing.T) {
	encoded, err := EncodeTasks([]TaskNode{})
	if err != nil {
		t.Fatalf("EncodeTasks: %v", err)
	}
	if encoded != "[]" {
		t.Errorf("EncodeTasks(empty) = %q, want %q", encoded, "[]")
	}
	decoded, err := DecodeTasks(encoded)
	if err != nil {
		t.Fatalf("DecodeTasks: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("DecodeTasks([]) = %v, want empty slice", decoded)
	}
}

// ─── U-GO-14: TestSessionGoalData_TasksTotal_countsRecursively ────────────────

func TestSessionGoalData_TasksTotal_countsRecursively(t *testing.T) {
	g := &SessionGoalData{
		Tasks: []TaskNode{
			{ID: "1", Title: "T1", Status: TaskStatusPending, Children: []TaskNode{
				{ID: "1.1", Title: "T1.1", Status: TaskStatusDone},
				{ID: "1.2", Title: "T1.2", Status: TaskStatusInProgress},
			}},
			{ID: "2", Title: "T2", Status: TaskStatusBlocked},
		},
	}
	total := g.TasksTotal()
	// 1 + 2 (children of 1) + 1 = 4
	if total != 4 {
		t.Errorf("TasksTotal() = %d, want 4", total)
	}
}

// ─── U-GO-15: TestSessionGoalData_TasksDone_countsOnlyDone ────────────────────

func TestSessionGoalData_TasksDone_countsOnlyDone(t *testing.T) {
	g := &SessionGoalData{
		Tasks: []TaskNode{
			{ID: "1", Title: "T1", Status: TaskStatusDone, Children: []TaskNode{
				{ID: "1.1", Title: "T1.1", Status: TaskStatusDone},
				{ID: "1.2", Title: "T1.2", Status: TaskStatusPending},
			}},
			{ID: "2", Title: "T2", Status: TaskStatusBlocked},
		},
	}
	done := g.TasksDone()
	// T1 (done) + T1.1 (done) = 2
	if done != 2 {
		t.Errorf("TasksDone() = %d, want 2", done)
	}
}

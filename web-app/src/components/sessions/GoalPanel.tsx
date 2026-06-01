"use client";

// +feature: session-goal-panel
import { useState, useMemo } from "react";
import { SessionGoalSummary } from "@/gen/session/v1/types_pb";
import {
  panelContainer,
  summary,
  summaryLabel,
  statusChipBase,
  statusChipVariants,
  body,
  goalText,
  goalTextClamped,
  expandButton,
  taskFraction,
  taskList,
  taskItem,
  taskTitle,
  taskChildren,
  taskStatusChipBase,
  taskStatusVariants,
} from "./GoalPanel.css";

/** TypeScript mirror of Go's session.TaskNode. */
export interface TaskNode {
  id: string;
  title: string;
  status: string;
  children?: TaskNode[];
}

/**
 * Parses a JSON string into a TaskNode array. Returns [] on any parse failure.
 * Exported for direct unit testing.
 */
export function parseTasksJson(json: string | null | undefined): TaskNode[] {
  if (!json) return [];
  try {
    const parsed = JSON.parse(json);
    if (!Array.isArray(parsed)) return [];
    return parsed as TaskNode[];
  } catch {
    return [];
  }
}

/** Goal status label map */
const STATUS_LABELS: Record<string, string> = {
  idle: "Idle",
  working: "Working",
  blocked: "Blocked",
  done: "Done",
};

/** Task status label map */
const TASK_STATUS_LABELS: Record<string, string> = {
  pending: "Pending",
  in_progress: "In Progress",
  done: "Done",
  blocked: "Blocked",
};

const MAX_GOAL_DISPLAY_LENGTH = 120;

/** Single task node rendered with status badge and optional children. */
function TaskTreeNode({ node, depth }: { node: TaskNode; depth: number }) {
  if (depth > 3) return null; // guard against malformed data
  const statusClass =
    taskStatusVariants[node.status as keyof typeof taskStatusVariants] ??
    taskStatusVariants["pending"];
  const statusLabel = TASK_STATUS_LABELS[node.status] ?? node.status;

  return (
    <li className={taskItem}>
      <span className={`${taskStatusChipBase} ${statusClass}`} aria-label={`Status: ${statusLabel}`}>
        {statusLabel}
      </span>
      <span className={taskTitle}>{node.title}</span>
      {node.children && node.children.length > 0 && (
        <ul className={taskChildren} role="list">
          {node.children.map((child) => (
            <TaskTreeNode key={child.id} node={child} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  );
}

export interface GoalPanelProps {
  goal: SessionGoalSummary | null | undefined;
}

/**
 * GoalPanel renders the session goal and task tree in the session detail view.
 * Returns null when no goal is set.
 */
export function GoalPanel({ goal }: GoalPanelProps) {
  const [expanded, setExpanded] = useState(false);

  const tasks = useMemo(() => parseTasksJson(goal?.tasksJson), [goal?.tasksJson]);

  if (!goal || !goal.goalText) return null;

  const isLong = goal.goalText.length > MAX_GOAL_DISPLAY_LENGTH;
  const displayText =
    !isLong || expanded
      ? goal.goalText
      : goal.goalText.slice(0, MAX_GOAL_DISPLAY_LENGTH) + "…";

  const chipClass =
    statusChipVariants[goal.status as keyof typeof statusChipVariants] ??
    statusChipVariants["idle"];
  const statusLabel = STATUS_LABELS[goal.status] ?? goal.status;

  const hasFraction = (goal.tasksTotal ?? 0) > 0;

  return (
    <details className={panelContainer}>
      <summary className={summary}>
        <span className={`${statusChipBase} ${chipClass}`} aria-label={`Goal status: ${statusLabel}`}>
          {statusLabel}
        </span>
        <span className={summaryLabel}>Goal &amp; Tasks</span>
      </summary>

      <div className={body}>
        <p className={`${goalText} ${!expanded && isLong ? goalTextClamped : ""}`}>
          {displayText}
          {isLong && (
            <button
              className={expandButton}
              onClick={() => setExpanded((v) => !v)}
              aria-label={expanded ? "Show less goal text" : "Show full goal text"}
            >
              {expanded ? "less" : "more"}
            </button>
          )}
        </p>

        {tasks.length > 0 ? (
          <ul className={taskList} role="list" aria-label="Task list">
            {tasks.map((t) => (
              <TaskTreeNode key={t.id} node={t} depth={1} />
            ))}
          </ul>
        ) : hasFraction ? (
          <p className={taskFraction} aria-label="Task progress">
            {goal.tasksDone}/{goal.tasksTotal} done
          </p>
        ) : null}
      </div>
    </details>
  );
}

import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { GoalPanel, parseTasksJson } from "./GoalPanel";
import type { SessionGoalSummary } from "@/gen/session/v1/types_pb";

// Minimal factory for test SessionGoalSummary objects.
function makeGoal(overrides?: Partial<SessionGoalSummary>): SessionGoalSummary {
  return {
    goalText: "Implement the feature",
    status: "working",
    tasksTotal: 0,
    tasksDone: 0,
    tasksJson: "",
    ...overrides,
  } as unknown as SessionGoalSummary;
}

describe("GoalPanel", () => {
  // U-TS-01
  it("renders nothing when goal is null", () => {
    const { container } = render(<GoalPanel goal={null} />);
    expect(container.firstChild).toBeNull();
  });

  // U-TS-02
  it("renders nothing when goal is undefined", () => {
    const { container } = render(<GoalPanel goal={undefined} />);
    expect(container.firstChild).toBeNull();
  });

  // U-TS-01 (goalText empty)
  it("renders nothing when goalText is empty string", () => {
    const { container } = render(<GoalPanel goal={makeGoal({ goalText: "" })} />);
    expect(container.firstChild).toBeNull();
  });

  // U-TS-03
  it("renders goal text when goal is set", () => {
    render(<GoalPanel goal={makeGoal()} />);
    expect(screen.getByText("Implement the feature")).toBeInTheDocument();
  });

  // U-TS-04
  it("truncates long goal text at 120 chars and shows expand toggle", () => {
    const longGoal = "A".repeat(130);
    render(<GoalPanel goal={makeGoal({ goalText: longGoal })} />);
    // Text should be truncated — exact display shows first 120 chars + "…"
    const truncated = "A".repeat(120) + "…";
    expect(screen.getByText(truncated)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /show full goal text/i })).toBeInTheDocument();
  });

  it("expand toggle shows full text on click", () => {
    const longGoal = "B".repeat(130);
    render(<GoalPanel goal={makeGoal({ goalText: longGoal })} />);
    const expandBtn = screen.getByRole("button", { name: /show full goal text/i });
    fireEvent.click(expandBtn);
    expect(screen.getByText(longGoal)).toBeInTheDocument();
  });

  // U-TS-05
  it("renders task fraction when tasks are set", () => {
    render(<GoalPanel goal={makeGoal({ tasksTotal: 5, tasksDone: 2 })} />);
    expect(screen.getByText(/2\/5 done/)).toBeInTheDocument();
  });

  it("does not render task fraction when tasksTotal is 0", () => {
    render(<GoalPanel goal={makeGoal({ tasksTotal: 0, tasksDone: 0, tasksJson: "" })} />);
    expect(screen.queryByText(/done/)).toBeNull();
  });

  // U-TS-06
  it.each(["idle", "working", "blocked", "done"])(
    "shows correct status chip label for status=%s",
    (status) => {
      const labelMap: Record<string, string> = {
        idle: "Idle",
        working: "Working",
        blocked: "Blocked",
        done: "Done",
      };
      render(<GoalPanel goal={makeGoal({ status })} />);
      expect(screen.getByLabelText(`Goal status: ${labelMap[status]}`)).toBeInTheDocument();
    }
  );

  // U-TS-07
  it("renders task tree from tasksJson", () => {
    const tasks = [
      { id: "t1", title: "Task One", status: "pending" },
      { id: "t2", title: "Task Two", status: "done" },
    ];
    render(<GoalPanel goal={makeGoal({ tasksJson: JSON.stringify(tasks), tasksTotal: 2, tasksDone: 1 })} />);
    expect(screen.getByText("Task One")).toBeInTheDocument();
    expect(screen.getByText("Task Two")).toBeInTheDocument();
  });

  // U-TS-08
  it("renders nested tasks with indentation", () => {
    const tasks = [
      {
        id: "parent",
        title: "Parent Task",
        status: "in_progress",
        children: [{ id: "child1", title: "Child Task", status: "pending" }],
      },
    ];
    render(<GoalPanel goal={makeGoal({ tasksJson: JSON.stringify(tasks), tasksTotal: 2, tasksDone: 0 })} />);
    expect(screen.getByText("Parent Task")).toBeInTheDocument();
    expect(screen.getByText("Child Task")).toBeInTheDocument();
  });

  // U-TS-09
  it("falls back to fraction when tasksJson is invalid JSON", () => {
    render(
      <GoalPanel goal={makeGoal({ tasksJson: "not json", tasksTotal: 3, tasksDone: 1 })} />
    );
    // Should not throw; should show fraction
    expect(screen.getByText(/1\/3 done/)).toBeInTheDocument();
  });

  // U-TS-10
  it("falls back to fraction when tasksJson is empty string", () => {
    render(
      <GoalPanel goal={makeGoal({ tasksJson: "", tasksTotal: 2, tasksDone: 0 })} />
    );
    expect(screen.getByText(/0\/2 done/)).toBeInTheDocument();
  });
});

// U-TS-20, U-TS-21
describe("parseTasksJson", () => {
  it("returns empty array on invalid JSON", () => {
    expect(parseTasksJson("not json")).toEqual([]);
  });

  it("returns empty array on null", () => {
    expect(parseTasksJson(null)).toEqual([]);
  });

  it("returns empty array on undefined", () => {
    expect(parseTasksJson(undefined)).toEqual([]);
  });

  it("returns empty array on empty string", () => {
    expect(parseTasksJson("")).toEqual([]);
  });

  it("parses valid JSON task array", () => {
    const tasks = [{ id: "a", title: "T", status: "pending" }];
    expect(parseTasksJson(JSON.stringify(tasks))).toEqual(tasks);
  });

  it("returns empty array when JSON is not an array", () => {
    expect(parseTasksJson('{"id":"a"}')).toEqual([]);
  });
});

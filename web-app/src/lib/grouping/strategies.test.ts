import { create } from "@bufbuild/protobuf";
import { SessionSchema, SessionStatus } from "@/gen/session/v1/types_pb";
import { groupSessions, GroupingStrategy } from "./strategies";

describe("groupSessions", () => {
  const mockSessions = [
    create(SessionSchema, {
      title: "session1",
      category: "Work",
      tags: ["urgent", "frontend"],
      status: SessionStatus.RUNNING,
    }),
    create(SessionSchema, {
      title: "session2",
      category: "Personal",
      tags: ["frontend"],
      status: SessionStatus.PAUSED,
    }),
    create(SessionSchema, {
      title: "session3",
      category: "Work",
      tags: [],
      status: SessionStatus.RUNNING,
    }),
  ];

  it("should group by category", () => {
    const result = groupSessions(mockSessions, GroupingStrategy.Category);
    expect(result).toHaveLength(2);

    const workGroup = result.find(g => g.groupKey === "Work");
    const personalGroup = result.find(g => g.groupKey === "Personal");

    expect(workGroup?.sessions).toHaveLength(2);
    expect(personalGroup?.sessions).toHaveLength(1);
  });

  it("should group by tag (multi-membership)", () => {
    const result = groupSessions(mockSessions, GroupingStrategy.Tag);
    // Groups: urgent, frontend, Untagged
    expect(result).toHaveLength(3);

    const urgentGroup = result.find(g => g.groupKey === "urgent");
    const frontendGroup = result.find(g => g.groupKey === "frontend");
    const untaggedGroup = result.find(g => g.groupKey === "Untagged");

    expect(urgentGroup?.sessions).toHaveLength(1);
    expect(frontendGroup?.sessions).toHaveLength(2);
    expect(untaggedGroup?.sessions).toHaveLength(1);
  });

  it("should return single group for Strategy.None", () => {
    const result = groupSessions(mockSessions, GroupingStrategy.None);
    expect(result).toHaveLength(1);
    expect(result[0].groupKey === "all");
    expect(result[0].sessions).toHaveLength(3);
  });

  it("should handle sessions with missing categories", () => {
    const sessionsWithMissing = [
      ...mockSessions,
      create(SessionSchema, { title: "session4", category: "" })
    ];
    const result = groupSessions(sessionsWithMissing, GroupingStrategy.Category);
    const uncategorizedGroup = result.find(g => g.groupKey === "Uncategorized");
    expect(uncategorizedGroup?.sessions).toHaveLength(1);
  });

  describe("GroupingStrategy.Workflow", () => {
    const wfSession1 = create(SessionSchema, {
      title: "wf-session-1",
      workflowId: "wf-uuid-1",
      workflowName: "Knowledge Maintenance",
    });
    const wfSession2 = create(SessionSchema, {
      title: "wf-session-2",
      workflowId: "wf-uuid-2",
      workflowName: "Daily Sync",
    });
    const manualSession = create(SessionSchema, {
      title: "manual-session",
      workflowId: "",
    });

    it("Workflow_strategy_groups_by_workflow_name", () => {
      const workflowIdToName = new Map([
        ["wf-uuid-1", "Knowledge Maintenance"],
        ["wf-uuid-2", "Daily Sync"],
      ]);
      const result = groupSessions(
        [wfSession1, wfSession2],
        GroupingStrategy.Workflow,
        { workflowIdToName }
      );
      expect(result).toHaveLength(2);
      const kmGroup = result.find(g => g.groupKey === "Knowledge Maintenance");
      const dsGroup = result.find(g => g.groupKey === "Daily Sync");
      expect(kmGroup?.sessions).toHaveLength(1);
      expect(dsGroup?.sessions).toHaveLength(1);
    });

    it("Workflow_strategy_puts_manual_sessions_last", () => {
      const workflowIdToName = new Map([["wf-uuid-1", "Knowledge Maintenance"]]);
      const result = groupSessions(
        [wfSession1, manualSession],
        GroupingStrategy.Workflow,
        { workflowIdToName }
      );
      expect(result).toHaveLength(2);
      // "Manual Sessions" should sort last
      expect(result[result.length - 1].groupKey).toBe("Manual Sessions");
    });

    it("falls back to workflow UUID when workflowIdToName is not provided", () => {
      const result = groupSessions(
        [wfSession1],
        GroupingStrategy.Workflow
      );
      expect(result).toHaveLength(1);
      // Falls back to UUID since no name map provided
      expect(result[0].groupKey).toBe("wf-uuid-1");
    });

    it("puts sessions without workflowId into Manual Sessions group", () => {
      const result = groupSessions(
        [manualSession],
        GroupingStrategy.Workflow
      );
      expect(result).toHaveLength(1);
      expect(result[0].groupKey).toBe("Manual Sessions");
    });
  });
});

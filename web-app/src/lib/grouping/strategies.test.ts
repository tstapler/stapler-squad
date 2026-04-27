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
});

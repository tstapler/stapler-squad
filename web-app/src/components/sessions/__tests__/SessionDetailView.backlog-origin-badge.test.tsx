/**
 * Covers the SessionDetailView header's BacklogOriginBadge: a session dispatched by
 * backlog automation (work/review/triage) shows the badge; one with no backlogEntry
 * does not.
 */
import "./sessionDetailViewBaseMocks";
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
import { installConsoleErrorSilencer } from "./sessionDetailViewTestFixtures";

jest.mock("@/lib/hooks/useWorkflows", () => require("./sessionDetailViewTestFixtures").makeUseWorkflowsFromGetter(() => []));

const session: Session = {
  id: "sess-1",
  title: "Test Session",
  status: SessionStatus.STOPPED,
  instanceType: InstanceType.MANAGED,
  sessionType: SessionType.DIRECTORY,
  path: "/tmp/test",
  branch: "main",
  program: "claude",
  workingDir: "",
  category: "",
  tags: [],
  externalMetadata: undefined,
} as unknown as Session;

function renderView(backlogEntry?: BacklogIndexEntry) {
  const actions = {} as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView
      session={session}
      allSessions={[]}
      actions={actions}
      onClose={jest.fn()}
      initialTab="info"
      backlogEntry={backlogEntry}
    />
  );
}

installConsoleErrorSilencer();

describe("SessionDetailView — backlog-origin badge", () => {
  it("shows the badge when backlogEntry is set", () => {
    renderView({ itemId: "item-1", itemTitle: "Fix the thing", itemStatus: "in_progress", sessionRole: "work" });
    expect(screen.getByTestId("backlog-origin-badge")).toBeInTheDocument();
  });

  it("does not show the badge when there is no backlogEntry", () => {
    renderView(undefined);
    expect(screen.queryByTestId("backlog-origin-badge")).not.toBeInTheDocument();
  });
});

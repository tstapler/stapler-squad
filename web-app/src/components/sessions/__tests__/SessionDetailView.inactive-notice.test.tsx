/**
 * SessionDetailView mounts SessionInactiveNotice from the server-computed
 * Session.terminalReadOnly and wires its Retry to actions.retry (the shared
 * useSessionService retrySession), surfacing a failure inline.
 */
import "./sessionDetailViewBaseMocks";
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SessionDetailView } from "../SessionDetailView";
import type { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";
import { installConsoleErrorSilencer } from "./sessionDetailViewTestFixtures";

jest.mock("@/lib/hooks/useWorkflows", () => require("./sessionDetailViewTestFixtures").mockUseWorkflowsEmpty());

const makeSession = (overrides: Partial<Session>): Session =>
  ({
    id: "sess-1",
    title: "Failed Session",
    status: SessionStatus.PERMANENTLY_FAILED,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    workflowId: "",
    hidden: false,
    terminalReadOnly: true,
    ...overrides,
  }) as unknown as Session;

function renderView(session: Session, retry: jest.Mock) {
  const actions = { retry } as unknown as ReturnType<typeof useSessionActions>;
  return render(
    <SessionDetailView session={session} allSessions={[]} actions={actions} onClose={jest.fn()} initialTab="terminal" />,
  );
}

installConsoleErrorSilencer();

describe("SessionDetailView inactive notice", () => {
  it("mounts the notice when the server reports the terminal read-only", () => {
    renderView(makeSession({}), jest.fn());
    expect(screen.getByTestId("session-inactive-notice")).toBeInTheDocument();
  });

  it("does not mount the notice when the terminal accepts input", () => {
    renderView(makeSession({ status: SessionStatus.STOPPED, terminalReadOnly: false }), jest.fn());
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });

  it("does not stack the notice over a hidden session's read-only banner", () => {
    renderView(makeSession({ hidden: true }), jest.fn());
    expect(screen.queryByTestId("session-inactive-notice")).toBeNull();
  });

  it("calls actions.retry on click and shows an inline error when it resolves false", async () => {
    const retry = jest.fn().mockResolvedValue(false);
    renderView(makeSession({}), retry);
    fireEvent.click(screen.getByTestId("session-inactive-notice-retry"));
    expect(retry).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.getByTestId("session-inactive-notice-error")).toHaveTextContent("Failed to retry session."));
  });

  it("shows no error when the retry succeeds", async () => {
    const retry = jest.fn().mockResolvedValue(true);
    renderView(makeSession({}), retry);
    fireEvent.click(screen.getByTestId("session-inactive-notice-retry"));
    await waitFor(() => expect(retry).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId("session-inactive-notice-error")).toBeNull();
  });
});

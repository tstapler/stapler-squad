import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ImportExternalSessionsPanel } from "./ImportExternalSessionsPanel";
import { InstanceType } from "@/gen/session/v1/types_pb";
import { ImportSourceKind } from "@/gen/session/v1/import_pb";

let mockSessions: unknown[] = [];

jest.mock("./ImportExternalSessionsPanel.css", () =>
  new Proxy(
    {},
    {
      get: (_target, key) => (typeof key === "string" ? key : ""),
    }
  )
);

jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) =>
    selector({ sessions: {} }),
}));

jest.mock("@/lib/store/sessionsSlice", () => ({
  selectAllSessions: () => mockSessions,
}));

function externalSession(overrides: Partial<Record<string, unknown>> = {}) {
  return {
    id: "sess-1",
    title: "My External Session",
    path: "/home/user/project",
    program: "claude",
    instanceType: InstanceType.EXTERNAL,
    externalMetadata: {
      tmuxSocket: "",
      tmuxSessionName: "tmux-1",
      originalPid: 1234,
      muxSocketPath: "/tmp/ssq-mux.sock",
      muxEnabled: true,
      sourceTerminal: "VSCode",
    },
    ...overrides,
  };
}

describe("ImportExternalSessionsPanel", () => {
  beforeEach(() => {
    mockSessions = [];
  });

  it("shows empty state when no external sessions are present", () => {
    mockSessions = [
      { id: "managed-1", instanceType: InstanceType.MANAGED, path: "/a", program: "claude" },
    ];

    render(<ImportExternalSessionsPanel />);

    expect(screen.getByText(/No external sessions detected/i)).toBeInTheDocument();
  });

  it("renders a row for each external session, filtering out managed sessions", () => {
    mockSessions = [
      externalSession(),
      { id: "managed-1", instanceType: InstanceType.MANAGED, path: "/a", program: "claude" },
    ];

    render(<ImportExternalSessionsPanel />);

    expect(screen.getByText("My External Session")).toBeInTheDocument();
    expect(screen.getByText("/home/user/project")).toBeInTheDocument();
    expect(screen.getByText("VSCode")).toBeInTheDocument();
  });

  it("derives MUX_DISCOVERED source kind when muxEnabled and socket path are present", () => {
    const onImport = jest.fn();
    mockSessions = [externalSession()];

    render(<ImportExternalSessionsPanel onImport={onImport} />);

    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    expect(onImport).toHaveBeenCalledWith([
      expect.objectContaining({
        sourceKind: ImportSourceKind.MUX_DISCOVERED,
        pid: 1234,
        tmuxSession: "tmux-1",
        socketPath: "/tmp/ssq-mux.sock",
        path: "/home/user/project",
        program: "claude",
      }),
    ]);
  });

  it("derives PLAIN_TMUX source kind when mux is not enabled", () => {
    const onImport = jest.fn();
    mockSessions = [
      externalSession({
        externalMetadata: {
          tmuxSocket: "",
          tmuxSessionName: "tmux-2",
          originalPid: 5678,
          muxSocketPath: "",
          muxEnabled: false,
          sourceTerminal: "",
        },
      }),
    ];

    render(<ImportExternalSessionsPanel onImport={onImport} />);

    fireEvent.click(screen.getByRole("button", { name: "Import" }));

    expect(onImport).toHaveBeenCalledWith([
      expect.objectContaining({ sourceKind: ImportSourceKind.PLAIN_TMUX }),
    ]);
  });

  it("supports selecting multiple rows and importing the selection in bulk", () => {
    const onImport = jest.fn();
    mockSessions = [
      externalSession({ id: "sess-1", title: "Session One" }),
      externalSession({ id: "sess-2", title: "Session Two" }),
    ];

    render(<ImportExternalSessionsPanel onImport={onImport} />);

    fireEvent.click(screen.getByLabelText("Select Session One"));
    fireEvent.click(screen.getByLabelText("Select Session Two"));

    expect(screen.getByText("2 selected")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Import selected/i }));

    expect(onImport).toHaveBeenCalledWith([
      expect.objectContaining({ tmuxSession: "tmux-1" }),
      expect.objectContaining({ tmuxSession: "tmux-1" }),
    ]);
    expect(onImport.mock.calls[0][0]).toHaveLength(2);
  });

  it("toggles select-all checkbox to select and clear all rows", () => {
    mockSessions = [
      externalSession({ id: "sess-1", title: "Session One" }),
      externalSession({ id: "sess-2", title: "Session Two" }),
    ];

    render(<ImportExternalSessionsPanel />);

    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.getByText("2 selected")).toBeInTheDocument();

    fireEvent.click(screen.getByLabelText("Select all"));
    expect(screen.queryByText("2 selected")).not.toBeInTheDocument();
  });

  it("invokes onRefresh when the refresh button is clicked", () => {
    const onRefresh = jest.fn();
    mockSessions = [externalSession()];

    render(<ImportExternalSessionsPanel onRefresh={onRefresh} />);

    fireEvent.click(screen.getByLabelText("Refresh discovered sessions"));

    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});

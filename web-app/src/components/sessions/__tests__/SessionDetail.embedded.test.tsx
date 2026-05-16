/**
 * Enforcement tests for two bugs:
 *
 * Bug 3 — PaneHeader tab sync broken:
 *   `useState(initialTab)` only reads the prop once. When PaneHeader dispatches
 *   ASSIGN_TAB and `initialTab` prop changes, `SessionDetail.activeTab` never
 *   updated and content stayed on the old tab.
 *   Fix: `useEffect(() => { setActiveTab(initialTab); }, [initialTab])`.
 *   Pre-fix failure: step 4 would find terminal panel still aria-hidden after rerender.
 *
 * Bug 4 — Duplicate chrome layers:
 *   `SessionDetail` always rendered its own header + tab strip, stacking on top
 *   of PaneHeader when used in a tiling pane. Fix: `embedded` prop suppresses them.
 *   Pre-fix failure: `embedded` prop didn't exist; header/tabs always rendered.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetail } from "../SessionDetail";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// --- Component mocks ---

jest.mock("next/dynamic", () => (loader: () => Promise<{ default: React.ComponentType }>) => {
  return function DynamicStub() {
    return <div data-testid="terminal-output" />;
  };
});

jest.mock("../DiffViewer", () => ({ DiffViewer: () => <div data-testid="diff-viewer" /> }));
jest.mock("../VcsPanel", () => ({ VcsPanel: () => <div data-testid="vcs-panel" /> }));
jest.mock("../SessionLogsTab", () => ({ SessionLogsTab: () => <div data-testid="logs-tab" /> }));
jest.mock("../FilesTab", () => ({ FilesTab: () => <div data-testid="files-tab" /> }));
jest.mock("../WorkspaceSwitchModal", () => ({ WorkspaceSwitchModal: () => null }));
jest.mock("../TagEditor", () => ({ TagEditor: () => null }));
jest.mock("../ResumeSessionModal", () => ({ ResumeSessionModal: () => null }));
jest.mock("@/components/ui/ActionBar", () => ({
  ActionBar: ({ children, className }: { children: React.ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
}));
jest.mock("@/components/ui/Modal", () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalTitle: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  ModalFooter: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));
jest.mock("@/lib/hooks/useSessionActions", () => ({
  useSessionActions: () => ({
    pause: jest.fn(),
    resume: jest.fn(),
    delete: jest.fn(),
    rename: jest.fn(),
    restart: jest.fn(),
    update: jest.fn(),
    runOneShot: jest.fn(),
  }),
}));
jest.mock("@/lib/contexts/SessionVcsContext", () => ({
  SessionVcsProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));
jest.mock("@/lib/hooks/useVcsStatus", () => ({ prefetchVcsStatus: jest.fn() }));
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543" }));
jest.mock("@/lib/constants/programs", () => ({
  getProgramDisplay: (p: string) => p,
  isKnownProgram: () => true,
  PROGRAMS: [],
}));
jest.mock("@/lib/store", () => ({ useAppSelector: jest.fn(() => []) }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: jest.fn() }));

// --- Minimal session fixture ---

const makeSession = (): Session =>
  ({
    id: "sess-1",
    title: "Test Session",
    status: SessionStatus.RUNNING,
    instanceType: InstanceType.MANAGED,
    sessionType: SessionType.DIRECTORY,
    path: "/tmp/test",
    branch: "main",
    program: "claude",
    workingDir: "",
    category: "",
    tags: [],
    externalMetadata: undefined,
  }) as unknown as Session;

// ─────────────────────────────────────────────
// Bug 4: embedded prop suppresses header + tabs
// ─────────────────────────────────────────────
describe("SessionDetail — embedded mode (Bug 4)", () => {
  it("does NOT render the title header when embedded=true", () => {
    render(
      <SessionDetail session={makeSession()} embedded onClose={jest.fn()} />
    );
    expect(screen.queryByTestId("session-header")).not.toBeInTheDocument();
  });

  it("does NOT render the tab strip when embedded=true", () => {
    render(
      <SessionDetail session={makeSession()} embedded onClose={jest.fn()} />
    );
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
  });

  it("renders the title header when embedded is not set", () => {
    render(<SessionDetail session={makeSession()} onClose={jest.fn()} />);
    expect(screen.getByTestId("session-header")).toBeInTheDocument();
  });

  it("renders the tab strip when embedded is not set", () => {
    render(<SessionDetail session={makeSession()} onClose={jest.fn()} />);
    expect(screen.getByRole("tablist")).toBeInTheDocument();
  });

  it("still renders tab content when embedded=true", () => {
    render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="terminal"
      />
    );
    // Content area must still exist even without the chrome
    expect(screen.getByRole("tabpanel", { hidden: true })).toBeInTheDocument();
  });
});

// ─────────────────────────────────────────────────────────────────────────
// Bug 3: initialTab prop changes must sync to displayed content
// ─────────────────────────────────────────────────────────────────────────
describe("SessionDetail — initialTab sync (Bug 3)", () => {
  it("starts on the given initialTab", () => {
    render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="info"
      />
    );
    // Terminal panel should be hidden when starting on "info"
    const terminalPanel = document.querySelector('[aria-labelledby="tab-terminal"]');
    expect(terminalPanel).toHaveAttribute("aria-hidden", "true");
  });

  it("switches displayed content when initialTab prop changes", () => {
    const { rerender } = render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="info"
      />
    );

    // Verify we're on info tab: terminal panel is hidden
    const terminalPanel = document.querySelector('[aria-labelledby="tab-terminal"]');
    expect(terminalPanel).toHaveAttribute("aria-hidden", "true");

    // Simulate PaneHeader dispatching ASSIGN_TAB → initialTab prop changes
    rerender(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="terminal"
      />
    );

    // Terminal panel must now be visible — this fails against pre-fix code
    // because useState(initialTab) never re-syncs from prop changes.
    expect(terminalPanel).not.toHaveAttribute("aria-hidden", "true");
  });

  it("switches back when initialTab reverts to original value", () => {
    const { rerender } = render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="terminal"
      />
    );

    const terminalPanel = document.querySelector('[aria-labelledby="tab-terminal"]');
    expect(terminalPanel).not.toHaveAttribute("aria-hidden", "true");

    rerender(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="diff"
      />
    );

    expect(terminalPanel).toHaveAttribute("aria-hidden", "true");
  });
});

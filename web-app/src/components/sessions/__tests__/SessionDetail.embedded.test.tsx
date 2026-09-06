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
 *   `SessionDetail` always rendered its own title header, stacking on top of
 *   `PaneHeader`'s title when used in a tiling pane. Fix: `embedded` prop suppresses
 *   the title header only. The tab strip is NOT suppressed: `PaneHeader` no longer
 *   renders its own tab switcher (it was a redundant, less-capable duplicate of this
 *   tab strip — see PaneHeader.tsx), so this tab strip is now the sole tab UI in
 *   both embedded and non-embedded contexts.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { SessionDetail } from "../SessionDetail";
import { SessionStatus, InstanceType, SessionType } from "@/gen/session/v1/types_pb";
import type { Session } from "@/gen/session/v1/types_pb";

// --- Component mocks ---

// SessionDetail.tsx dynamically imports SessionDetailView, which itself dynamically
// imports TerminalOutput — one blanket stub can't serve both nested call sites once we
// want the real SessionDetailView (with its correct ARIA markup) to render. Resolve the
// SessionDetailView loader synchronously via require so assertions can stay synchronous;
// stub everything else (i.e. TerminalOutput) as before.
jest.mock("next/dynamic", () => (loader: () => Promise<{ default: React.ComponentType }>) => {
  if (loader.toString().includes("SessionDetailView")) {
    return require("../SessionDetailView").SessionDetailView;
  }
  return function DynamicStub() {
    return <div data-testid="terminal-output" />;
  };
});

jest.mock("../DiffViewer", () => ({ DiffViewer: () => <div data-testid="diff-viewer" /> }));
// HandoffSummarySection (Info tab) embeds RestartWithSummaryButton, which
// calls useSessionService -> useAnalytics -- unavailable without an
// AnalyticsContextProvider wrapper, which this file's render tree doesn't
// set up (it isn't relevant to embedded-mode/initialTab behavior, this
// file's own concern).
jest.mock("../HandoffSummarySection", () => ({ HandoffSummarySection: () => null }));
jest.mock("../VcsPanel", () => ({ VcsPanel: () => <div data-testid="vcs-panel" /> }));
jest.mock("../SessionLogsTab", () => ({ SessionLogsTab: () => <div data-testid="logs-tab" /> }));
jest.mock("../FilesTab", () => ({ FilesTab: () => <div data-testid="files-tab" /> }));
jest.mock("../WorkspaceSwitchModal", () => ({ WorkspaceSwitchModal: () => null }));
jest.mock("../TagEditor", () => ({ TagEditor: () => null }));
jest.mock("../ResumeSessionModal", () => ({ ResumeSessionModal: () => null }));
jest.mock("../BrowserTab", () => ({
  BrowserTab: ({ sessionId }: { sessionId: string }) => (
    <div data-testid={`browser-tab-stub-${sessionId}`} />
  ),
  VNCStatus: { UNSPECIFIED: 0, STARTING: 1, READY: 2, NO_BROWSER: 3, UNAVAILABLE: 4 },
}));
jest.mock("../NoVNCViewer", () => ({
  __esModule: true,
  default: () => <div data-testid="novnc-viewer-stub" />,
}));
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
jest.mock("@/lib/config", () => ({ getApiBaseUrl: () => "http://localhost:8543", createAuthInterceptor: () => jest.fn() }));
jest.mock("@/lib/constants/programs", () => ({
  getProgramDisplay: (p: string) => p,
  isKnownProgram: () => true,
  PROGRAMS: [],
}));
jest.mock("@/lib/store", () => ({ useAppSelector: jest.fn(() => []) }));
jest.mock("@/lib/store/sessionsSlice", () => ({ selectAllSessions: jest.fn() }));

// useShells otherwise fires a real ConnectRPC listShells call on mount, and
// useAvailablePrograms fires a real fetch("/api/server-info") — both land outside this
// test's act() scope and produce noisy "not wrapped in act(...)" warnings plus real
// network attempts. Stub them to keep runs deterministic (mirrors
// SessionDetailView.summary-tab.test.tsx's useShells mock and Omnibar.alias.test.tsx's
// useAvailablePrograms mock).
jest.mock("@/lib/hooks/useShells", () => ({
  useShells: () => ({
    shells: [],
    isLoading: false,
    spawnShell: jest.fn(),
    stopShell: jest.fn(),
    restartShell: jest.fn(),
    deleteShell: jest.fn(),
    updateShellStatus: jest.fn(),
    refetch: jest.fn(),
  }),
}));
jest.mock("@/lib/hooks/useAvailablePrograms", () => ({
  useAvailablePrograms: jest.fn(() => []),
}));

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

  it("still renders the tab strip when embedded=true (it's the only tab UI)", () => {
    render(
      <SessionDetail session={makeSession()} embedded onClose={jest.fn()} />
    );
    expect(screen.getByRole("tablist")).toBeInTheDocument();
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
    // Multiple tabpanels are always mounted (terminal + browser use keep-alive pattern)
    const panels = screen.getAllByRole("tabpanel", { hidden: true });
    expect(panels.length).toBeGreaterThan(0);
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

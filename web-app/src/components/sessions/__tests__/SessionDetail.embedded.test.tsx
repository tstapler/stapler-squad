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

// SessionDetail lazy-loads SessionDetailView via next/dynamic (and SessionDetailView
// itself lazy-loads TerminalOutput the same way), so the tab/header DOM structure
// these tests assert on only exists once the dynamic import(s) resolve. next/dynamic
// in the App Router is React.lazy()+Suspense under the hood with no synchronous
// escape hatch, so rather than fight it with a mock, these tests await the first
// element that only appears post-resolution (via findBy*/waitFor) before making
// further assertions — see below.

jest.mock("../DiffViewer", () => ({ DiffViewer: () => <div data-testid="diff-viewer" /> }));
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
  it("does NOT render the title header when embedded=true", async () => {
    render(
      <SessionDetail session={makeSession()} embedded onClose={jest.fn()} />
    );
    // Wait for the lazy-loaded SessionDetailView to resolve via a marker that's
    // always present (tab content), then assert the header is absent.
    await screen.findAllByRole("tabpanel", { hidden: true });
    expect(screen.queryByTestId("session-header")).not.toBeInTheDocument();
  });

  it("still renders the tab strip when embedded=true (it's the only tab UI)", () => {
    render(
      <SessionDetail session={makeSession()} embedded onClose={jest.fn()} />
    );
    expect(screen.getByRole("tablist")).toBeInTheDocument();
  });

  it("renders the title header when embedded is not set", async () => {
    render(<SessionDetail session={makeSession()} onClose={jest.fn()} />);
    expect(await screen.findByTestId("session-header")).toBeInTheDocument();
  });

  it("renders the tab strip when embedded is not set", async () => {
    render(<SessionDetail session={makeSession()} onClose={jest.fn()} />);
    expect(await screen.findByRole("tablist")).toBeInTheDocument();
  });

  it("still renders tab content when embedded=true", async () => {
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
    const panels = await screen.findAllByRole("tabpanel", { hidden: true });
    expect(panels.length).toBeGreaterThan(0);
  });
});

// ─────────────────────────────────────────────────────────────────────────
// Bug 3: initialTab prop changes must sync to displayed content
// ─────────────────────────────────────────────────────────────────────────
describe("SessionDetail — initialTab sync (Bug 3)", () => {
  it("starts on the given initialTab", async () => {
    render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="info"
      />
    );
    // Wait for the lazy-loaded SessionDetailView to resolve before querying its
    // DOM directly (document.querySelector has no async/waiting variant).
    await screen.findAllByRole("tabpanel", { hidden: true });
    // Terminal panel should be hidden when starting on "info"
    const terminalPanel = document.querySelector('[aria-labelledby="tab-terminal"]');
    expect(terminalPanel).toHaveAttribute("aria-hidden", "true");
  });

  it("switches displayed content when initialTab prop changes", async () => {
    const { rerender } = render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="info"
      />
    );
    await screen.findAllByRole("tabpanel", { hidden: true });

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

  it("switches back when initialTab reverts to original value", async () => {
    const { rerender } = render(
      <SessionDetail
        session={makeSession()}
        embedded
        onClose={jest.fn()}
        initialTab="terminal"
      />
    );
    await screen.findAllByRole("tabpanel", { hidden: true });

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

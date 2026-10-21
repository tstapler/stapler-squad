/**
 * Tests for the Backlog Stages settings page (Epic 2.8, Story 2.8.1, Task
 * 2.8.1c):
 *  1. The rendered page never shows "Workflow" as the stage-management
 *     concept's name (research/ux.md §0's naming-collision finding).
 *  2. Toggling a custom stage's enabled switch calls updateStage with only
 *     `{ enabled }` set — no other fields smuggled in.
 */

import React from "react";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import BacklogStagesPage from "./page";
import { useBacklogStagesAdmin } from "@/lib/hooks/useBacklogStages";
import type { BacklogStage } from "@/lib/hooks/useBacklogStages";
import { useSearchParams } from "@/lib/nav";
import { useEngineModeStatus } from "@/lib/hooks/useEngineModeStatus";
import type { EngineModeView } from "@/lib/hooks/useEngineModeStatus";

jest.mock("@/lib/hooks/useBacklogStages", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogStages");
  return {
    ...actual,
    useBacklogStagesAdmin: jest.fn(),
  };
});

// Overrides jest.setup.js's global next/navigation stub (which always
// returns an empty URLSearchParams) so the ?editStage= tests below can
// control what the page reads on mount.
jest.mock("@/lib/nav", () => ({
  ...jest.requireActual("@/lib/nav/impl.memory"),
  useSearchParams: jest.fn(() => new URLSearchParams()),
}));
const mockUseSearchParams = useSearchParams as jest.Mock;

jest.mock("@/components/analytics/PageViewTracker", () => ({
  PageViewTracker: () => null,
}));

// StageForm's own render/data-fetching behavior (transitions, gates,
// pipeline modes) is covered by StageForm.test.tsx — stubbed here so these
// page-level tests only assert what `editingStage` was initialized to.
jest.mock("./StageForm", () => ({
  StageForm: ({
    stage,
    engineMode,
    onSaved,
  }: {
    stage: { slug: string } | null;
    engineMode?: { kind: string };
    onSaved: (s: unknown) => void;
  }) => (
    <div data-testid="stage-form-stub">
      {stage ? `editing:${stage.slug}` : "creating"}
      <span data-testid="stage-form-stub-mode">{engineMode?.kind}</span>
      <button onClick={() => onSaved(stage)}>stub-save</button>
    </div>
  ),
}));

// The status fetch itself is covered by useEngineModeStatus.test.tsx; here the
// page only needs a controllable view shared by the banner and the form.
jest.mock("@/lib/hooks/useEngineModeStatus", () => ({
  ...jest.requireActual("@/lib/hooks/useEngineModeStatus"),
  useEngineModeStatus: jest.fn(),
}));
const mockUseEngineModeStatus = useEngineModeStatus as jest.MockedFunction<typeof useEngineModeStatus>;
const mockRefetchEngineMode = jest.fn().mockResolvedValue(undefined);

function pgTestView(overrides: Partial<EngineModeView> = {}): EngineModeView {
  return {
    kind: "shadow",
    classes: { operator: "shadow", agent: "shadow", system: "shadow" },
    divergence24h: 3,
    stale: false,
    lastCheckedAt: Date.now(),
    fallbackActive: false,
    invalidatorWired: true,
    gateWired: true,
    review: null,
    flagRowId: "flag-row-backlog-workflow-engine-enforce",
    ...overrides,
  };
}

const mockUseBacklogStages = useBacklogStagesAdmin as jest.MockedFunction<typeof useBacklogStagesAdmin>;
const mockListStages = jest.fn();
const mockUpdateStage = jest.fn();

function makeStage(overrides: Partial<BacklogStage> & Pick<BacklogStage, "id" | "slug" | "name">): BacklogStage {
  return {
    description: "",
    isEntry: false,
    isTerminal: false,
    enabled: true,
    ...overrides,
  };
}

// Same benign vanilla-extract jest-mock className warning noted in
// PipelineModeForm.test.tsx / pipeline-modes/page.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
  mockUseBacklogStages.mockReturnValue({
    listStages: mockListStages,
    updateStage: mockUpdateStage,
  } as unknown as ReturnType<typeof useBacklogStagesAdmin>);
  mockUseSearchParams.mockReturnValue(new URLSearchParams());
  mockRefetchEngineMode.mockResolvedValue(undefined);
  mockUseEngineModeStatus.mockReturnValue({ view: pgTestView(), refetch: mockRefetchEngineMode });
});

describe("BacklogStagesPage engine-mode banner (Story 1.3.2)", () => {
  it("renders the banner before the stage list, with the carve-out note", async () => {
    mockListStages.mockResolvedValue([makeStage({ id: "1", slug: "idea", name: "Idea" })]);
    render(<BacklogStagesPage />);
    const banner = screen.getByTestId("engine-mode-banner");
    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());
    expect(banner.compareDocumentPosition(screen.getByTestId("backlog-stage-row-idea"))).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING
    );
    expect(banner).toHaveTextContent("not enforced. 3 differences in the last 24h.");
    expect(screen.getByTestId("backlog-stages-carve-out-note")).toHaveTextContent(
      "Archive from active stages cannot be gated here; gate done->archived instead."
    );
  });

  it("passes the same status object to the form and the banner", async () => {
    mockListStages.mockResolvedValue([makeStage({ id: "1", slug: "idea", name: "Idea" })]);
    mockUseEngineModeStatus.mockReturnValue({ view: pgTestView({ kind: "off" }), refetch: mockRefetchEngineMode });
    render(<BacklogStagesPage />);
    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("backlog-stage-edit-idea"));
    expect(screen.getByTestId("stage-form-stub-mode")).toHaveTextContent("off");
    expect(screen.getByTestId("engine-mode-banner")).toHaveTextContent("Not in use");
  });

  it("a save shows the mode-aware confirmation and refetches the status", async () => {
    mockListStages.mockResolvedValue([makeStage({ id: "1", slug: "idea", name: "Idea" })]);
    render(<BacklogStagesPage />);
    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());
    expect(screen.getByTestId("stage-save-status")).toBeEmptyDOMElement();
    fireEvent.click(screen.getByTestId("backlog-stage-edit-idea"));
    fireEvent.click(screen.getByText("stub-save"));
    expect(screen.getByTestId("stage-save-status")).toHaveTextContent("Saved. Not enforced yet (shadow)");
    expect(mockRefetchEngineMode).toHaveBeenCalledTimes(1);
  });

  it("Retry on the banner refetches the status", async () => {
    mockListStages.mockResolvedValue([]);
    mockUseEngineModeStatus.mockReturnValue({ view: pgTestView({ kind: "unknown" }), refetch: mockRefetchEngineMode });
    render(<BacklogStagesPage />);
    fireEvent.click(screen.getByTestId("engine-mode-banner-retry"));
    expect(mockRefetchEngineMode).toHaveBeenCalledTimes(1);
  });
});

describe("BacklogStagesPage", () => {
  it('never renders "Workflow" as the stage-management concept\'s name', async () => {
    mockListStages.mockResolvedValue([
      makeStage({ id: "1", slug: "idea", name: "Idea", isEntry: true }),
      makeStage({ id: "2", slug: "design-review", name: "Design Review" }),
    ]);

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());

    expect(document.body.textContent).not.toMatch(/Workflow/);
  });

  it("toggling a custom stage's enabled switch calls updateStage with only { enabled } set", async () => {
    const customStage = makeStage({ id: "2", slug: "design-review", name: "Design Review", enabled: true });
    mockListStages.mockResolvedValue([customStage]);
    mockUpdateStage.mockResolvedValue({ ...customStage, enabled: false });

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("backlog-stage-toggle-design-review")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("backlog-stage-toggle-design-review"));

    await waitFor(() => expect(mockUpdateStage).toHaveBeenCalledTimes(1));
    expect(mockUpdateStage).toHaveBeenCalledWith("2", { enabled: false });
  });

  it("disabling a stage does not remove it from the list (still queryable)", async () => {
    const customStage = makeStage({ id: "2", slug: "design-review", name: "Design Review", enabled: true });
    mockListStages.mockResolvedValue([customStage]);
    mockUpdateStage.mockResolvedValue({ ...customStage, enabled: false });

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("backlog-stage-toggle-design-review")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("backlog-stage-toggle-design-review"));

    await waitFor(() => expect(screen.getByTestId("backlog-stage-toggle-design-review")).toHaveAttribute("aria-checked", "false"));
    expect(screen.getByTestId("backlog-stage-row-design-review")).toBeInTheDocument();
  });

  it("shows an empty state when no stages exist", async () => {
    mockListStages.mockResolvedValue([]);
    render(<BacklogStagesPage />);
    await waitFor(() =>
      expect(
        screen.getByText("No stages configured — the built-in 9-stage workflow is active by default.")
      ).toBeInTheDocument()
    );
  });
});

describe("BacklogStagesPage — ?editStage= query param (ADR-005 Decision point 4)", () => {
  it("opens the matching stage's edit form on load", async () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams("editStage=design-review"));
    mockListStages.mockResolvedValue([
      makeStage({ id: "1", slug: "idea", name: "Idea", isEntry: true }),
      makeStage({ id: "2", slug: "design-review", name: "Design Review" }),
    ]);

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("stage-form-stub")).toHaveTextContent("editing:design-review"));
  });

  it("is a no-op when the param is absent, leaving the page in its normal closed-form state", async () => {
    mockListStages.mockResolvedValue([makeStage({ id: "1", slug: "idea", name: "Idea", isEntry: true })]);

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());
    expect(screen.queryByTestId("stage-form-stub")).not.toBeInTheDocument();
  });

  it("is a no-op when the param matches no fetched stage", async () => {
    mockUseSearchParams.mockReturnValue(new URLSearchParams("editStage=does-not-exist"));
    mockListStages.mockResolvedValue([makeStage({ id: "1", slug: "idea", name: "Idea", isEntry: true })]);

    render(<BacklogStagesPage />);

    await waitFor(() => expect(screen.getByTestId("backlog-stage-row-idea")).toBeInTheDocument());
    expect(screen.queryByTestId("stage-form-stub")).not.toBeInTheDocument();
  });
});

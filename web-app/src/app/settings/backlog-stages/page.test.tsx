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
import { useBacklogStages } from "@/lib/hooks/useBacklogStages";
import type { BacklogStage } from "@/lib/hooks/useBacklogStages";

jest.mock("@/lib/hooks/useBacklogStages", () => {
  const actual = jest.requireActual("@/lib/hooks/useBacklogStages");
  return {
    ...actual,
    useBacklogStages: jest.fn(),
  };
});

jest.mock("@/components/analytics/PageViewTracker", () => ({
  PageViewTracker: () => null,
}));

const mockUseBacklogStages = useBacklogStages as jest.MockedFunction<typeof useBacklogStages>;
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
  } as unknown as ReturnType<typeof useBacklogStages>);
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

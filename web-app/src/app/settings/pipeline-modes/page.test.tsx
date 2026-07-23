/**
 * Smoke test for the pipeline-modes management page (Story 3.3.1): renders
 * both enabled and disabled modes, with the disabled mode visually flagged.
 * Not required by validation.md but cheap coverage per plan.md's Task list.
 */

import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import PipelineModesPage from "./page";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import type { PipelineMode } from "@/lib/hooks/useBacklogService";

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

jest.mock("@/components/analytics/PageViewTracker", () => ({
  PageViewTracker: () => null,
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;
const mockListPipelineModes = jest.fn();
const mockUpdatePipelineMode = jest.fn();

function makeMode(overrides: Partial<PipelineMode> & Pick<PipelineMode, "id" | "slug" | "name">): PipelineMode {
  return {
    description: "",
    enabled: true,
    statusCommandTemplate: "",
    doneCommandTemplate: "",
    failCommandTemplate: "",
    reviewCommandTemplate: "",
    shipCommandTemplate: "",
    helpCommandTemplate: "",
    triagePromptTemplate: "",
    reviewPromptTemplate: "",
    initialPromptTemplate: "",
    contentHash: "hash",
    ...overrides,
  };
}

// Same benign vanilla-extract jest-mock className warning as
// PipelineModeForm.test.tsx — see that file's comment for details.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
  mockUseBacklogService.mockReturnValue({
    listPipelineModes: mockListPipelineModes,
    updatePipelineMode: mockUpdatePipelineMode,
  } as unknown as ReturnType<typeof useBacklogService>);
});

describe("PipelineModesPage", () => {
  it("renders both enabled and disabled modes, with the disabled row flagged", async () => {
    mockListPipelineModes.mockResolvedValue([
      makeMode({ id: "1", slug: "quick", name: "Quick Fix", enabled: true }),
      makeMode({ id: "2", slug: "legacy", name: "Legacy", enabled: false }),
    ]);

    render(<PipelineModesPage />);

    await waitFor(() => expect(screen.getByTestId("pipeline-mode-row-quick")).toBeInTheDocument());
    expect(screen.getByTestId("pipeline-mode-row-legacy")).toBeInTheDocument();

    // Disabled row shows a "disabled" badge; the enabled row does not.
    const legacyRow = screen.getByTestId("pipeline-mode-row-legacy");
    expect(legacyRow).toHaveTextContent("disabled");
    const quickRow = screen.getByTestId("pipeline-mode-row-quick");
    expect(quickRow).not.toHaveTextContent("disabled");
  });

  it("shows an empty state when no modes exist", async () => {
    mockListPipelineModes.mockResolvedValue([]);
    render(<PipelineModesPage />);
    await waitFor(() => expect(screen.getByText("No pipeline modes defined yet.")).toBeInTheDocument());
  });
});

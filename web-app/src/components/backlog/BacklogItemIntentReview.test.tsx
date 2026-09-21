/**
 * Tests for BacklogItemIntentReview — the omnibar's "backlog: <message>"
 * review/edit UI (AC1): renders pre-filled editable fields from a parsed
 * draft, falls back to the raw text on parse failure instead of losing it,
 * and lets an edit reach CreateBacklogItem instead of the original draft.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { BacklogItemIntentReview } from "./BacklogItemIntentReview";
import { useBacklogService } from "@/lib/hooks/useBacklogService";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";

jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({
  useSessionRepoPaths: () => [],
}));

jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));

jest.mock("@/lib/hooks/useBacklogService", () => ({
  useBacklogService: jest.fn(),
}));

jest.mock("@/lib/contexts/FeatureFlagsContext", () => ({
  useFeatureFlag: jest.fn(() => false),
}));

const mockUseBacklogService = useBacklogService as jest.MockedFunction<typeof useBacklogService>;
const mockUseFeatureFlag = useFeatureFlag as jest.MockedFunction<typeof useFeatureFlag>;

function mockService(overrides: Partial<ReturnType<typeof useBacklogService>>) {
  mockUseBacklogService.mockReturnValue({
    listPipelineModes: () => Promise.resolve([]),
    parseBacklogItemIntent: jest.fn().mockResolvedValue(null),
    createBacklogItem: jest.fn().mockResolvedValue(null),
    ...overrides,
  } as unknown as ReturnType<typeof useBacklogService>);
}

beforeEach(() => {
  mockUseBacklogService.mockReset();
  mockUseFeatureFlag.mockReset();
  mockUseFeatureFlag.mockReturnValue(false);
});

describe("BacklogItemIntentReview — successful parse", () => {
  it("renders pre-filled editable fields from the parsed draft, not the raw text", async () => {
    const parseBacklogItemIntent = jest.fn().mockResolvedValue({
      title: "Add CSV export",
      description: "Let users export their data as CSV from settings.",
      acceptanceCriteria: ["Export button appears in settings", "Downloaded file is valid CSV"],
      confidence: 0.8,
    });
    mockService({ parseBacklogItemIntent });

    render(
      <BacklogItemIntentReview
        initialText="please add a way to export my data as csv from settings thanks"
        onDone={jest.fn()}
        onCancel={jest.fn()}
      />
    );

    expect(screen.getByText("Parsing your message into a backlog item…")).toBeInTheDocument();

    const titleInput = await screen.findByDisplayValue("Add CSV export");
    expect(titleInput).toBeInTheDocument();
    expect(
      screen.getByDisplayValue("Let users export their data as CSV from settings.")
    ).toBeInTheDocument();
    expect(screen.queryByText("Couldn’t parse this into a structured item — review the fields below before creating.", { exact: false })).not.toBeInTheDocument();
  });

  it("submits the user's edited values, not the original draft, on confirm", async () => {
    const parseBacklogItemIntent = jest.fn().mockResolvedValue({
      title: "Add CSV export",
      description: "Let users export their data as CSV from settings.",
      acceptanceCriteria: [],
      confidence: 0.8,
    });
    const createBacklogItem = jest.fn().mockResolvedValue({
      item: { id: "item-1" },
      triageTriggered: false,
    });
    mockService({ parseBacklogItemIntent, createBacklogItem });
    const onDone = jest.fn();

    render(
      <BacklogItemIntentReview initialText="export data as csv" onDone={onDone} onCancel={jest.fn()} />
    );

    const titleInput = await screen.findByDisplayValue("Add CSV export");
    fireEvent.change(titleInput, { target: { value: "Add CSV export button" } });
    fireEvent.change(screen.getByTestId("backlog-repo-path-input"), {
      target: { value: "/repo" },
    });

    fireEvent.click(screen.getByRole("button", { name: /create/i }));

    await waitFor(() => expect(createBacklogItem).toHaveBeenCalled());
    expect(createBacklogItem.mock.calls[0][0]).toMatchObject({ title: "Add CSV export button" });
    expect(createBacklogItem.mock.calls[0][0].title).not.toBe("Add CSV export");
    await waitFor(() => expect(onDone).toHaveBeenCalledWith({ id: "item-1" }));
  });
});

describe("BacklogItemIntentReview — parse failure fallback", () => {
  it("pre-fills the raw text instead of losing it, and shows a fallback banner", async () => {
    const parseBacklogItemIntent = jest.fn().mockResolvedValue(null);
    mockService({ parseBacklogItemIntent });

    const rawText = "fix the login bug\nusers get logged out after 5 minutes";
    render(
      <BacklogItemIntentReview initialText={rawText} onDone={jest.fn()} onCancel={jest.fn()} />
    );

    expect(
      await screen.findByTestId("backlog-intent-review-parse-failed-banner")
    ).toHaveTextContent("Couldn’t parse this into a structured item");
    expect(screen.getByDisplayValue("fix the login bug")).toBeInTheDocument();
    expect(screen.getByTestId("backlog-description-input")).toHaveValue(rawText);
  });
});

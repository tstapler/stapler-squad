/**
 * Integration coverage for the triage clarifying-question answer flow, replacing
 * tests/e2e/triage-question-answer.spec.ts. Renders the REAL BacklogItemDetail →
 * TriageReviewPanel → TriageDiffSection chain (only network/hook edges are mocked)
 * so the prop threading onAnswerQuestion → handleRefineTriage → triggerTriage is exercised.
 *
 * Fixture invariant: TriageReviewPanel only renders TriageDiffSection (and so the
 * question block) when at least one non-"question" suggestion exists, so
 * the fixture always carries one AC suggestion alongside the questions.
 */

import React from "react";
import { render, screen, act, fireEvent } from "@testing-library/react";
import { BacklogItemDetail } from "./BacklogItemDetail";
import type { BacklogItem } from "@/lib/hooks/useBacklogService";

jest.mock("./SessionMonitor", () => ({ SessionMonitor: () => null }));
jest.mock("./GateVerdictBox", () => ({ GateVerdictBox: () => null }));
jest.mock("./TriageRelatedWorkSection", () => ({ TriageRelatedWorkSection: () => null }));
jest.mock("./TriageLoadingIndicator", () => ({ TriageLoadingIndicator: () => null }));
jest.mock("./ReviewChangesModal", () => ({ ReviewChangesModal: () => null }));
jest.mock("./BacklogFileBrowserModal", () => ({ BacklogFileBrowserModal: () => null }));

jest.mock("@/lib/hooks/useVcsStatus", () => ({
  useVcsStatus: () => ({ data: null, loading: false, error: null, refetch: jest.fn() }),
}));
jest.mock("@/lib/hooks/useBacklogItemShipStatus", () => ({
  useBacklogItemShipStatus: () => ({ data: null, loading: false, refetch: jest.fn() }),
}));
jest.mock("@/lib/hooks/useStuckBacklogItems", () => ({
  useStuckBacklogItems: () => ({ items: [], isLoading: false, error: null }),
}));
jest.mock("@/lib/hooks/useSessionRepoPaths", () => ({ useSessionRepoPaths: () => [] }));
jest.mock("@/lib/hooks/usePathCompletions", () => ({
  usePathCompletions: () => ({ entries: [], isLoading: false }),
}));
jest.mock("@/lib/hooks/useSessionService", () => ({
  useSessionService: () => ({ deleteSession: jest.fn() }),
}));
jest.mock("@/lib/analytics", () => ({ useAnalytics: () => ({ track: jest.fn() }) }));
jest.mock("@/lib/hooks/useWatchBacklogItems", () => ({
  useWatchBacklogItems: () => ({ items: [], connectionState: "live" }),
}));
jest.mock("@/lib/store", () => ({
  useAppSelector: (selector: (state: unknown) => unknown) => selector({ backlogItems: { items: {} } }),
}));
jest.mock("@connectrpc/connect", () => ({
  ...jest.requireActual("@connectrpc/connect"),
  createClient: () => ({
    watchBacklogItems: () =>
      (async function* () {
        /* no events */
      })(),
  }),
}));
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));

const getBacklogItem = jest.fn();
const listPipelineModes = jest.fn().mockResolvedValue([]);
const transitionStatus = jest.fn().mockResolvedValue(true);
const triggerTriage = jest.fn().mockResolvedValue(undefined);

jest.mock("@/lib/hooks/useBacklogService", () => ({
  ...jest.requireActual("@/lib/hooks/useBacklogService"),
  useBacklogService: () => ({
    getBacklogItem,
    transitionStatus,
    triggerTriage,
    cancelTriage: jest.fn(),
    spawnSessionFromItem: jest.fn(),
    approvePlan: jest.fn(),
    overrideVerdict: jest.fn(),
    triggerReReview: jest.fn(),
    triggerShipPR: jest.fn(),
    submitManualReview: jest.fn(),
    archiveBacklogItem: jest.fn(),
    deleteBacklogItem: jest.fn(),
    updateBacklogItem: jest.fn().mockResolvedValue(null),
    listPipelineModes,
    lastError: null,
  }),
}));

// vanilla-extract .css.ts jest mock wraps every export in a callable proxy,
// which triggers a benign "Invalid value for prop className" warning —
// pre-existing limitation, silenced the same way BacklogItemDetail.test.tsx does.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});
afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  transitionStatus.mockClear().mockResolvedValue(true);
  triggerTriage.mockClear().mockResolvedValue(undefined);
});

function makeItem(questions: string[], withAcSuggestion = true): BacklogItem {
  return {
    id: "item-1",
    title: "Answer triage questions",
    description: "desc",
    status: "idea",
    priority: 3,
    repoPath: "/tmp/repo",
    skipPlanning: false,
    skipReviewGate: false,
    autoSpawnSession: false,
    autoCreatePR: false,
    autoApprovePlan: false,
    planApproved: false,
    planArtifactsPath: undefined,
    acCriteria: [],
    linkedSessions: [],
    notes: "",
    createdAt: "2026-08-02T18:00:00Z",
    updatedAt: "2026-08-03T02:52:54Z",
    statusEvents: [],
    progressNotes: [],
    activityNotes: [],
    totalEstimatedCostUsd: 0,
    triageStatus: "completed",
    triageResult: {
      summary: "Found 1 suggestion and clarifying questions.",
      suggestions: [
        ...(withAcSuggestion ? [{ text: "Add a regression test for the new path", rationale: "coverage" }] : []),
        ...questions.map((q) => ({ text: q, rationale: "question" })),
      ],
      clarifyingQuestions: [],
      tasks: [],
      iteration: 1,
      feedback: "",
    },
  };
}

async function renderItem(questions: string[], withAcSuggestion = true) {
  getBacklogItem.mockReset().mockResolvedValue(makeItem(questions, withAcSuggestion));
  render(<BacklogItemDetail itemId="item-1" />);
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

describe("BacklogItemDetail — triage question answer flow", () => {
  it("renders the question block only because the fixture has a non-question suggestion (hasSuggestions gate)", async () => {
    await renderItem(["What database should this use?"]);
    expect(await screen.findByTestId("triage-question-answer-toggle-0")).toBeInTheDocument();
  });

  it("hides the question block when only question suggestions exist (hasSuggestions gate)", async () => {
    await renderItem(["What database should this use?"], false);
    expect(screen.queryByTestId("triage-question-answer-toggle-0")).not.toBeInTheDocument();
  });

  it("submits a composed Q:/A: answer through handleRefineTriage to triggerTriage and shows the answered marker", async () => {
    await renderItem(["What database should this use?"]);
    fireEvent.click(await screen.findByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), { target: { value: "Postgres" } });
    const submit = screen.getByTestId("triage-question-answer-submit-0");
    expect(submit).toBeEnabled();
    await act(async () => {
      fireEvent.click(submit);
      await Promise.resolve();
    });

    expect(triggerTriage).toHaveBeenCalledTimes(1);
    const [itemId, feedback] = triggerTriage.mock.calls[0];
    expect(itemId).toBe("item-1");
    expect(feedback).toContain("What database should this use?");
    expect(feedback).toContain("Postgres");
    expect(await screen.findByText("✓ Answered: Postgres")).toBeInTheDocument();
    // handleRefineTriage reloads the item after the RPC resolves.
    expect(getBacklogItem.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("keeps the draft and surfaces an error when triggerTriage rejects", async () => {
    triggerTriage.mockRejectedValue(new Error("a triage run is already in progress for this item"));
    await renderItem(["Should this support offline mode?"]);
    fireEvent.click(await screen.findByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), {
      target: { value: "Yes, offline-first." },
    });
    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-question-answer-submit-0"));
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(await screen.findByText(/Failed to submit answer/)).toBeInTheDocument();
    expect(screen.getByTestId("triage-question-answer-input-0")).toHaveValue("Yes, offline-first.");
    expect(screen.queryByText(/✓ Answered/)).not.toBeInTheDocument();
  });

  it("returns focus to the Answer toggle when Cancel is pressed", async () => {
    await renderItem(["What database should this use?"]);
    fireEvent.click(await screen.findByTestId("triage-question-answer-toggle-0"));
    fireEvent.change(screen.getByTestId("triage-question-answer-input-0"), { target: { value: "partial draft" } });
    await act(async () => {
      fireEvent.click(screen.getByTestId("triage-question-answer-cancel-0"));
      await Promise.resolve();
    });

    const toggle = screen.getByTestId("triage-question-answer-toggle-0");
    expect(toggle).toBeVisible();
    expect(toggle).toHaveFocus();
  });

  it("keeps Submit disabled until an answer is typed and never fires triggerTriage", async () => {
    await renderItem(["What database should this use?"]);
    fireEvent.click(await screen.findByTestId("triage-question-answer-toggle-0"));
    const submit = screen.getByTestId("triage-question-answer-submit-0");
    expect(submit).toHaveAttribute("aria-disabled", "true");
    expect(submit).toBeDisabled();
    fireEvent.click(submit);
    expect(triggerTriage).not.toHaveBeenCalled();
  });
});

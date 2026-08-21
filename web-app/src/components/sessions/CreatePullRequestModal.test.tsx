/**
 * Tests for CreatePullRequestModal (Epic 3.1 of project_plans/session-pr-creation).
 *
 * Covers ux.md's UX Acceptance Criteria as they apply to this component:
 *  - Prefill-on-open (AC1) and free editing of the base-branch field (AC2)
 *  - Existing-PR "View PR" dead-end state (AC4, Surface 5)
 *  - Submit calls createPullRequest with edited field values
 *  - Persist-failure warning renders alongside (not instead of) the success
 *    PR link (Surface 7 Variant C, criterion 5)
 *  - Field values survive a submit failure (criterion 7)
 *  - Draft-fetch failure offers both Retry and Close — no dead ends (criterion 6)
 *  - All fields/buttons disabled while submitting, and a double-click can
 *    only ever trigger one createPullRequest call (criterion 8)
 *  - Every input has a programmatically-associated label (criterion 11)
 *  - Success/warning states are conveyed by icon + text, not color alone (criterion 13)
 *
 * jest-axe is not a devDependency in this repo yet and no other test file
 * imports it, so per the task's own guidance this suite asserts the a11y
 * contract manually (getByLabelText resolving for every field) rather than
 * introducing a new dependency for a single test.
 */

import React from "react";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import { CreatePullRequestModal } from "./CreatePullRequestModal";
import type { Session } from "@/gen/session/v1/types_pb";
import type {
  DraftPullRequestResponse,
  CreatePullRequestResponse,
} from "@/gen/session/v1/session_pb";

// ---------------------------------------------------------------------------
// Mock: useFocusTrap — the hook uses DOM focus operations that behave poorly
// in jsdom (same pattern as ResumeSessionModal.test.tsx).
// ---------------------------------------------------------------------------
jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: () => undefined,
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeSession(overrides: Partial<Record<string, unknown>> = {}): Session {
  return {
    id: "session-1",
    branch: "feature/rate-limit-toggle",
    ...overrides,
  } as unknown as Session;
}

function makeDraft(
  overrides: Partial<DraftPullRequestResponse> = {}
): DraftPullRequestResponse {
  return {
    title: "Add rate limit toggle",
    body: "## Summary\nAdds a per-user rate limit toggle.",
    baseBranch: "main",
    hasCommitsAhead: true,
    existingPrUrl: "",
    existingPrNumber: 0,
    ...overrides,
  } as unknown as DraftPullRequestResponse;
}

function makeCreateSuccess(
  overrides: Partial<CreatePullRequestResponse> = {}
): CreatePullRequestResponse {
  return {
    prUrl: "https://github.com/tstapler/stapler-squad/pull/512",
    prNumber: 512,
    alreadyExisted: false,
    persisted: true,
    persistError: "",
    ...overrides,
  } as unknown as CreatePullRequestResponse;
}

/** Waits for the drafting spinner to disappear and the editable form to render. */
async function waitForDraftedForm() {
  await waitFor(() => {
    expect(screen.queryByTestId("create-pr-loading")).not.toBeInTheDocument();
  });
  return screen.getByTestId("create-pr-title-input") as HTMLInputElement;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("CreatePullRequestModal", () => {
  it("CreatePullRequestModal_should_PrefillFields_When_Opened", async () => {
    const draft = makeDraft();
    const draftPullRequest = jest.fn().mockResolvedValue(draft);

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={jest.fn()}
      />
    );

    const titleInput = await waitForDraftedForm();
    expect(titleInput.value).toBe(draft.title);
    expect(
      (screen.getByTestId("create-pr-body-input") as HTMLTextAreaElement).value
    ).toBe(draft.body);
    expect(
      (screen.getByTestId("create-pr-base-branch-select") as HTMLInputElement).value
    ).toBe(draft.baseBranch);
    expect(draftPullRequest).toHaveBeenCalledWith("session-1");
  });

  it("CreatePullRequestModal_should_AllowEditingBaseBranch_When_UserTypes", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={jest.fn()}
      />
    );

    await waitForDraftedForm();
    const baseBranchInput = screen.getByTestId(
      "create-pr-base-branch-select"
    ) as HTMLInputElement;

    fireEvent.change(baseBranchInput, { target: { value: "release/1.2" } });

    expect(baseBranchInput.value).toBe("release/1.2");
  });

  it("CreatePullRequestModal_should_ShowViewPrLink_When_PrAlreadyExists", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(
      makeDraft({
        existingPrUrl: "https://github.com/tstapler/stapler-squad/pull/512",
        existingPrNumber: 512,
      })
    );

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={jest.fn()}
      />
    );

    const link = await screen.findByTestId("github-pr-link");
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/tstapler/stapler-squad/pull/512"
    );
    expect(screen.queryByTestId("create-pr-title-input")).not.toBeInTheDocument();
  });

  it("CreatePullRequestModal_should_CallCreatePullRequest_When_Submitted", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());
    const createPullRequest = jest.fn().mockResolvedValue(makeCreateSuccess());

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
      />
    );

    await waitForDraftedForm();
    fireEvent.change(screen.getByTestId("create-pr-title-input"), {
      target: { value: "Edited title" },
    });
    fireEvent.change(screen.getByTestId("create-pr-body-input"), {
      target: { value: "Edited body" },
    });
    fireEvent.change(screen.getByTestId("create-pr-base-branch-select"), {
      target: { value: "release/1.2" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("create-pr-submit"));
    });

    expect(createPullRequest).toHaveBeenCalledWith({
      sessionId: "session-1",
      title: "Edited title",
      body: "Edited body",
      baseBranch: "release/1.2",
    });
  });

  it("CreatePullRequestModal_should_ShowPersistWarning_When_PersistedFalse", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());
    const createPullRequest = jest.fn().mockResolvedValue(
      makeCreateSuccess({ persisted: false, persistError: "disk full" })
    );

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
      />
    );

    await waitForDraftedForm();
    await act(async () => {
      fireEvent.click(screen.getByTestId("create-pr-submit"));
    });

    // Success PR link must still be present — persist failure is additive.
    const link = await screen.findByTestId("github-pr-link");
    expect(link).toHaveAttribute(
      "href",
      "https://github.com/tstapler/stapler-squad/pull/512"
    );

    const warning = screen.getByRole("alert");
    expect(warning).toHaveTextContent("disk full");
  });

  it("CreatePullRequestModal_should_PreserveFieldValues_When_SubmitFails", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());
    const createPullRequest = jest
      .fn()
      .mockRejectedValue(new Error("GitHub CLI is not configured. Please run 'gh auth login' first"));

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
      />
    );

    await waitForDraftedForm();
    fireEvent.change(screen.getByTestId("create-pr-title-input"), {
      target: { value: "My edited title" },
    });
    fireEvent.change(screen.getByTestId("create-pr-body-input"), {
      target: { value: "My edited body" },
    });
    fireEvent.change(screen.getByTestId("create-pr-base-branch-select"), {
      target: { value: "my-base" },
    });

    await act(async () => {
      fireEvent.click(screen.getByTestId("create-pr-submit"));
    });

    expect(screen.getByTestId("create-pr-error")).toHaveTextContent(
      "GitHub CLI is not configured. Please run 'gh auth login' first"
    );
    expect(
      (screen.getByTestId("create-pr-title-input") as HTMLInputElement).value
    ).toBe("My edited title");
    expect(
      (screen.getByTestId("create-pr-body-input") as HTMLTextAreaElement).value
    ).toBe("My edited body");
    expect(
      (screen.getByTestId("create-pr-base-branch-select") as HTMLInputElement).value
    ).toBe("my-base");
  });

  it("CreatePullRequestModal_should_OfferRetryAndClose_When_DraftFetchFails", async () => {
    const draftPullRequest = jest.fn().mockRejectedValue(new Error("network error"));

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={jest.fn()}
      />
    );

    const errorEl = await screen.findByTestId("create-pr-error");
    expect(errorEl).toBeInTheDocument();

    // Retry re-fires the draft fetch.
    const retryButton = screen.getByTestId("create-pr-retry");
    expect(retryButton).toBeInTheDocument();
    expect(retryButton).toBeEnabled();

    // Close is an unconditional exit.
    const closeButton = screen.getByTestId("create-pr-close");
    expect(closeButton).toBeInTheDocument();
    expect(closeButton).toBeEnabled();

    await act(async () => {
      fireEvent.click(retryButton);
    });
    expect(draftPullRequest).toHaveBeenCalledTimes(2);
  });

  it("CreatePullRequestModal_should_DisableAllFieldsAndButtons_When_Submitting", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());
    // Never resolves — keeps the component in the submitting state so we can
    // assert the disabled/locked UI (Surface 6).
    let resolveCreate: (value: CreatePullRequestResponse) => void = () => {};
    const createPullRequest = jest.fn(
      () =>
        new Promise<CreatePullRequestResponse>((resolve) => {
          resolveCreate = resolve;
        })
    );

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
      />
    );

    await waitForDraftedForm();
    const submitButton = screen.getByTestId("create-pr-submit");

    // Fire two rapid clicks (before the state update from the first click
    // has a chance to disable the button) — the submittingRef guard must
    // still ensure createPullRequest is called exactly once.
    await act(async () => {
      fireEvent.click(submitButton);
      fireEvent.click(submitButton);
    });

    expect(createPullRequest).toHaveBeenCalledTimes(1);

    await waitFor(() => {
      expect(screen.getByTestId("create-pr-title-input")).toBeDisabled();
    });
    expect(screen.getByTestId("create-pr-body-input")).toBeDisabled();
    expect(screen.getByTestId("create-pr-base-branch-select")).toBeDisabled();
    expect(screen.getByTestId("create-pr-cancel")).toBeDisabled();
    expect(screen.getByTestId("create-pr-submit")).toBeDisabled();

    // Clean up the pending promise so it doesn't leak into the next test.
    await act(async () => {
      resolveCreate(makeCreateSuccess());
    });
  });

  it("CreatePullRequestModal_should_HaveAccessibleLabelsForAllInputs_When_Rendered", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={jest.fn()}
      />
    );

    await waitForDraftedForm();

    expect(screen.getByLabelText("Title")).toBe(screen.getByTestId("create-pr-title-input"));
    expect(screen.getByLabelText("Description")).toBe(
      screen.getByTestId("create-pr-body-input")
    );
    expect(screen.getByLabelText("Base branch")).toBe(
      screen.getByTestId("create-pr-base-branch-select")
    );

    // Dialog itself is labeled via aria-labelledby pointing at the <h3> title.
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAccessibleName("Create Pull Request");
  });

  it("CreatePullRequestModal_should_PairIconAndTextWithSuccessAndWarningStates_When_Rendered", async () => {
    const draftPullRequest = jest.fn().mockResolvedValue(makeDraft());
    const createPullRequest = jest.fn().mockResolvedValue(
      makeCreateSuccess({ persisted: false, persistError: "disk full" })
    );

    render(
      <CreatePullRequestModal
        session={makeSession()}
        isOpen={true}
        onClose={jest.fn()}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
      />
    );

    await waitForDraftedForm();
    await act(async () => {
      fireEvent.click(screen.getByTestId("create-pr-submit"));
    });

    await screen.findByTestId("github-pr-link");

    // Success state pairs the ✅ icon with actual text, not styling alone.
    const successText = screen.getByText(/Created PR #512/);
    expect(successText).toHaveTextContent("✅");
    expect(successText).toHaveTextContent("Created PR #512");

    // Warning banner pairs the ⚠ icon with actual text.
    const warning = screen.getByRole("alert");
    expect(warning).toHaveTextContent("⚠");
    expect(warning).toHaveTextContent("couldn't be saved to the session");
  });
});

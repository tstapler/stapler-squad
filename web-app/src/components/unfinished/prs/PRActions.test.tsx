import React from "react";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { create } from "@bufbuild/protobuf";
import { PRActionResponseSchema } from "@/gen/session/v1/github_user_pb";
import type { PRActionClient } from "@/lib/hooks/usePRActions";
import { PRCard } from "./PRCard";
import { makePR } from "./prTestFixtures";

jest.mock("./PRCard.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));
jest.mock("./NudgeButton.css", () => new Proxy({}, { get: (_t, prop) => (typeof prop === "string" ? prop : "") }));

const ok = () => Promise.resolve(create(PRActionResponseSchema, { success: true }));

function fakeClient(over: Partial<PRActionClient> = {}) {
  const client = {
    mergePRDirect: jest.fn(ok),
    closePRDirect: jest.fn(ok),
    convertPRToDraft: jest.fn(ok),
    markPRReady: jest.fn(ok),
    ...over,
  } as jest.Mocked<PRActionClient>;
  return client;
}

beforeEach(() => window.localStorage.clear());

describe("PRCard details section", () => {
  const pr = makePR({
    body: "x".repeat(400),
    labels: ["bug", "ui"],
    additions: 12,
    deletions: 3,
    changedFiles: 4,
    requestedReviewerLogins: ["bob"],
  });

  it("prCard_should_CollapseDetailsByDefaultAndShowPreviewLabelsStatsReviewers_When_Expanded", () => {
    render(<PRCard pr={pr} />);
    expect(screen.queryByTestId("pr-details")).toBeNull();
    fireEvent.click(screen.getByTestId("pr-details-toggle"));
    expect(screen.getByTestId("pr-details").textContent).toContain("x".repeat(300) + "…");
    expect(screen.getByTestId("pr-details").textContent).not.toContain("x".repeat(301));
    expect(screen.getByText("bug")).toBeInTheDocument();
    expect(screen.getByTestId("pr-stats")).toHaveTextContent("+12 / -3 / 4 files");
    expect(screen.getByTestId("pr-reviewers")).toHaveTextContent("bob");
  });

  it("prCard_should_OmitDetails_When_DetailsNotLoaded", () => {
    render(<PRCard pr={makePR({ detailsLoaded: false })} />);
    expect(screen.queryByTestId("pr-details-toggle")).toBeNull();
  });
});

describe("PRCard actions", () => {
  it("prCard_should_MergeWithPersistedMethodAndHideCard_When_MergeClicked", async () => {
    const client = fakeClient();
    const onNothingToFix = jest.fn();
    render(<PRCard pr={makePR({ nodeId: "PR_1" })} actionClient={client} onNothingToFix={onNothingToFix} />);
    fireEvent.change(screen.getByLabelText("Merge method"), { target: { value: "squash" } });
    expect(window.localStorage.getItem("pr-panel-merge-method")).toBe("squash");
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-merge-button"));
    });
    expect(client.mergePRDirect).toHaveBeenCalledWith(
      expect.objectContaining({ owner: "acme", repo: "api", number: 42, host: "github.com", method: "squash" }),
    );
    expect(screen.queryByTestId("github-pr-card")).toBeNull();
    expect(onNothingToFix).toHaveBeenCalled();
  });

  it("prCard_should_KeepDialogAndReturnFocusToClose_When_CloseFails", async () => {
    const client = fakeClient({ closePRDirect: jest.fn(() => Promise.reject(new Error("denied"))) });
    render(<PRCard pr={makePR()} actionClient={client} />);
    fireEvent.click(screen.getByTestId("pr-close-button"));
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-close-confirm"));
    });
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("alert")).toHaveTextContent("denied");
    expect(screen.getByTestId("pr-close-button")).toHaveFocus();
  });

  it("prCard_should_DisableButtonsWhileMergeInFlight", async () => {
    let resolve!: () => void;
    const client = fakeClient({
      mergePRDirect: jest.fn(() => new Promise((r) => (resolve = () => r(create(PRActionResponseSchema))))),
    });
    render(<PRCard pr={makePR({ nodeId: "PR_1" })} actionClient={client} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-merge-button"));
    });
    expect(screen.getByTestId("pr-merge-button")).toBeDisabled();
    expect(screen.getByTestId("pr-merge-button")).toHaveTextContent("Merging…");
    expect(screen.getByTestId("pr-close-button")).toBeDisabled();
    await act(async () => resolve());
  });

  it("prCard_should_ShowInlineError_When_MergeFails", async () => {
    const client = fakeClient({ mergePRDirect: jest.fn(() => Promise.reject(new Error("not mergeable"))) });
    render(<PRCard pr={makePR()} actionClient={client} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-merge-button"));
    });
    expect(screen.getByRole("alert")).toHaveTextContent("not mergeable");
    expect(screen.getByTestId("github-pr-card")).toBeInTheDocument();
  });

  it("prCard_should_NotShowMerge_When_Draft", () => {
    render(<PRCard pr={makePR({ isDraft: true, nodeId: "PR_1" })} actionClient={fakeClient()} />);
    expect(screen.queryByTestId("pr-merge-button")).toBeNull();
    expect(screen.getByTestId("pr-draft-toggle-button")).toHaveTextContent("Mark as ready");
  });

  it("prCard_should_ConfirmBeforeClosing_When_CloseClicked", async () => {
    const client = fakeClient();
    render(<PRCard pr={makePR()} actionClient={client} />);
    fireEvent.click(screen.getByTestId("pr-close-button"));
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(client.closePRDirect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();

    fireEvent.click(screen.getByTestId("pr-close-button"));
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-close-confirm"));
    });
    expect(client.closePRDirect).toHaveBeenCalledWith(expect.objectContaining({ number: 42 }));
    expect(screen.queryByTestId("github-pr-card")).toBeNull();
  });

  it("prCard_should_CallDraftRPCs_When_Toggled", async () => {
    const toDraft = fakeClient();
    const { unmount } = render(<PRCard pr={makePR({ nodeId: "PR_1" })} actionClient={toDraft} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-draft-toggle-button"));
    });
    expect(toDraft.convertPRToDraft).toHaveBeenCalledWith(expect.objectContaining({ owner: "acme", number: 42 }));
    unmount();

    const toReady = fakeClient();
    render(<PRCard pr={makePR({ isDraft: true, nodeId: "PR_1" })} actionClient={toReady} />);
    await act(async () => {
      fireEvent.click(screen.getByTestId("pr-draft-toggle-button"));
    });
    expect(toReady.markPRReady).toHaveBeenCalledWith(expect.objectContaining({ owner: "acme", number: 42 }));
  });

  it("prCard_should_DisableDraftToggle_When_NodeIdMissing", () => {
    render(<PRCard pr={makePR({ nodeId: "" })} actionClient={fakeClient()} />);
    expect(screen.getByTestId("pr-draft-toggle-button")).toBeDisabled();
  });
});

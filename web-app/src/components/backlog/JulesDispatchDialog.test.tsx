/**
 * Story 3.2.1 (google-jules-integration): JulesDispatchDialog — branch
 * prefill, first-use egress confirmation gating, and the shared
 * useFocusTrap wiring (mirrors ReviewChangesModal's own focus-return
 * coverage in __tests__/ReviewChangesModal.focusReturn.test.tsx).
 */

import React, { useRef, useState } from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { JulesDispatchDialog } from "./JulesDispatchDialog";
import type { AcCriterion } from "@/lib/hooks/useBacklogService";

const dispatchToJules = jest.fn();
const confirmEgressConsent = jest.fn();

jest.mock("@connectrpc/connect", () => {
  const actual = jest.requireActual("@connectrpc/connect");
  return {
    ...actual,
    createClient: () => ({
      dispatchToJules: (...args: unknown[]) => dispatchToJules(...args),
      confirmEgressConsent: (...args: unknown[]) => confirmEgressConsent(...args),
    }),
  };
});
jest.mock("@connectrpc/connect-web", () => ({
  createConnectTransport: jest.fn().mockReturnValue({}),
}));
jest.mock("@/lib/config", () => ({
  getApiBaseUrl: () => "http://localhost:8543",
  createAuthInterceptor: () => (next: unknown) => next,
}));
jest.mock("./JulesDispatchDialog.css", () => {
  return new Proxy({}, { get: (_target, prop) => (typeof prop === "string" ? prop : "") });
});

const AC_CRITERIA: AcCriterion[] = [{ index: 0, text: "Fix the flaky poller test", status: "pending" }];
const REPO_PATH = "/home/tstapler/code/github.com/tstapler/stapler-squad";

function Harness({
  initialBranch = "backlog/fix-flaky-poller-test",
  egressAcknowledged = false,
}: {
  initialBranch?: string;
  egressAcknowledged?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement | null>(null);

  return (
    <div>
      <button
        data-testid="dispatch-opener"
        onClick={(e) => {
          triggerRef.current = e.currentTarget;
          setOpen(true);
        }}
      >
        Dispatch to Jules
      </button>
      {open && (
        <JulesDispatchDialog
          itemId="item-1"
          itemTitle="Fix flaky poller test"
          acCriteria={AC_CRITERIA}
          repoPath={REPO_PATH}
          initialBranch={initialBranch}
          egressAcknowledged={egressAcknowledged}
          onClose={() => setOpen(false)}
          triggerRef={triggerRef}
        />
      )}
    </div>
  );
}

async function openDialog(props: { initialBranch?: string; egressAcknowledged?: boolean } = {}) {
  render(<Harness {...props} />);
  fireEvent.click(screen.getByTestId("dispatch-opener"));
  await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
}

describe("JulesDispatchDialog", () => {
  beforeEach(() => {
    dispatchToJules.mockReset();
    confirmEgressConsent.mockReset();
  });

  it("pre-fills the Branch field with the item's most recent non-empty worktree_branch and keeps it editable", async () => {
    await openDialog({ initialBranch: "backlog/fix-flaky-poller-test" });

    const branchInput = screen.getByTestId("jules-dispatch-branch") as HTMLInputElement;
    expect(branchInput.value).toBe("backlog/fix-flaky-poller-test");

    fireEvent.change(branchInput, { target: { value: "backlog/some-other-branch" } });
    expect(branchInput.value).toBe("backlog/some-other-branch");
  });

  it("disables Dispatch until the named-repo egress checkbox is checked for an unacknowledged repo", async () => {
    await openDialog({ egressAcknowledged: false });

    // Branch + prompt are already prefilled by the harness's defaults.
    const dispatchButton = screen.getByTestId("jules-dispatch-submit");
    expect(dispatchButton).toBeDisabled();

    const checkbox = screen.getByTestId("jules-dispatch-egress-checkbox");
    fireEvent.click(checkbox);

    expect(dispatchButton).toBeEnabled();
  });

  it("omits the egress confirmation block when the repo is already acknowledged", async () => {
    await openDialog({ egressAcknowledged: true });

    expect(screen.queryByTestId("jules-dispatch-egress-block")).not.toBeInTheDocument();
    expect(screen.queryByTestId("jules-dispatch-egress-checkbox")).not.toBeInTheDocument();

    // Dispatch enables on branch+prompt alone, no checkbox required.
    expect(screen.getByTestId("jules-dispatch-submit")).toBeEnabled();
  });

  it("shows the pushed-branch helper text on focus and keeps Dispatch disabled for an empty branch", async () => {
    await openDialog({ initialBranch: "", egressAcknowledged: true });

    const branchInput = screen.getByTestId("jules-dispatch-branch");
    fireEvent.focus(branchInput);

    expect(
      screen.getByText(
        "Jules starts from a branch already pushed to GitHub — local-only branches won't work."
      )
    ).toBeInTheDocument();
    expect(screen.getByTestId("jules-dispatch-submit")).toBeDisabled();
  });

  it("traps Tab at the dialog boundary and returns focus to the opening button on close", async () => {
    await openDialog({ egressAcknowledged: true });

    // egressAcknowledged suppresses the checkbox block, so the focusable
    // order is: Branch input -> Prompt textarea -> Cancel -> Dispatch (last,
    // enabled since branch+prompt are both prefilled).
    const branchInput = screen.getByTestId("jules-dispatch-branch");
    const dispatchButton = screen.getByTestId("jules-dispatch-submit");

    dispatchButton.focus();
    expect(document.activeElement).toBe(dispatchButton);

    fireEvent.keyDown(document, { key: "Tab" });
    expect(document.activeElement).toBe(branchInput);

    fireEvent.click(screen.getByTestId("jules-dispatch-cancel"));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(document.activeElement).toBe(screen.getByTestId("dispatch-opener"));
  });

  it("calls dispatchToJules with the trimmed payload and closes the dialog on success when egress is already acknowledged", async () => {
    dispatchToJules.mockResolvedValue({});
    const onClose = jest.fn();

    render(
      <JulesDispatchDialog
        itemId="item-42"
        itemTitle="Fix flaky poller test"
        acCriteria={AC_CRITERIA}
        repoPath={REPO_PATH}
        initialBranch="backlog/fix-flaky-poller-test"
        egressAcknowledged={true}
        onClose={onClose}
      />
    );

    fireEvent.change(screen.getByTestId("jules-dispatch-branch"), {
      target: { value: "  backlog/fix-flaky-poller-test  " },
    });
    fireEvent.change(screen.getByTestId("jules-dispatch-prompt"), {
      target: { value: "  Fix the flaky poller  " },
    });

    fireEvent.click(screen.getByTestId("jules-dispatch-submit"));

    await waitFor(() =>
      expect(dispatchToJules).toHaveBeenCalledWith({
        itemId: "item-42",
        branch: "backlog/fix-flaky-poller-test",
        prompt: "Fix the flaky poller",
      })
    );
    expect(confirmEgressConsent).not.toHaveBeenCalled();
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  });

  it("calls confirmEgressConsent before dispatchToJules when egress hasn't been acknowledged yet", async () => {
    const callOrder: string[] = [];
    confirmEgressConsent.mockImplementation(async () => {
      callOrder.push("confirmEgressConsent");
      return {};
    });
    dispatchToJules.mockImplementation(async () => {
      callOrder.push("dispatchToJules");
      return {};
    });
    const onClose = jest.fn();

    render(
      <JulesDispatchDialog
        itemId="item-1"
        itemTitle="Fix flaky poller test"
        acCriteria={AC_CRITERIA}
        repoPath={REPO_PATH}
        initialBranch="backlog/fix-flaky-poller-test"
        egressAcknowledged={false}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByTestId("jules-dispatch-egress-checkbox"));
    fireEvent.click(screen.getByTestId("jules-dispatch-submit"));

    await waitFor(() => expect(dispatchToJules).toHaveBeenCalled());
    expect(confirmEgressConsent).toHaveBeenCalledWith({ repoPath: REPO_PATH });
    expect(callOrder).toEqual(["confirmEgressConsent", "dispatchToJules"]);
  });

  it("shows an error message and keeps the dialog open when dispatchToJules rejects", async () => {
    dispatchToJules.mockRejectedValue(new Error("dispatch failed"));
    const onClose = jest.fn();

    render(
      <JulesDispatchDialog
        itemId="item-1"
        itemTitle="Fix flaky poller test"
        acCriteria={AC_CRITERIA}
        repoPath={REPO_PATH}
        initialBranch="backlog/fix-flaky-poller-test"
        egressAcknowledged={true}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByTestId("jules-dispatch-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("jules-dispatch-error")).toHaveTextContent("dispatch failed")
    );
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("shows an error message and keeps the dialog open when confirmEgressConsent rejects", async () => {
    confirmEgressConsent.mockRejectedValue(new Error("consent failed"));
    const onClose = jest.fn();

    render(
      <JulesDispatchDialog
        itemId="item-1"
        itemTitle="Fix flaky poller test"
        acCriteria={AC_CRITERIA}
        repoPath={REPO_PATH}
        initialBranch="backlog/fix-flaky-poller-test"
        egressAcknowledged={false}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByTestId("jules-dispatch-egress-checkbox"));
    fireEvent.click(screen.getByTestId("jules-dispatch-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("jules-dispatch-error")).toHaveTextContent("consent failed")
    );
    expect(dispatchToJules).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });
});

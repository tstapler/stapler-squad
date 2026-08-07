/**
 * Tests for TriggerFormModal (webhook-triggers Epic 7.1, Story 7.1.2).
 *
 * Covers: field-visibility-by-type, masked-secret-on-edit vs. show-once-on-create,
 * submit wiring, and inline backend-validation error mapping.
 */

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { TriggerFormModal } from "./TriggerFormModal";
import { WorkflowProto } from "@/gen/session/v1/session_pb";

function makeWorkflow(overrides: Partial<WorkflowProto> = {}): WorkflowProto {
  return {
    id: "wf-1",
    slug: "jira-ticket",
    name: "Triage tickets",
    description: "",
    command: "Triage the ticket",
    targetDirectory: "/repo",
    inputTemplate: "",
    sessionType: "directory",
    model: "",
    agentType: "",
    cronExpression: "",
    cronEnabled: true,
    triggerType: "webhook",
    githubRepo: "",
    githubBranch: "",
    webhookSlug: "jira-ticket",
    eventFilter: "issue_created",
    labelFilter: "urgent",
    promptTemplate: "Triage {{.issue.key}}",
    ...overrides,
  } as unknown as WorkflowProto;
}

describe("TriggerFormModal", () => {
  it("TriggerFormModal_should_showGithubPushFields_When_defaultTypeSelected", () => {
    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);

    expect(screen.getByTestId("trigger-github-repo-input")).toBeInTheDocument();
    expect(screen.getByTestId("trigger-github-branch-input")).toBeInTheDocument();
    expect(screen.queryByTestId("trigger-cron-expression-input")).not.toBeInTheDocument();
    expect(screen.queryByTestId("trigger-webhook-slug-input")).not.toBeInTheDocument();
  });

  it("TriggerFormModal_should_switchToCronFields_When_cronTypeSelected", () => {
    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-type-cron"));

    expect(screen.getByTestId("trigger-cron-expression-input")).toBeInTheDocument();
    expect(screen.queryByTestId("trigger-github-repo-input")).not.toBeInTheDocument();
    expect(screen.queryByTestId("trigger-webhook-slug-input")).not.toBeInTheDocument();
  });

  it("TriggerFormModal_should_switchToWebhookFields_When_webhookTypeSelected", () => {
    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-type-webhook"));

    expect(screen.getByTestId("trigger-webhook-slug-input")).toBeInTheDocument();
    expect(screen.getByTestId("trigger-event-filter-input")).toBeInTheDocument();
    expect(screen.getByTestId("trigger-label-filter-input")).toBeInTheDocument();
    expect(screen.queryByTestId("trigger-github-repo-input")).not.toBeInTheDocument();
    expect(screen.queryByTestId("trigger-cron-expression-input")).not.toBeInTheDocument();
  });

  it("TriggerFormModal_should_showGenerateSecretAffordance_When_creatingWebhookTrigger", () => {
    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);
    fireEvent.click(screen.getByTestId("trigger-type-webhook"));

    expect(screen.queryByTestId("trigger-secret-value")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("trigger-secret-generate"));
    expect(screen.getByTestId("trigger-secret-value")).toBeInTheDocument();
    expect(screen.getByTestId("trigger-secret-value").textContent).toMatch(/^[0-9a-f]{64}$/);
  });

  it("TriggerFormModal_should_showMaskedSecretPlaceholder_When_editingExistingWebhookTrigger", () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={jest.fn()} onClose={jest.fn()} />);

    expect(screen.getByTestId("trigger-secret-masked")).toHaveTextContent("•••• (unchanged)");
    expect(screen.queryByTestId("trigger-secret-generate")).not.toBeInTheDocument();
  });

  it("TriggerFormModal_should_disableTypeSwitch_When_editing", () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={jest.fn()} onClose={jest.fn()} />);

    expect(screen.getByTestId("trigger-type-webhook")).toBeDisabled();
    expect(screen.getByTestId("trigger-type-github_push")).toBeDisabled();
  });

  it("TriggerFormModal_should_prefillFieldsFromExistingTrigger_When_editing", () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={jest.fn()} onClose={jest.fn()} />);

    expect(screen.getByTestId("trigger-webhook-slug-input")).toHaveValue("jira-ticket");
    expect(screen.getByTestId("trigger-event-filter-input")).toHaveValue("issue_created");
    expect(screen.getByTestId("trigger-command-input")).toHaveValue("Triage the ticket");
  });

  it("TriggerFormModal_should_callOnSaveAndClose_When_validSubmission", async () => {
    const onSave = jest.fn().mockResolvedValue(undefined);
    const onClose = jest.fn();
    render(<TriggerFormModal open={true} onSave={onSave} onClose={onClose} />);

    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    fireEvent.change(screen.getByTestId("trigger-github-repo-input"), { target: { value: "owner/repo" } });

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(
      expect.objectContaining({
        slug: "new-trigger",
        name: "New Trigger",
        triggerType: "github_push",
        githubRepo: "owner/repo",
      })
    );
    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
  });

  it("TriggerFormModal_should_blockSubmit_When_requiredTypeSpecificFieldMissing", async () => {
    const onSave = jest.fn();
    render(<TriggerFormModal open={true} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    // githubRepo intentionally left blank

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    expect(onSave).not.toHaveBeenCalled();
  });

  it("TriggerFormModal_should_showInlineFieldError_When_backendRejectsWithFieldSpecificMessage", async () => {
    const onSave = jest.fn().mockRejectedValue(
      new Error('webhook_slug requires trigger_type="webhook", got trigger_type="github_push"')
    );
    render(<TriggerFormModal open={true} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    fireEvent.change(screen.getByTestId("trigger-github-repo-input"), { target: { value: "owner/repo" } });

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => {
      expect(screen.getAllByText(/webhook_slug requires trigger_type/).length).toBeGreaterThan(0);
    });
    // Form stays open — input value preserved, not cleared.
    expect(screen.getByTestId("trigger-slug-input")).toHaveValue("new-trigger");
  });

  it("TriggerFormModal_should_showFieldSpecificError_When_theRejectedFieldIsVisibleForCurrentType", async () => {
    const onSave = jest.fn().mockRejectedValue(new Error("invalid cron expression: expected 5 fields, found 1"));
    render(<TriggerFormModal open={true} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-type-cron"));
    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    fireEvent.change(screen.getByTestId("trigger-cron-expression-input"), { target: { value: "garbage" } });

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => {
      expect(screen.getAllByText(/invalid cron expression/).length).toBeGreaterThan(0);
    });
    // No generic top-of-form banner — the error attached to the cron field instead.
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  // ─── webhook_secret wiring (Phase 7 backend follow-up, Task 7.2) ──────────────

  it("TriggerFormModal_should_sendGeneratedSecret_When_creatingWebhookTrigger", async () => {
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<TriggerFormModal open={true} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-type-webhook"));
    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    fireEvent.change(screen.getByTestId("trigger-webhook-slug-input"), { target: { value: "wh-slug" } });
    fireEvent.click(screen.getByTestId("trigger-secret-generate"));
    const generated = screen.getByTestId("trigger-secret-value").textContent;

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ webhookSecret: generated }));
  });

  it("TriggerFormModal_should_sendEmptySecret_When_creatingWebhookTriggerWithoutGenerating", async () => {
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<TriggerFormModal open={true} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-type-webhook"));
    fireEvent.change(screen.getByTestId("trigger-slug-input"), { target: { value: "new-trigger" } });
    fireEvent.change(screen.getByTestId("trigger-name-input"), { target: { value: "New Trigger" } });
    fireEvent.change(screen.getByTestId("trigger-target-directory-input"), { target: { value: "/repo" } });
    fireEvent.change(screen.getByTestId("trigger-command-input"), { target: { value: "Do the thing" } });
    fireEvent.change(screen.getByTestId("trigger-webhook-slug-input"), { target: { value: "wh-slug" } });

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ webhookSecret: "" }));
  });

  it("TriggerFormModal_should_sendEmptySecret_When_editingWithoutRotating", async () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ webhookSecret: "" }));
  });

  it("TriggerFormModal_should_sendRotatedSecret_When_editingAfterClickingRotate", async () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={onSave} onClose={jest.fn()} />);

    expect(screen.queryByTestId("trigger-secret-generate")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("trigger-secret-rotate"));
    expect(screen.getByTestId("trigger-secret-generate")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("trigger-secret-generate"));
    const generated = screen.getByTestId("trigger-secret-value").textContent;

    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
    expect(onSave).toHaveBeenCalledWith(expect.objectContaining({ webhookSecret: generated }));
  });

  // ─── clipboard-copy correctness (Fix 1) ───────────────────────────────────

  it("TriggerFormModal_should_showCopiedOnlyOnGenuineSuccess_When_clipboardWriteSucceeds", async () => {
    const writeText = jest.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);
    fireEvent.click(screen.getByTestId("trigger-type-webhook"));
    fireEvent.click(screen.getByTestId("trigger-secret-generate"));

    fireEvent.click(screen.getByTestId("trigger-secret-copy"));

    await waitFor(() => expect(screen.getByTestId("trigger-secret-copy")).toHaveTextContent("Copied ✓"));
    expect(screen.queryByTestId("trigger-secret-copy-error")).not.toBeInTheDocument();
  });

  it("TriggerFormModal_should_notReportCopiedAndShowFallback_When_clipboardWriteFails", async () => {
    const writeText = jest.fn().mockRejectedValue(new Error("permission denied"));
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });

    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);
    fireEvent.click(screen.getByTestId("trigger-type-webhook"));
    fireEvent.click(screen.getByTestId("trigger-secret-generate"));

    fireEvent.click(screen.getByTestId("trigger-secret-copy"));

    await waitFor(() => expect(screen.getByTestId("trigger-secret-copy-error")).toBeInTheDocument());
    // Never reports success when the write actually failed.
    expect(screen.getByTestId("trigger-secret-copy")).not.toHaveTextContent("Copied ✓");
    // Secret text remains visible/focusable for manual copy.
    expect(screen.getByTestId("trigger-secret-value")).toHaveAttribute("tabIndex", "0");
  });

  it("TriggerFormModal_should_notReportCopiedAndShowFallback_When_clipboardApiUnavailable", async () => {
    Object.defineProperty(navigator, "clipboard", { value: undefined, configurable: true });

    render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);
    fireEvent.click(screen.getByTestId("trigger-type-webhook"));
    fireEvent.click(screen.getByTestId("trigger-secret-generate"));

    fireEvent.click(screen.getByTestId("trigger-secret-copy"));

    await waitFor(() => expect(screen.getByTestId("trigger-secret-copy-error")).toBeInTheDocument());
    expect(screen.getByTestId("trigger-secret-copy")).not.toHaveTextContent("Copied ✓");
  });

  // ─── rotate-without-secret validation (Fix 8) ─────────────────────────────

  it("TriggerFormModal_should_blockSubmitWithValidationError_When_rotatingWithoutGeneratingSecret", async () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    const onSave = jest.fn();
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-secret-rotate"));
    fireEvent.click(screen.getByTestId("trigger-form-submit"));

    expect(onSave).not.toHaveBeenCalled();
    expect(screen.getByTestId("trigger-secret-field-error")).toHaveTextContent(
      "Generate a secret before saving."
    );
  });

  it("TriggerFormModal_should_clearRotateValidationError_When_secretGeneratedAfterFailedSubmit", async () => {
    const existing = makeWorkflow({ triggerType: "webhook" });
    const onSave = jest.fn().mockResolvedValue(undefined);
    render(<TriggerFormModal open={true} editTrigger={existing} onSave={onSave} onClose={jest.fn()} />);

    fireEvent.click(screen.getByTestId("trigger-secret-rotate"));
    fireEvent.click(screen.getByTestId("trigger-form-submit"));
    expect(screen.getByTestId("trigger-secret-field-error")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("trigger-secret-generate"));
    expect(screen.queryByTestId("trigger-secret-field-error")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("trigger-form-submit"));
    await waitFor(() => expect(onSave).toHaveBeenCalledTimes(1));
  });

  // ─── secret-generation availability (Fix 6) ───────────────────────────────

  it("TriggerFormModal_should_disableGenerateAndShowMessage_When_webCryptoUnavailable", () => {
    const originalCrypto = globalThis.crypto;
    // @ts-expect-error - simulating an environment without the Web Crypto API
    delete globalThis.crypto;

    try {
      render(<TriggerFormModal open={true} onSave={jest.fn()} onClose={jest.fn()} />);
      fireEvent.click(screen.getByTestId("trigger-type-webhook"));
      fireEvent.click(screen.getByTestId("trigger-secret-generate"));

      expect(screen.getByTestId("trigger-secret-generate")).toBeDisabled();
      expect(screen.queryByTestId("trigger-secret-value")).not.toBeInTheDocument();
      expect(screen.getByText(/requires a modern browser/)).toBeInTheDocument();
    } finally {
      Object.defineProperty(globalThis, "crypto", { value: originalCrypto, configurable: true });
    }
  });
});

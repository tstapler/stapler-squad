/**
 * Tests for AddRemoteForm + HostKeyTrustDialog — ssh-remote-workspaces
 * Phase 6, Epic 6.1, Task 6.1.1g.
 *
 * Covers:
 *  1. Add flow: an immediately-successful TestRemoteConnection (known host)
 *     persists the remote via CreateRemote and calls onSaved -- no dialog.
 *  2. Trust flow: a host_key_unknown response opens HostKeyTrustDialog with
 *     the fingerprint; "Trust and connect" calls TrustRemoteHostKey then
 *     CreateRemote, and onSaved fires with the saved remote.
 *  3. Cancel flow: cancelling the HostKeyTrustDialog never calls CreateRemote
 *     (remote stays unsaved), closes the dialog, leaves the Add Remote form
 *     mounted and editable with its entered values intact, and returns focus
 *     to the triggering "Test connection" button.
 */

import React from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AddRemoteForm } from "./AddRemoteForm";
import { useRemotesService } from "@/lib/hooks/useRemotesService";
import type { RemoteConfigInfo } from "@/lib/hooks/useRemotesService";

jest.mock("@/lib/hooks/useRemotesService", () => ({
  useRemotesService: jest.fn(),
}));

const mockUseRemotesService = useRemotesService as jest.MockedFunction<typeof useRemotesService>;

const mockCreateRemote = jest.fn();
const mockDeleteRemote = jest.fn();
const mockGenerateRemoteIdentity = jest.fn();
const mockTestRemoteConnectionDraft = jest.fn();
const mockTrustRemoteHostKeyDraft = jest.fn();

function makeRemote(overrides: Partial<RemoteConfigInfo> = {}): RemoteConfigInfo {
  return {
    $typeName: "session.v1.RemoteConfigProto",
    name: "prod-box",
    host: "prod.example.com",
    user: "tyler",
    port: 0,
    basePath: "/srv/workspaces",
    hasIdentity: true,
    ...overrides,
  } as RemoteConfigInfo;
}

// The jest styleMock for `.css.ts` files wraps every export in a callable
// proxy, which triggers a benign "Invalid value for prop className" React
// warning -- same pre-existing jest/vanilla-extract mock limitation
// documented in PipelineModeForm.test.tsx.
beforeAll(() => {
  jest.spyOn(console, "error").mockImplementation(() => {});
});

afterAll(() => {
  jest.restoreAllMocks();
});

beforeEach(() => {
  jest.clearAllMocks();
  mockGenerateRemoteIdentity.mockResolvedValue({
    publicKeyText: "ssh-ed25519 AAAAtest prod-box",
    authorizedKeysLine: 'command="/path/to/stapler-squad-ssh-wrapper.sh",restrict,pty ssh-ed25519 AAAAtest prod-box',
  });
  mockUseRemotesService.mockReturnValue({
    listRemotes: jest.fn(),
    createRemote: mockCreateRemote,
    deleteRemote: mockDeleteRemote,
    generateRemoteIdentity: mockGenerateRemoteIdentity,
    testRemoteConnectionDraft: mockTestRemoteConnectionDraft,
    testRemoteConnectionSaved: jest.fn(),
    trustRemoteHostKeyDraft: mockTrustRemoteHostKeyDraft,
    trustRemoteHostKeySaved: jest.fn(),
  } as unknown as ReturnType<typeof useRemotesService>);
});

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByTestId("add-remote-name"), "prod-box");
  await user.type(screen.getByTestId("add-remote-host"), "prod.example.com");
  await user.type(screen.getByTestId("add-remote-user"), "tyler");
  await user.type(screen.getByTestId("add-remote-base-path"), "/srv/workspaces");
}

describe("AddRemoteForm", () => {
  it("add flow: an immediately-successful test connection persists the remote and calls onSaved", async () => {
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: true,
      hostKeyUnknown: false,
      fingerprint: "",
      errorMessage: "",
    });
    const saved = makeRemote();
    mockCreateRemote.mockResolvedValue(saved);
    const onSaved = jest.fn();
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={onSaved} onCancel={jest.fn()} />);
    await fillRequiredFields(user);
    await user.click(screen.getByTestId("add-remote-submit"));

    await waitFor(() => expect(mockGenerateRemoteIdentity).toHaveBeenCalledWith("prod-box"));
    await waitFor(() =>
      expect(mockTestRemoteConnectionDraft).toHaveBeenCalledWith({
        name: "prod-box",
        host: "prod.example.com",
        user: "tyler",
        port: 0,
      })
    );
    await waitFor(() =>
      expect(mockCreateRemote).toHaveBeenCalledWith({
        name: "prod-box",
        host: "prod.example.com",
        user: "tyler",
        port: 0,
        basePath: "/srv/workspaces",
      })
    );
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
    expect(screen.queryByTestId("host-key-trust-overlay")).not.toBeInTheDocument();
  });

  it("trust flow: host_key_unknown opens the dialog; Trust and connect trusts then creates the remote", async () => {
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: false,
      hostKeyUnknown: true,
      fingerprint: "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k",
      errorMessage: "",
    });
    mockTrustRemoteHostKeyDraft.mockResolvedValue({ success: true, errorMessage: "" });
    const saved = makeRemote();
    mockCreateRemote.mockResolvedValue(saved);
    const onSaved = jest.fn();
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={onSaved} onCancel={jest.fn()} />);
    await fillRequiredFields(user);
    await user.click(screen.getByTestId("add-remote-submit"));

    const dialog = await screen.findByTestId("host-key-trust-overlay");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByTestId("host-key-trust-fingerprint")).toHaveTextContent(
      "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k"
    );
    // Focus lands on Cancel by default, never Trust -- a stray Enter must
    // not silently trust an unverified host (ux.md Surface 3 step 1).
    expect(screen.getByTestId("host-key-trust-cancel")).toHaveFocus();
    expect(mockCreateRemote).not.toHaveBeenCalled();

    await user.click(screen.getByTestId("host-key-trust-confirm"));

    await waitFor(() =>
      expect(mockTrustRemoteHostKeyDraft).toHaveBeenCalledWith(
        { name: "prod-box", host: "prod.example.com", user: "tyler", port: 0 },
        "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k"
      )
    );
    await waitFor(() => expect(mockCreateRemote).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
    expect(screen.queryByTestId("host-key-trust-overlay")).not.toBeInTheDocument();
  });

  it("regression: locks every field once an identity is generated, so a rename can't silently orphan the shown authorized_keys line", async () => {
    // Fix-first review finding: identity is generated and keyed by `name`.
    // If Name (or any field) stayed editable after generation, a rename
    // followed by Create would call CreateRemote with a NEW name that has
    // no stored identity -- its server-side GenerateOrDescribeIdentity
    // fallback would then silently mint a FRESH, different keypair than the
    // one actually shown/pasted by the user, and the saved remote's stored
    // identity would never match what's in the remote's authorized_keys.
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: false,
      hostKeyUnknown: true,
      fingerprint: "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k",
      errorMessage: "",
    });
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={jest.fn()} onCancel={jest.fn()} />);
    await fillRequiredFields(user);
    await user.click(screen.getByTestId("add-remote-submit"));
    await screen.findByTestId("host-key-trust-overlay");

    // Identity now exists (its authorized_keys block is on screen) -- every
    // field must be structurally locked, not just visually.
    expect(screen.getByTestId("add-remote-authorized-keys")).toBeInTheDocument();
    expect(screen.getByTestId("add-remote-name")).toBeDisabled();
    expect(screen.getByTestId("add-remote-host")).toBeDisabled();
    expect(screen.getByTestId("add-remote-user")).toBeDisabled();
    expect(screen.getByTestId("add-remote-port")).toBeDisabled();
    expect(screen.getByTestId("add-remote-base-path")).toBeDisabled();

    // A real user interaction can't type into a disabled field -- the value
    // must stay exactly what was submitted, so a subsequent retry/Create can
    // never resolve to a different, unsaved identity.
    await user.type(screen.getByTestId("add-remote-name"), "renamed-box").catch(() => {});
    expect(screen.getByTestId("add-remote-name")).toHaveValue("prod-box");

    // Only ONE identity was ever generated for this form session -- the
    // exact scenario a bypassed lock would have doubled.
    expect(mockGenerateRemoteIdentity).toHaveBeenCalledTimes(1);
    expect(mockGenerateRemoteIdentity).toHaveBeenCalledWith("prod-box");
  });

  it("cancel flow: cancelling the trust dialog leaves the remote unsaved, the form editable, and returns focus to Test connection", async () => {
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: false,
      hostKeyUnknown: true,
      fingerprint: "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k",
      errorMessage: "",
    });
    const onSaved = jest.fn();
    const onCancel = jest.fn();
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={onSaved} onCancel={onCancel} />);
    await fillRequiredFields(user);
    const submitButton = screen.getByTestId("add-remote-submit");
    await user.click(submitButton);

    await screen.findByTestId("host-key-trust-overlay");
    await user.click(screen.getByTestId("host-key-trust-cancel"));

    // Dialog closed; nothing was ever persisted or trusted.
    expect(screen.queryByTestId("host-key-trust-overlay")).not.toBeInTheDocument();
    expect(mockTrustRemoteHostKeyDraft).not.toHaveBeenCalled();
    expect(mockCreateRemote).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
    // The Add Remote form itself was never told to close -- Cancel on the
    // dialog is distinct from Cancel on the whole form.
    expect(onCancel).not.toHaveBeenCalled();

    // Form remains mounted and the entered values are intact -- "editable"
    // here means the user can still act (retry Test connection, or Cancel
    // the whole form), not that the raw fields are unlocked: an identity
    // was already generated for "prod-box" before the dialog opened, and it
    // stays generated after Cancel (so a retry reuses the same key -- see
    // GenerateRemoteIdentity's idempotency) -- so per the fix-first review,
    // fields stay LOCKED here too. Only the whole-form Cancel (tested below)
    // clears the identity and unlocks a fresh attempt.
    expect(screen.getByTestId("add-remote-form")).toBeInTheDocument();
    expect(screen.getByTestId("add-remote-name")).toHaveValue("prod-box");
    expect(screen.getByTestId("add-remote-name")).toBeDisabled();
    expect(screen.getByTestId("add-remote-host")).toHaveValue("prod.example.com");

    // Focus returns to the control that triggered the dialog.
    await waitFor(() => expect(submitButton).toHaveFocus());
  });

  it("regression: cancelling the trust dialog while Trust and connect is still in flight never persists the remote once it resolves", async () => {
    // Fix-first review finding (final sdd:6-verify holistic pass, MUST FIX
    // 2): handleTrustCancel only cleared pendingFingerprint -- it never
    // signaled the in-flight handleTrust call, which unconditionally
    // persisted the remote and fired onSaved once trustRemoteHostKeyDraft
    // resolved, even if the user had already clicked Cancel. The user sees
    // the dialog close, believing nothing happened, while the remote is
    // created behind their back moments later.
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: false,
      hostKeyUnknown: true,
      fingerprint: "SHA256:k3jd93kfDJKS93kdjfKSJDF93kdjf93k",
      errorMessage: "",
    });
    let resolveTrust: (value: { success: boolean; errorMessage: string }) => void = () => {};
    mockTrustRemoteHostKeyDraft.mockReturnValue(
      new Promise((resolve) => {
        resolveTrust = resolve;
      })
    );
    const onSaved = jest.fn();
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={onSaved} onCancel={jest.fn()} />);
    await fillRequiredFields(user);
    await user.click(screen.getByTestId("add-remote-submit"));
    await screen.findByTestId("host-key-trust-overlay");

    // Start the trust RPC (it never resolves until we say so below), then
    // cancel while it's still pending.
    await user.click(screen.getByTestId("host-key-trust-confirm"));
    await waitFor(() => expect(mockTrustRemoteHostKeyDraft).toHaveBeenCalledTimes(1));
    await user.click(screen.getByTestId("host-key-trust-cancel"));
    expect(screen.queryByTestId("host-key-trust-overlay")).not.toBeInTheDocument();

    // Now let the RPC resolve successfully, well after Cancel, and flush the
    // resulting handleTrust continuation (await -> cancellation check ->
    // any setState calls) before asserting.
    await act(async () => {
      resolveTrust({ success: true, errorMessage: "" });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCreateRemote).not.toHaveBeenCalled();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("form Cancel: with an identity already generated, deletes the orphaned identity before calling onCancel", async () => {
    mockTestRemoteConnectionDraft.mockResolvedValue({
      success: false,
      hostKeyUnknown: false,
      fingerprint: "",
      errorMessage: "Couldn't reach prod-box.",
    });
    mockDeleteRemote.mockResolvedValue(undefined);
    const onCancel = jest.fn();
    const user = userEvent.setup();

    render(<AddRemoteForm onSaved={jest.fn()} onCancel={onCancel} />);
    await fillRequiredFields(user);
    await user.click(screen.getByTestId("add-remote-submit"));

    // Identity was generated (first Test connection attempt) even though
    // the dial itself failed.
    await waitFor(() => expect(mockGenerateRemoteIdentity).toHaveBeenCalled());
    await screen.findByTestId("add-remote-error");

    await user.click(screen.getByTestId("add-remote-cancel"));

    await waitFor(() => expect(mockDeleteRemote).toHaveBeenCalledWith("prod-box"));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});

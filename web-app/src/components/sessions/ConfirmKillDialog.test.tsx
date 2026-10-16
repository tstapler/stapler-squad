import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { ConfirmKillDialog } from "./ConfirmKillDialog";
import { KillStatus, type PIDIdentity } from "@/gen/session/v1/import_pb";

jest.mock("./ConfirmKillDialog.css", () =>
  new Proxy(
    {},
    {
      get: (_target, key) => (typeof key === "string" ? key : ""),
    }
  )
);

jest.mock("@/lib/hooks/useFocusTrap", () => ({
  useFocusTrap: jest.fn(),
}));

const mockConfirmKill = jest.fn();
const mockCancelPendingKill = jest.fn();

jest.mock("@/lib/hooks/useImportSessionService", () => ({
  useImportSessionService: () => ({
    confirmKill: mockConfirmKill,
    cancelPendingKill: mockCancelPendingKill,
  }),
}));

function pidIdentity(): PIDIdentity {
  return { pid: 4321, createTimeMs: BigInt(1000) } as PIDIdentity;
}

describe("ConfirmKillDialog", () => {
  beforeEach(() => {
    mockConfirmKill.mockReset();
    mockCancelPendingKill.mockReset();
  });

  it("focuses the Cancel button by default, not the destructive Confirm Kill action", () => {
    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={jest.fn()}
        onClose={jest.fn()}
      />
    );

    const buttons = screen.getAllByRole("button");
    expect(buttons[0]).toHaveTextContent("Cancel");
    expect(buttons[1]).toHaveTextContent("Confirm Kill");
  });

  it("transitions to imported and closes when confirmKill reports KILLED", async () => {
    mockConfirmKill.mockResolvedValue({ status: KillStatus.KILLED, error: "" });
    const onStatusChange = jest.fn();
    const onClose = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Confirm Kill" }));

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith("imported");
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mockConfirmKill).toHaveBeenCalledWith({
      instanceId: "inst-1",
      pidIdentity: pidIdentity(),
    });
  });

  it("transitions to imported and closes when confirmKill reports ALREADY_GONE", async () => {
    mockConfirmKill.mockResolvedValue({ status: KillStatus.ALREADY_GONE, error: "" });
    const onStatusChange = jest.fn();
    const onClose = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Confirm Kill" }));

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith("imported");
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not regress row status when confirmKill reports FAILED, and keeps the dialog open", async () => {
    mockConfirmKill.mockResolvedValue({
      status: KillStatus.FAILED,
      error: "tmux kill-session failed",
    });
    const onStatusChange = jest.fn();
    const onClose = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Confirm Kill" }));

    expect(await screen.findByText("tmux kill-session failed")).toBeInTheDocument();
    expect(onStatusChange).toHaveBeenCalledWith("kill_failed");
    expect(onStatusChange).not.toHaveBeenCalledWith("imported");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("treats a null confirmKill result the same as a kill failure", async () => {
    mockConfirmKill.mockResolvedValue(null);
    const onStatusChange = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={jest.fn()}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Confirm Kill" }));

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith("kill_failed");
    });
  });

  it("transitions to reverted and closes when cancelPendingKill reports resumed=true", async () => {
    mockCancelPendingKill.mockResolvedValue({ resumed: true, error: "" });
    const onStatusChange = jest.fn();
    const onClose = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    await waitFor(() => {
      expect(onStatusChange).toHaveBeenCalledWith("reverted");
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mockCancelPendingKill).toHaveBeenCalledWith({
      instanceId: "inst-1",
      pidIdentity: pidIdentity(),
    });
  });

  it("leaves the row pending and shows an error when cancelPendingKill reports resumed=false", async () => {
    mockCancelPendingKill.mockResolvedValue({
      resumed: false,
      error: "compensating delete failed",
    });
    const onStatusChange = jest.fn();
    const onClose = jest.fn();

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={onStatusChange}
        onClose={onClose}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(await screen.findByText("compensating delete failed")).toBeInTheDocument();
    expect(onStatusChange).toHaveBeenCalledWith("committed_pending_kill");
    expect(onStatusChange).not.toHaveBeenCalledWith("reverted");
    expect(onClose).not.toHaveBeenCalled();
  });

  it("disables both actions while a kill or cancel request is in flight", async () => {
    mockConfirmKill.mockReturnValue(new Promise(() => {}));

    render(
      <ConfirmKillDialog
        instanceId="inst-1"
        pidIdentity={pidIdentity()}
        program="claude"
        onStatusChange={jest.fn()}
        onClose={jest.fn()}
      />
    );

    fireEvent.click(screen.getByRole("button", { name: "Confirm Kill" }));

    expect(await screen.findByRole("button", { name: "Killing..." })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
  });
});

import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { WorkflowForm } from "./WorkflowForm";

// RepoPathInput pulls in a Redux-backed hook (useSessionRepoPaths) for path history/
// autocomplete that isn't relevant to the cron submit-guard behavior under test here —
// swap it for a plain input so this test doesn't need a Redux <Provider>.
jest.mock("@/components/ui/RepoPathInput", () => ({
  RepoPathInput: ({ id, value, onChange, placeholder, required }: {
    id?: string;
    value: string;
    onChange: (v: string) => void;
    placeholder?: string;
    required?: boolean;
  }) => (
    <input id={id} value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} required={required} />
  ),
}));

function fillRequiredFields() {
  fireEvent.change(screen.getByLabelText("Slug", { exact: false }), { target: { value: "my-workflow" } });
  fireEvent.change(screen.getByLabelText("Name", { exact: false }), { target: { value: "My Workflow" } });
  fireEvent.change(screen.getByLabelText("Command / Prompt", { exact: false }), { target: { value: "echo hi" } });
  fireEvent.change(screen.getByLabelText("Target Directory", { exact: false }), { target: { value: "/tmp" } });
}

describe("WorkflowForm cron submit guard", () => {
  it("blocks submission and shows an error when the cron field holds an invalid expression", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(<WorkflowForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    fillRequiredFields();
    fireEvent.click(screen.getByLabelText("Advanced"));
    fireEvent.change(screen.getByLabelText("Advanced cron expression"), { target: { value: "99 9 * * *" } });

    fireEvent.click(screen.getByRole("button", { name: "Create Workflow" }));

    await waitFor(() => {
      expect(screen.getByText(/Cron expression:/)).toBeInTheDocument();
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("submits normally when the cron field is empty", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(<WorkflowForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    fillRequiredFields();
    // Simple mode defaults to a valid cron string; switch to Advanced and clear it
    // to exercise the "empty cron is optional" path specifically.
    fireEvent.click(screen.getByLabelText("Advanced"));
    fireEvent.change(screen.getByLabelText("Advanced cron expression"), { target: { value: "" } });

    fireEvent.click(screen.getByRole("button", { name: "Create Workflow" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
    expect(screen.queryByText(/Cron expression:/)).not.toBeInTheDocument();
  });

  it("submits once a previously invalid cron expression is corrected", async () => {
    const onSubmit = jest.fn().mockResolvedValue(undefined);
    render(<WorkflowForm onSubmit={onSubmit} onCancel={jest.fn()} />);

    fillRequiredFields();
    fireEvent.click(screen.getByLabelText("Advanced"));
    const cronInput = screen.getByLabelText("Advanced cron expression");
    fireEvent.change(cronInput, { target: { value: "99 9 * * *" } });
    fireEvent.click(screen.getByRole("button", { name: "Create Workflow" }));
    await waitFor(() => {
      expect(screen.getByText(/Cron expression:/)).toBeInTheDocument();
    });

    fireEvent.change(cronInput, { target: { value: "0 9 * * *" } });
    fireEvent.click(screen.getByRole("button", { name: "Create Workflow" }));

    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalledTimes(1);
    });
  });
});

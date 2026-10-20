import React, { useState } from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { GoToLineDialog } from "../files/GoToLineDialog";

function Host({ onSubmit }: { onSubmit: (n: number) => void }) {
  const [open, setOpen] = useState(false);
  return (
    <div>
      <button onClick={() => setOpen(true)}>invoke</button>
      {open && <GoToLineDialog onSubmit={(n) => { onSubmit(n); setOpen(false); }} onClose={() => setOpen(false)} />}
    </div>
  );
}

describe("GoToLineDialog", () => {
  it("goToLineDialog_should_SubmitNumber_And_IgnoreGarbage_And_RestoreFocus", async () => {
    const onSubmit = jest.fn();
    render(<Host onSubmit={onSubmit} />);
    const user = userEvent.setup();
    const invoker = screen.getByText("invoke");
    await user.click(invoker);
    const input = screen.getByRole("textbox", { name: "Go to line" });
    expect(input).toHaveFocus();
    await user.type(input, "abc{Enter}");
    expect(onSubmit).not.toHaveBeenCalled();
    await user.clear(input);
    await user.type(input, ":250{Enter}");
    expect(onSubmit).toHaveBeenCalledWith(250);
    expect(invoker).toHaveFocus();
    await user.click(invoker);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(invoker).toHaveFocus();
  });
});

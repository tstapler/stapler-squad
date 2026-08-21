import React from "react";
import { render, screen } from "@testing-library/react";
import { Input } from "./Input";

describe("Input", () => {
  it("renders an input element", () => {
    render(<Input id="test" />);
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("renders a label when label prop is provided", () => {
    render(<Input id="email" label="Email address" />);
    expect(screen.getByLabelText("Email address")).toBeInTheDocument();
  });

  it("renders error message when error prop is provided", () => {
    render(<Input id="email" label="Email" error="Email is required" />);
    expect(screen.getByText("Email is required")).toBeInTheDocument();
  });

  it("marks input aria-invalid when error prop is provided", () => {
    render(<Input id="email" label="Email" error="Required" />);
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "true");
  });

  it("associates error message with input via aria-describedby", () => {
    render(<Input id="email" label="Email" error="Required" />);
    const input = screen.getByLabelText("Email");
    expect(input).toHaveAttribute("aria-describedby", "email-error");
    expect(screen.getByText("Required")).toHaveAttribute("id", "email-error");
  });

  it("does not set aria-invalid when no error", () => {
    render(<Input id="email" label="Email" />);
    expect(screen.getByLabelText("Email")).toHaveAttribute("aria-invalid", "false");
  });

  it("forwards ref to the input element", () => {
    const ref = React.createRef<HTMLInputElement>();
    render(<Input ref={ref} id="test" />);
    expect(ref.current).toBeInstanceOf(HTMLInputElement);
  });

  it.each(["sm", "md", "lg"] as const)("renders inputSize=%s without error", (size) => {
    render(<Input id="test" inputSize={size} />);
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });
});

import React from "react";
import { render, screen } from "@testing-library/react";
import { Badge } from "./Badge";

describe("Badge", () => {
  it("renders children", () => {
    render(<Badge>Active</Badge>);
    expect(screen.getByText("Active")).toBeInTheDocument();
  });

  it("renders as a span element", () => {
    render(<Badge>Label</Badge>);
    expect(screen.getByText("Label").tagName).toBe("SPAN");
  });

  it.each(["default", "success", "warning", "error", "primary"] as const)(
    "renders intent=%s without error",
    (intent) => {
      render(<Badge intent={intent}>Label</Badge>);
      expect(screen.getByText("Label")).toBeInTheDocument();
    }
  );

  it.each(["sm", "md"] as const)("renders size=%s without error", (size) => {
    render(<Badge size={size}>Label</Badge>);
    expect(screen.getByText("Label")).toBeInTheDocument();
  });

  it("passes additional HTML attributes", () => {
    render(<Badge data-testid="my-badge">Status</Badge>);
    expect(screen.getByTestId("my-badge")).toBeInTheDocument();
  });

});

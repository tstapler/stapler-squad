import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { NavBadge } from "./NavBadge";

describe("NavBadge", () => {
  describe("visibility", () => {
    it("renders null when count=0 and showWhenEmpty is false (default)", () => {
      const { container } = render(<NavBadge element="span" count={0} />);
      expect(container.firstChild).toBeNull();
    });

    it("renders when count=0 and showWhenEmpty=true", () => {
      render(<NavBadge element="span" count={0} showWhenEmpty />);
      expect(screen.getByText("0")).toBeInTheDocument();
    });

    it("renders the count when count > 0", () => {
      render(<NavBadge element="span" count={5} />);
      expect(screen.getByText("5")).toBeInTheDocument();
    });

    it("caps display at 99+ when count > 99", () => {
      render(<NavBadge element="span" count={100} />);
      expect(screen.getByText("99+")).toBeInTheDocument();
    });

    it("shows exactly 99 (no cap) when count=99", () => {
      render(<NavBadge element="span" count={99} />);
      expect(screen.getByText("99")).toBeInTheDocument();
    });
  });

  describe("element=button", () => {
    it("renders a <button> element", () => {
      render(<NavBadge element="button" count={3} />);
      expect(screen.getByRole("button")).toBeInTheDocument();
    });

    it("forwards onClick to the button", () => {
      const onClick = jest.fn();
      render(<NavBadge element="button" count={3} onClick={onClick} />);
      fireEvent.click(screen.getByRole("button"));
      expect(onClick).toHaveBeenCalledTimes(1);
    });

    it("forwards aria-label to the button", () => {
      render(<NavBadge element="button" count={3} aria-label="Open approvals" />);
      expect(screen.getByRole("button", { name: "Open approvals" })).toBeInTheDocument();
    });
  });

  describe("element=span", () => {
    it("renders a <span> element (not a button)", () => {
      render(<NavBadge element="span" count={3} />);
      expect(screen.queryByRole("button")).toBeNull();
      expect(screen.getByText("3")).toBeInTheDocument();
    });

    it("forwards className and aria-hidden to the span", () => {
      const { container } = render(
        <NavBadge element="span" count={3} aria-hidden="true" />
      );
      const span = container.querySelector("span");
      expect(span).toHaveAttribute("aria-hidden", "true");
    });
  });
});

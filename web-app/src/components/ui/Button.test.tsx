import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { Button } from "./Button";

describe("Button", () => {
  it("renders with role button", () => {
    render(<Button>Click me</Button>);
    expect(screen.getByRole("button", { name: "Click me" })).toBeInTheDocument();
  });

  it("fires onClick when clicked", () => {
    const onClick = jest.fn();
    render(<Button onClick={onClick}>Click me</Button>);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it.each(["primary", "secondary", "danger", "ghost"] as const)(
    "renders intent=%s without error",
    (intent) => {
      render(<Button intent={intent}>Button</Button>);
      expect(screen.getByRole("button")).toBeInTheDocument();
    }
  );

  it.each(["sm", "md", "lg"] as const)("renders size=%s without error", (size) => {
    render(<Button size={size}>Button</Button>);
    expect(screen.getByRole("button")).toBeInTheDocument();
  });

  it("is disabled when disabled prop is passed", () => {
    render(<Button disabled>Button</Button>);
    expect(screen.getByRole("button")).toBeDisabled();
  });

  it("forwards ref to the button element", () => {
    const ref = React.createRef<HTMLButtonElement>();
    render(<Button ref={ref}>Button</Button>);
    expect(ref.current).toBeInstanceOf(HTMLButtonElement);
  });

  it("renders child element via asChild", () => {
    render(
      <Button asChild>
        <a href="/test">Link Button</a>
      </Button>
    );
    expect(screen.getByRole("link", { name: "Link Button" })).toBeInTheDocument();
  });

  it("does not fire onClick when disabled", () => {
    const onClick = jest.fn();
    render(<Button disabled onClick={onClick}>Button</Button>);
    fireEvent.click(screen.getByRole("button"));
    expect(onClick).not.toHaveBeenCalled();
  });
});

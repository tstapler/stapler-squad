import React from "react";
import { render, screen } from "@testing-library/react";
import { Card, CardHeader, CardTitle, CardDescription, CardFooter } from "./Card";

describe("Card", () => {
  it("renders children", () => {
    render(<Card>Card content</Card>);
    expect(screen.getByText("Card content")).toBeInTheDocument();
  });

  it.each(["default", "elevated", "bordered", "interactive"] as const)(
    "renders variant=%s without error",
    (variant) => {
      render(<Card variant={variant}>Content</Card>);
      expect(screen.getByText("Content")).toBeInTheDocument();
    }
  );

  it.each(["none", "sm", "md", "lg"] as const)(
    "renders padding=%s variant without error",
    (padding) => {
      render(<Card padding={padding}>Content</Card>);
      expect(screen.getByText("Content")).toBeInTheDocument();
    }
  );

  it("renders CardHeader with children", () => {
    render(<CardHeader>Header content</CardHeader>);
    expect(screen.getByText("Header content")).toBeInTheDocument();
  });

  it("renders CardTitle", () => {
    render(<CardTitle>My Title</CardTitle>);
    expect(screen.getByText("My Title")).toBeInTheDocument();
  });

  it("renders CardDescription", () => {
    render(<CardDescription>Description text</CardDescription>);
    expect(screen.getByText("Description text")).toBeInTheDocument();
  });

  it("renders CardFooter", () => {
    render(<CardFooter>Footer content</CardFooter>);
    expect(screen.getByText("Footer content")).toBeInTheDocument();
  });

  it("renders composed card layout", () => {
    render(
      <Card data-testid="card">
        <CardHeader>
          <CardTitle>Session Active</CardTitle>
          <CardDescription>Running since 2 hours ago</CardDescription>
        </CardHeader>
        <CardFooter>Actions</CardFooter>
      </Card>
    );
    expect(screen.getByTestId("card")).toBeInTheDocument();
    expect(screen.getByText("Session Active")).toBeInTheDocument();
    expect(screen.getByText("Running since 2 hours ago")).toBeInTheDocument();
    expect(screen.getByText("Actions")).toBeInTheDocument();
  });
});

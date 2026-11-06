// +feature: ui-error-boundary
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { ErrorBoundary } from "./ErrorBoundary";

// Catches render errors in its subtree and shows ErrorState (or a custom fallback).
const meta: Meta<typeof ErrorBoundary> = {
  component: ErrorBoundary,
  title: "UI/ErrorBoundary",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof ErrorBoundary>;

function Bomb({ message = "Boom: render failed" }: { message?: string }): never {
  throw new Error(message);
}

function Healthy() {
  return <p>Child rendered fine.</p>;
}

function RecoverableDemo() {
  const [armed, setArmed] = useState(true);
  return (
    <ErrorBoundary fallback={<button onClick={() => setArmed(false)}>Disarm (fallback)</button>}>
      {armed ? <Bomb /> : <Healthy />}
    </ErrorBoundary>
  );
}

export const NoError: Story = { render: () => <ErrorBoundary><Healthy /></ErrorBoundary> };
export const DefaultFallback: Story = { render: () => <ErrorBoundary><Bomb /></ErrorBoundary> };
export const CustomFallback: Story = {
  render: () => <ErrorBoundary fallback={<div role="alert">Custom fallback UI</div>}><Bomb /></ErrorBoundary>,
};
export const LongErrorMessage: Story = {
  render: () => <ErrorBoundary><Bomb message={"Very long error message. ".repeat(20)} /></ErrorBoundary>,
};
export const Recoverable: Story = { render: () => <RecoverableDemo /> };

// +feature: ui-card
import type { Meta, StoryObj } from "@storybook/react";
import { Card, CardHeader, CardTitle, CardDescription, CardFooter } from "./Card";

const meta: Meta<typeof Card> = { component: Card, title: "UI/Card" };
export default meta;
type Story = StoryObj<typeof Card>;

const body = (
  <>
    <CardHeader>
      <CardTitle>Session summary</CardTitle>
      <CardDescription>Short supporting text</CardDescription>
    </CardHeader>
    <CardFooter>Footer</CardFooter>
  </>
);

export const Default: Story = { args: { variant: "default", children: body } };
export const Elevated: Story = { args: { variant: "elevated", children: body } };
export const Bordered: Story = { args: { variant: "bordered", children: body } };
export const Interactive: Story = { args: { variant: "interactive", children: body } };

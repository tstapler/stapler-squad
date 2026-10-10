// +feature: ui-repo-path-input
import { useState } from "react";
import { Provider } from "react-redux";
import type { Meta, StoryObj } from "@storybook/react";
import { store } from "@/lib/store/store";
import { RepoPathInput } from "./RepoPathInput";

// The shared rich path field: completions, recent repos, worktree grouping,
// optional GitHub-URL detection. Completions need the Go server; offline the
// field still renders and accepts typed paths.
const meta: Meta<typeof RepoPathInput> = {
  component: RepoPathInput,
  title: "UI/RepoPathInput",
  decorators: [(Story) => <Provider store={store}><div style={{ width: 420 }}><Story /></div></Provider>],
};
export default meta;
type Story = StoryObj<typeof RepoPathInput>;

function Controlled(props: Partial<React.ComponentProps<typeof RepoPathInput>>) {
  const [v, setV] = useState(props.value ?? "");
  return <RepoPathInput placeholder="/Users/you/projects/myrepo" {...props} value={v} onChange={setV} />;
}

export const Empty: Story = { render: () => <Controlled /> };
export const Filled: Story = { render: () => <Controlled value="/Users/you/code/stapler-squad" /> };
export const WithHint: Story = { render: () => <Controlled hint="Absolute path to a git repository" /> };
export const WithError: Story = { render: () => <Controlled value="/nope" error="Directory does not exist" /> };
export const Disabled: Story = { render: () => <Controlled value="/Users/you/locked" disabled /> };
export const GitHubUrlDetection: Story = {
  render: () => <Controlled value="https://github.com/tstapler/stapler-squad" detectGitHubUrl />,
};

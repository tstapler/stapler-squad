// +feature: ui-alias-palette
import type { Meta, StoryObj } from "@storybook/react";
import { SessionType } from "@/gen/session/v1/types_pb";
import type { AliasEntry } from "@/lib/hooks/useAliases";
import { AliasPalette } from "./AliasPalette";

// Omnibar "@" picker: ungrouped, grouped, filtered, empty and config-error states.
const meta: Meta<typeof AliasPalette> = {
  component: AliasPalette,
  title: "UI/AliasPalette",
  decorators: [(Story) => <div style={{ width: 520 }}><Story /></div>],
  args: { input: "@", selectedIndex: 0, onSelect: () => {} },
};
export default meta;
type Story = StoryObj<typeof AliasPalette>;

function alias(overrides: Partial<AliasEntry>): AliasEntry {
  return {
    name: "squad",
    group: "",
    path: "/Users/you/code/stapler-squad",
    description: "Main repo",
    profile: "",
    program: "claude",
    autoYes: false,
    tags: [],
    sessionType: SessionType.DIRECTORY,
    namePrefix: "",
    ...overrides,
  } as AliasEntry;
}

const aliases: AliasEntry[] = [
  alias({ name: "squad" }),
  alias({ name: "dotfiles", path: "/Users/you/dotfiles", description: "Dotfiles", program: "aider" }),
  alias({ name: "api", group: "work", path: "/Users/you/work/api", description: "Backend API" }),
  alias({ name: "web", group: "work", path: "/Users/you/work/web", description: "Frontend", program: "" }),
];

export const Default: Story = { args: { aliases } };
export const SecondSelected: Story = { args: { aliases, selectedIndex: 1 } };
export const Filtered: Story = { args: { aliases, input: "@wor" } };
export const Empty: Story = { args: { aliases: [] } };
export const NoMatches: Story = { args: { aliases, input: "@zzz" } };
export const ConfigError: Story = { args: { aliases: [], error: new Error("Unexpected token } in aliases.yaml at line 12") } };
export const LongContent: Story = {
  args: {
    aliases: [
      alias({
        name: "a-very-long-alias-name-for-wrapping",
        description: "An extremely long description that goes on and on to check how the row wraps or truncates in a narrow palette",
        path: "/Users/you/code/github.com/some-organization/some-deeply-nested-repository/packages/service/src",
      }),
    ],
  },
};

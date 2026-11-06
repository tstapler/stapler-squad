// Side-effect module (opt-in, import before the unit under test): swaps the real CodeEditor for a
// lightweight stand-in that records its props. Tests that exercise CodeEditor itself must not import it.

export const fakeCodeEditorProps: { last: Record<string, unknown> | null } = { last: null };

jest.mock("@/components/sessions/CodeEditor", () => {
  const { createElement } = jest.requireActual<typeof import("react")>("react");
  return {
    CodeEditor: (props: Record<string, unknown>) => {
      fakeCodeEditorProps.last = props;
      return createElement("div", { "data-testid": "fake-code-editor", "data-mode": props.mode, "data-path": props.path }, String(props.content));
    },
  };
});


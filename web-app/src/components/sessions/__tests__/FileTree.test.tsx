/**
 * Tests for FileTree component and its helpers.
 *
 * Covers:
 *  - buildTreeData: unloaded dirs get children:[], loaded dirs get actual children
 *  - handleToggle: fires loadDirectory when onToggle receives a dir ID
 *  - handleToggle: retries on previously errored directory
 *  - handleToggle: ignores non-directory IDs
 *  - NodeRenderer onClick: dirs call node.toggle(), files call node.activate()
 *  - Root load on mount calls fetchDirectoryFiles with "."
 *  - After toggle fires, fetchDirectoryFiles is called with the dir path
 *  - childrenAccessor: returns null for files, [] for dirs (keeps isLeaf=false)
 */

import React from "react";
import { render, fireEvent, act, waitFor } from "@testing-library/react";
import { FileTree, buildTreeData } from "../FileTree";
import type { TreeNode } from "../FileTree";

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// Capture Tree props so tests can inspect data and manually trigger callbacks.
let capturedOnToggle: ((id: string) => void) | undefined;
let capturedChildren:
  | ((props: { node: MockNodeApi; style: React.CSSProperties; dragHandle?: unknown }) => React.ReactNode)
  | undefined;
let capturedData: TreeNode[] = [];
// Captured ref so keyboard tests can inject a mock TreeApi.
let capturedTreeRef: React.MutableRefObject<MockTreeApi | undefined> | null = null;

interface MockNodeApi {
  id: string;
  data: TreeNode;
  isOpen: boolean;
  isFocused: boolean;
  level: number;
  parent: { isRoot: boolean; id: string } | null;
  toggle: jest.Mock;
  activate: jest.Mock;
}

/** Minimal mock of TreeApi methods used by handleTreeKeyDown. */
interface MockTreeApi {
  focus: jest.Mock;
  open: jest.Mock;
  close: jest.Mock;
  visibleNodes: MockNodeApi[];
  focusedNode: MockNodeApi | null;
}

jest.mock("react-arborist", () => {
  // eslint-disable-next-line @typescript-eslint/no-require-imports
  const React = require("react");
  const MockTree = React.forwardRef(
    (
      props: {
        data: TreeNode[];
        onToggle?: (id: string) => void;
        onActivate?: (node: unknown) => void;
        children: (props: { node: MockNodeApi; style: React.CSSProperties; dragHandle?: unknown }) => React.ReactNode;
        idAccessor?: (node: TreeNode) => string;
        childrenAccessor?: (node: TreeNode) => TreeNode[] | null;
      },
      ref: React.Ref<unknown>
    ) => {
      capturedOnToggle = props.onToggle;
      capturedChildren = props.children;
      capturedData = props.data;
      // Expose the ref so keyboard tests can inject a mock TreeApi.
      if (ref && typeof ref === "object" && "current" in ref) {
        capturedTreeRef = ref as React.MutableRefObject<MockTreeApi | undefined>;
      }
      return React.createElement("div", { "data-testid": "mock-tree" });
    }
  );
  MockTree.displayName = "MockTree";
  return { Tree: MockTree };
});

const mockFetchDirectoryFiles = jest.fn();
const mockSearchFiles = jest.fn();

jest.mock("@/lib/hooks/useFileService", () => ({
  fetchDirectoryFiles: (...args: unknown[]) => mockFetchDirectoryFiles(...args),
  searchFiles: (...args: unknown[]) => mockSearchFiles(...args),
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeFileNode(path: string, isDir: boolean, children?: TreeNode[]): TreeNode {
  return {
    id: path,
    name: path.split("/").pop() ?? path,
    isDir,
    size: BigInt(0),
    gitStatus: "",
    isSymlink: false,
    symlinkTarget: "",
    isIgnored: false,
    children,
  };
}

/** Build a mock fetchDirectoryFiles response from a list of entries. */
function makeFileResponse(entries: { path: string; isDir: boolean }[]) {
  return {
    files: entries.map((e) => ({
      path: e.path,
      name: e.path.split("/").pop() ?? e.path,
      isDir: e.isDir,
      size: BigInt(0),
      gitStatus: "",
      isSymlink: false,
      symlinkTarget: "",
      isIgnored: false,
    })),
  };
}

function makeMockNode(data: TreeNode, overrides: Partial<MockNodeApi> = {}): MockNodeApi {
  return {
    id: data.id,
    data,
    isOpen: false,
    isFocused: false,
    level: 0,
    parent: null,
    toggle: jest.fn(),
    activate: jest.fn(),
    ...overrides,
  };
}

/** Build a minimal mock TreeApi and inject it into the captured ref. */
function injectMockTreeApi(api: MockTreeApi): void {
  if (!capturedTreeRef) throw new Error("capturedTreeRef not set — render FileTree first");
  capturedTreeRef.current = api as unknown as (typeof capturedTreeRef)["current"];
}

/** Build a default MockTreeApi with no focused node and empty visible list. */
function makeMockTreeApi(overrides: Partial<MockTreeApi> = {}): MockTreeApi {
  return {
    focus: jest.fn(),
    open: jest.fn(),
    close: jest.fn(),
    visibleNodes: [],
    focusedNode: null,
    ...overrides,
  };
}

const defaultProps = {
  sessionId: "test-session-uuid",
  baseUrl: "http://localhost:8543",
  onFileSelect: jest.fn(),
};

// ---------------------------------------------------------------------------
// buildTreeData pure function
// ---------------------------------------------------------------------------

describe("buildTreeData", () => {
  it("returns non-dir nodes unchanged", () => {
    const file = makeFileNode("README.md", false);
    const result = buildTreeData([file], new Map());
    expect(result).toHaveLength(1);
    expect(result[0]).toBe(file);
  });

  it("gives unloaded dirs children: [] so arborist isLeaf=false (toggleable)", () => {
    const dir = makeFileNode("src", true, undefined);
    const result = buildTreeData([dir], new Map());
    expect(Array.isArray(result[0].children)).toBe(true);
    expect(result[0].children).toEqual([]);
  });

  it("gives loaded empty dirs children: [] (confirmed empty)", () => {
    const dir = makeFileNode("empty-dir", true, undefined);
    const contents = new Map([["empty-dir", [] as TreeNode[]]]);
    const result = buildTreeData([dir], contents);
    expect(result[0].children).toEqual([]);
  });

  it("attaches loaded children for loaded dirs", () => {
    const dir = makeFileNode("src", true, undefined);
    const child = makeFileNode("src/main.go", false);
    const contents = new Map([["src", [child]]]);
    const result = buildTreeData([dir], contents);
    expect(result[0].children).toHaveLength(1);
    expect(result[0].children![0].id).toBe("src/main.go");
  });

  it("recursively attaches nested loaded children", () => {
    const root = makeFileNode("src", true, undefined);
    const sub = makeFileNode("src/components", true, undefined);
    const file = makeFileNode("src/components/Button.tsx", false);
    const contents = new Map<string, TreeNode[]>([
      ["src", [sub]],
      ["src/components", [file]],
    ]);
    const result = buildTreeData([root], contents);
    const srcNode = result[0];
    expect(srcNode.children).toHaveLength(1);
    const subNode = srcNode.children![0];
    expect(subNode.id).toBe("src/components");
    expect(subNode.children).toHaveLength(1);
    expect(subNode.children![0].id).toBe("src/components/Button.tsx");
  });

  it("mixes loaded and unloaded dirs correctly", () => {
    const loaded = makeFileNode("loaded", true, undefined);
    const unloaded = makeFileNode("unloaded", true, undefined);
    const file = makeFileNode("loaded/file.go", false);
    const contents = new Map<string, TreeNode[]>([["loaded", [file]]]);

    const result = buildTreeData([loaded, unloaded], contents);
    expect(result[0].children).toHaveLength(1);
    expect(result[1].children).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// childrenAccessor contract (inline logic test — no component render needed)
// ---------------------------------------------------------------------------

describe("childrenAccessor logic", () => {
  // The actual childrenAccessor used in FileTree is:
  //   (node) => { if (!node.isDir) return null; return node.children ?? []; }
  // We test this logic directly to ensure the isLeaf contract is preserved.

  const accessor = (node: TreeNode): TreeNode[] | null => {
    if (!node.isDir) return null;
    return node.children ?? [];
  };

  it("returns null for a file node (makes it a leaf)", () => {
    expect(accessor(makeFileNode("main.go", false))).toBeNull();
  });

  it("returns [] for an unloaded directory (children: undefined)", () => {
    const result = accessor(makeFileNode("src", true, undefined));
    expect(Array.isArray(result)).toBe(true);
    expect(result).toEqual([]);
  });

  it("returns [] for a confirmed empty directory (children: [])", () => {
    const result = accessor(makeFileNode("empty", true, []));
    expect(result).toEqual([]);
  });

  it("returns loaded children for a dir with children", () => {
    const child = makeFileNode("src/main.go", false);
    const result = accessor(makeFileNode("src", true, [child]));
    expect(result).toHaveLength(1);
    expect(result![0].id).toBe("src/main.go");
  });

  it("[] from accessor means isLeaf=false (Array.isArray([]) is true)", () => {
    const children = accessor(makeFileNode("src", true, undefined));
    // react-arborist's isLeaf check: !Array.isArray(this.children)
    expect(Array.isArray(children)).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// FileTree – root loading
// ---------------------------------------------------------------------------

describe("FileTree – root loading", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedOnToggle = undefined;
    capturedChildren = undefined;
    capturedData = [];
  });

  it("calls fetchDirectoryFiles with '.' on mount", async () => {
    mockFetchDirectoryFiles.mockResolvedValue(
      makeFileResponse([{ path: "src", isDir: true }])
    );

    render(<FileTree {...defaultProps} />);

    await waitFor(() => {
      expect(mockFetchDirectoryFiles).toHaveBeenCalledWith(
        "test-session-uuid",
        ".",
        false,
        "http://localhost:8543"
      );
    });
  });

  it("passes root nodes to Tree after successful load", async () => {
    mockFetchDirectoryFiles.mockResolvedValue(
      makeFileResponse([
        { path: "README.md", isDir: false },
        { path: "src", isDir: true },
      ])
    );

    render(<FileTree {...defaultProps} />);

    await waitFor(() => {
      const srcNode = capturedData.find((n) => n.id === "src");
      expect(srcNode).toBeDefined();
      expect(srcNode!.isDir).toBe(true);
    });
  });
});

// ---------------------------------------------------------------------------
// FileTree – handleToggle loads directories
// ---------------------------------------------------------------------------

describe("FileTree – handleToggle loads directories", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedOnToggle = undefined;
    capturedData = [];
  });

  /** Wait for Tree to be rendered with at least one node containing the given id. */
  async function waitForTreeWithNode(id: string) {
    await waitFor(() => {
      expect(capturedOnToggle).toBeDefined();
      const node = capturedData.find((n) => n.id === id);
      expect(node).toBeDefined();
    });
  }

  it("calls fetchDirectoryFiles for a dir ID when onToggle fires", async () => {
    // Both root useEffects fire on mount; use mockImplementation to handle all calls.
    mockFetchDirectoryFiles.mockImplementation((_session: string, path: string) => {
      if (path === ".") return Promise.resolve(makeFileResponse([{ path: "src", isDir: true }]));
      if (path === "src") return Promise.resolve(makeFileResponse([{ path: "src/main.go", isDir: false }]));
      return Promise.resolve(makeFileResponse([]));
    });

    render(<FileTree {...defaultProps} />);
    await waitForTreeWithNode("src");

    // Simulate user toggling "src" directory.
    await act(async () => {
      capturedOnToggle!("src");
    });

    await waitFor(() => {
      expect(mockFetchDirectoryFiles).toHaveBeenCalledWith(
        "test-session-uuid",
        "src",
        false,
        "http://localhost:8543"
      );
    });
  });

  it("does not reload a directory that is already loaded", async () => {
    let srcLoadCount = 0;
    mockFetchDirectoryFiles.mockImplementation((_session: string, path: string) => {
      if (path === ".") return Promise.resolve(makeFileResponse([{ path: "src", isDir: true }]));
      if (path === "src") {
        srcLoadCount++;
        return Promise.resolve(makeFileResponse([{ path: "src/main.go", isDir: false }]));
      }
      return Promise.resolve(makeFileResponse([]));
    });

    render(<FileTree {...defaultProps} />);
    await waitForTreeWithNode("src");

    // First toggle loads src.
    await act(async () => { capturedOnToggle!("src"); });
    await waitFor(() => expect(srcLoadCount).toBe(1));

    // Second toggle should NOT reload (already in dirContents).
    await act(async () => { capturedOnToggle!("src"); });
    expect(srcLoadCount).toBe(1);
  });

  it("retries a directory that previously errored", async () => {
    let srcLoadCount = 0;
    mockFetchDirectoryFiles.mockImplementation((_session: string, path: string) => {
      if (path === ".") return Promise.resolve(makeFileResponse([{ path: "src", isDir: true }]));
      if (path === "src") {
        srcLoadCount++;
        if (srcLoadCount === 1) return Promise.reject(new Error("network error"));
        return Promise.resolve(makeFileResponse([{ path: "src/main.go", isDir: false }]));
      }
      return Promise.resolve(makeFileResponse([]));
    });

    render(<FileTree {...defaultProps} />);
    await waitForTreeWithNode("src");

    // First toggle — fails.
    await act(async () => { capturedOnToggle!("src"); });
    await waitFor(() => expect(srcLoadCount).toBe(1));

    // Second toggle — should retry because errorPaths contains "src".
    await act(async () => { capturedOnToggle!("src"); });
    await waitFor(() => expect(srcLoadCount).toBe(2));
  });

  it("ignores toggles for non-directory IDs", async () => {
    let nonDirLoadCount = 0;
    mockFetchDirectoryFiles.mockImplementation((_session: string, path: string) => {
      if (path === ".") return Promise.resolve(makeFileResponse([{ path: "README.md", isDir: false }]));
      nonDirLoadCount++;
      return Promise.resolve(makeFileResponse([]));
    });

    render(<FileTree {...defaultProps} />);
    await waitFor(() => expect(capturedOnToggle).toBeDefined());

    // Toggle a file node — should not trigger any directory fetch.
    await act(async () => { capturedOnToggle!("README.md"); });

    // No fetch beyond root should have happened.
    expect(nonDirLoadCount).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// NodeRenderer onClick – dirs call toggle, files call activate
// ---------------------------------------------------------------------------

describe("FileTree – NodeRenderer click behavior", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    capturedOnToggle = undefined;
    capturedChildren = undefined;
  });

  async function setupWithRootNodes(entries: { path: string; isDir: boolean }[]) {
    mockFetchDirectoryFiles.mockResolvedValue(makeFileResponse(entries));
    render(<FileTree {...defaultProps} />);
    await waitFor(() => expect(capturedChildren).toBeDefined());
  }

  it("clicking a closed directory node calls node.toggle() not node.activate()", async () => {
    await setupWithRootNodes([{ path: "src", isDir: true }]);

    const dirData = makeFileNode("src", true, []);
    const mockNode = makeMockNode(dirData, { isOpen: false });

    const { container } = render(
      <>{capturedChildren!({ node: mockNode, style: {} })}</>
    );

    fireEvent.click(container.querySelector("div")!);

    expect(mockNode.toggle).toHaveBeenCalledTimes(1);
    expect(mockNode.activate).not.toHaveBeenCalled();
  });

  it("clicking a file node calls node.activate() not node.toggle()", async () => {
    await setupWithRootNodes([{ path: "main.go", isDir: false }]);

    const fileData = makeFileNode("main.go", false);
    const mockNode = makeMockNode(fileData);

    const { container } = render(
      <>{capturedChildren!({ node: mockNode, style: {} })}</>
    );

    fireEvent.click(container.querySelector("div")!);

    expect(mockNode.activate).toHaveBeenCalledTimes(1);
    expect(mockNode.toggle).not.toHaveBeenCalled();
  });

  it("clicking an open directory node also calls node.toggle() to collapse it", async () => {
    await setupWithRootNodes([{ path: "src", isDir: true }]);

    const dirData = makeFileNode("src", true, []);
    const mockNode = makeMockNode(dirData, { isOpen: true });

    const { container } = render(
      <>{capturedChildren!({ node: mockNode, style: {} })}</>
    );

    fireEvent.click(container.querySelector("div")!);

    expect(mockNode.toggle).toHaveBeenCalledTimes(1);
    expect(mockNode.activate).not.toHaveBeenCalled();
  });
});

// ---------------------------------------------------------------------------
// FileTree – keyboard navigation
// ---------------------------------------------------------------------------

describe("FileTree – keyboard navigation", () => {
  /**
   * Render a loaded FileTree, wait for the container div (with onKeyDown),
   * inject a mock TreeApi, and return a helper to fire key events.
   */
  async function setupKeyboard(treeApi: MockTreeApi) {
    mockFetchDirectoryFiles.mockResolvedValue(
      makeFileResponse([{ path: "a.go", isDir: false }])
    );

    const { container } = render(<FileTree {...defaultProps} />);

    // Wait for the tree to render (root loading completes).
    await waitFor(() => expect(capturedTreeRef).not.toBeNull());

    injectMockTreeApi(treeApi);

    // The container div has tabIndex=0 and onKeyDown.
    const div = container.querySelector("[tabindex='0']") as HTMLElement;
    expect(div).not.toBeNull();

    function fireKey(key: string, modifiers: Partial<KeyboardEventInit> = {}) {
      fireEvent.keyDown(div, { key, ...modifiers });
    }

    return { fireKey };
  }

  beforeEach(() => {
    jest.clearAllMocks();
    capturedOnToggle = undefined;
    capturedChildren = undefined;
    capturedData = [];
    capturedTreeRef = null;
  });

  it("j key moves focus to next visible node", async () => {
    const nodeA = makeMockNode(makeFileNode("a.go", false));
    const nodeB = makeMockNode(makeFileNode("b.go", false));
    const api = makeMockTreeApi({
      visibleNodes: [nodeA, nodeB],
      focusedNode: nodeA,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("j");

    expect(api.focus).toHaveBeenCalledWith(nodeB.id);
  });

  it("k key moves focus to previous visible node", async () => {
    const nodeA = makeMockNode(makeFileNode("a.go", false));
    const nodeB = makeMockNode(makeFileNode("b.go", false));
    const api = makeMockTreeApi({
      visibleNodes: [nodeA, nodeB],
      focusedNode: nodeB,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("k");

    expect(api.focus).toHaveBeenCalledWith(nodeA.id);
  });

  it("l key on directory opens it", async () => {
    const dirData = makeFileNode("src", true, []);
    const dirNode = makeMockNode(dirData);
    const api = makeMockTreeApi({
      visibleNodes: [dirNode],
      focusedNode: dirNode,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("l");

    expect(api.open).toHaveBeenCalledWith(dirNode.id);
    expect(dirNode.activate).not.toHaveBeenCalled();
  });

  it("l key on file activates it", async () => {
    const fileData = makeFileNode("main.go", false);
    const fileNode = makeMockNode(fileData);
    const api = makeMockTreeApi({
      visibleNodes: [fileNode],
      focusedNode: fileNode,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("l");

    expect(fileNode.activate).toHaveBeenCalledTimes(1);
    expect(api.open).not.toHaveBeenCalled();
  });

  it("h key collapses open directory", async () => {
    const dirData = makeFileNode("src", true, []);
    const dirNode = makeMockNode(dirData, { isOpen: true });
    const api = makeMockTreeApi({
      visibleNodes: [dirNode],
      focusedNode: dirNode,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("h");

    expect(api.close).toHaveBeenCalledWith(dirNode.id);
  });

  it("G key jumps to last visible node", async () => {
    const nodeA = makeMockNode(makeFileNode("a.go", false));
    const nodeB = makeMockNode(makeFileNode("b.go", false));
    const nodeC = makeMockNode(makeFileNode("c.go", false));
    const api = makeMockTreeApi({
      visibleNodes: [nodeA, nodeB, nodeC],
      focusedNode: nodeA,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("G");

    expect(api.focus).toHaveBeenCalledWith(nodeC.id);
  });

  it("modifier key (Ctrl+j) does not move focus", async () => {
    const nodeA = makeMockNode(makeFileNode("a.go", false));
    const nodeB = makeMockNode(makeFileNode("b.go", false));
    const api = makeMockTreeApi({
      visibleNodes: [nodeA, nodeB],
      focusedNode: nodeA,
    });

    const { fireKey } = await setupKeyboard(api);
    fireKey("j", { ctrlKey: true });

    expect(api.focus).not.toHaveBeenCalled();
  });
});

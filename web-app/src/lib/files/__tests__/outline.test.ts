import { buildOutline, OUTLINE_MAX_BYTES, outlineLanguages } from "../outline";

const lines = (n: number, at: Record<number, string>) =>
  Array.from({ length: n }, (_, i) => at[i + 1] ?? "// filler").join("\n");

describe("buildOutline", () => {
  it("outline_should_ListFuncAndTypeWithLines_When_GoFile", () => {
    const text = lines(30, {
      5: "func A() {}",
      12: "func (s *Server) Handle(w int) error {",
      20: "type B struct {",
      25: "type Reader interface {",
    });
    const out = buildOutline(text, "go");
    expect(out).toEqual({
      kind: "symbols",
      symbols: [
        { name: "A", line: 5, kind: "function", depth: 0 },
        { name: "Server.Handle", line: 12, kind: "method", depth: 0 },
        { name: "B", line: 20, kind: "type", depth: 0 },
        { name: "Reader", line: 25, kind: "type", depth: 0 },
      ],
    });
  });

  it.each([
    ["typescript", "export async function load() {}\nexport class Foo {}\nexport interface Bar {}\nexport type Baz = 1;\nexport const run = async (x: number) => x;\nconst notFn = 5;", ["load", "Foo", "Bar", "Baz", "run"]],
    ["javascript", "function a() {}\nclass B {}\nconst c = () => 1;", ["a", "B", "c"]],
    ["python", "def top():\n    pass\nclass K:\n    def method(self):\n        pass\n", ["top", "K", "method"]],
    ["rust", "pub fn run() {}\npub(crate) struct S;\nenum E {}\ntrait T {}\nimpl S {\n    fn inner(&self) {}\n}", ["run", "S", "E", "T", "impl S", "inner"]],
    ["java", "public class Main {\n    public static void main(String[] args) {\n    }\n    private int helper(int x) {\n    }\n}", ["Main", "main", "helper"]],
    ["markdown", "# Title\ntext\n## Sub\n```\n# not a heading\n```\n### Deep", ["Title", "Sub", "Deep"]],
  ])("outline_should_FindSymbols_When_%s", (language, text, names) => {
    const out = buildOutline(text, language);
    expect(out.kind).toBe("symbols");
    if (out.kind === "symbols") expect(out.symbols.map((s) => s.name)).toEqual(names);
  });

  it("outline_should_NestByIndentAndHeadingLevel_When_PythonOrMarkdown", () => {
    const py = buildOutline("class K:\n    def m(self):\n        pass", "python");
    expect(py.kind === "symbols" && py.symbols.map((s) => s.depth)).toEqual([0, 1]);
    const md = buildOutline("# A\n## B\n### C", "markdown");
    expect(md.kind === "symbols" && md.symbols.map((s) => s.depth)).toEqual([0, 1, 2]);
  });

  it("outline_should_ReturnUnavailable_When_LargeFileOrUnsupportedLanguage", () => {
    expect(buildOutline("func A() {}", "go", { size: OUTLINE_MAX_BYTES + 1 })).toEqual({ kind: "unavailable", reason: "large" });
    expect(buildOutline("anything", "yaml")).toEqual({ kind: "unavailable", reason: "unsupported" });
    expect(buildOutline("func A() {}", "go", { size: OUTLINE_MAX_BYTES })).toMatchObject({ kind: "symbols" });
    expect(() => buildOutline(null as unknown as string, "go")).not.toThrow();
    expect(buildOutline(null as unknown as string, "go")).toEqual({ kind: "unavailable", reason: "error" });
  });

  it("outline_should_ReturnNoSymbols_When_SupportedLanguageHasNone", () => {
    expect(buildOutline("// nothing here", "go")).toEqual({ kind: "symbols", symbols: [] });
    expect(outlineLanguages()).toEqual(expect.arrayContaining(["go", "typescript", "python", "rust", "java", "markdown"]));
  });
});

/**
 * Returns an emoji/symbol icon for a given filename based on its extension.
 */
export function getFileIcon(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() || "";
  const icons: Record<string, string> = {
    go: "🐹",
    ts: "𝐓",
    tsx: "⚛",
    js: "𝐉",
    jsx: "⚛",
    py: "🐍",
    rs: "🦀",
    md: "📄",
    json: "{}",
    yaml: "⚙",
    yml: "⚙",
    toml: "⚙",
    sh: "💲",
    css: "🎨",
    html: "🌐",
  };
  return icons[ext] || "📄";
}

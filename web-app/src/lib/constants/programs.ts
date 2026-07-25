export interface ProgramOption {
  value: string;
  label: string;
  description?: string;
}

export const PROGRAMS: ProgramOption[] = [
  { value: "", label: "System default", description: "Use default_program from config (default: claude)" },
  { value: "claude", label: "Claude Code", description: "Anthropic's CLI assistant" },
  { value: "env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=http://localhost:47000 claude", label: "Claude Code (Proxy via localhost:47000)", description: "Via local proxy" },
  { value: "aider", label: "Aider", description: "AI pair programming with git" },
  { value: "aider --model ollama_chat/gemma3:1b", label: "Aider (Ollama Gemma 1B)", description: "Local model" },
  { value: "opencode", label: "OpenCode", description: "OpenCode CLI assistant" },
  { value: "gemini", label: "Gemini CLI", description: "Google Gemini CLI" },
  { value: "agy", label: "Antigravity", description: "Antigravity CLI (agy)" },
  { value: "bash", label: "Terminal", description: "Interactive shell session" },
];

export const DEFAULT_PROGRAM = "claude";

export interface ModelOption {
  value: string;
  label: string;
}

/** Known Claude model IDs, newest first. */
export const CLAUDE_MODELS: ModelOption[] = [
  { value: "claude-opus-4-8",            label: "Claude Opus 4.8 (latest)" },
  { value: "claude-sonnet-4-6",          label: "Claude Sonnet 4.6" },
  { value: "claude-haiku-4-5-20251001",  label: "Claude Haiku 4.5" },
  { value: "claude-opus-4-5",            label: "Claude Opus 4.5" },
  { value: "claude-sonnet-4-5",          label: "Claude Sonnet 4.5" },
  { value: "claude-haiku-4-5",           label: "Claude Haiku 4.5 (base)" },
  { value: "claude-opus-4",              label: "Claude Opus 4" },
  { value: "claude-sonnet-4",            label: "Claude Sonnet 4" },
  { value: "claude-haiku-4",             label: "Claude Haiku 4" },
];

export function getProgramDisplay(program?: string): string {
  if (!program) return "Claude Code (default)";
  const option = PROGRAMS.find((p) => p.value === program);
  if (option) return option.label;
  if (program.startsWith("aider --model")) return program;
  return program;
}

export function isKnownProgram(program: string): boolean {
  return PROGRAMS.some((p) => p.value === program) || program.startsWith("aider --model");
}

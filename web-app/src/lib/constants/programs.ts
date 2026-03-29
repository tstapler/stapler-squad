export interface ProgramOption {
  value: string;
  label: string;
  description?: string;
}

export const PROGRAMS: ProgramOption[] = [
  { value: "claude", label: "Claude Code", description: "Anthropic's CLI assistant" },
  { value: "env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=http://localhost:47000 claude", label: "Claude Code (Proxy via localhost:47000)", description: "Via local proxy" },
  { value: "aider", label: "Aider", description: "AI pair programming with git" },
  { value: "aider --model ollama_chat/gemma3:1b", label: "Aider (Ollama Gemma 1B)", description: "Local model" },
  { value: "opencode", label: "OpenCode", description: "OpenCode CLI assistant" },
  { value: "gemini", label: "Gemini CLI", description: "Google Gemini CLI" },
];

export const DEFAULT_PROGRAM = "claude";

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

export interface ProgramOption {
  value: string;
  label: string;
  description?: string;
}

// pi-support's flag name (mirrors config.FeaturePiSupport / feature_flag_service.go's
// piSupportFlagName — kept as a plain string constant here since the frontend has no
// shared import path to those Go constants). Single source of truth for the frontend;
// import this everywhere instead of redeclaring the literal.
export const PI_SUPPORT_FLAG_NAME = "pi-support";

export const PROGRAMS: ProgramOption[] = [
  { value: "", label: "System default", description: "Use default_program from config (default: claude)" },
  { value: "claude", label: "Claude Code", description: "Anthropic's CLI assistant" },
  { value: "env -u CLAUDE_CODE_USE_BEDROCK ANTHROPIC_BASE_URL=http://localhost:47000 claude", label: "Claude Code (Proxy via localhost:47000)", description: "Via local proxy" },
  { value: "pi", label: "pi", description: "@earendil-works/pi-coding-agent — TypeScript-extensible CLI" },
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

/**
 * Model-family pseudo-entries. The `family:` prefix is intentionally left
 * unresolved here — resolution to a concrete model ID happens server-side at
 * workflow fire-time (server/workflows/model_families.go's ResolveModel,
 * called from scheduler.go's FireNow), not client-side, so a new Anthropic
 * release becomes "latest" without a frontend redeploy.
 */
export const MODEL_FAMILIES: ModelOption[] = [
  { value: "family:opus",   label: "Opus (latest)" },
  { value: "family:sonnet", label: "Sonnet (latest)" },
  { value: "family:haiku",  label: "Haiku (latest)" },
];

/** Every model suggestion offered in the Workflow model picker: families first, then concrete IDs. */
export const MODEL_AUTOCOMPLETE_OPTIONS: ModelOption[] = [...MODEL_FAMILIES, ...CLAUDE_MODELS];

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

/**
 * Programs to offer in the creation-panel picker, filtered from whichever program
 * list the caller has (usually `PROGRAMS`, but the creation panel derives its own
 * list via useAvailablePrograms() to include extra programs detected on the host).
 * `PROGRAMS` itself stays unconditional (getProgramDisplay/isKnownProgram must
 * recognize "pi" even with the flag off, for sessions that already have
 * program: "pi" set from before pi-support was disabled) -- this helper filters
 * only the rendered option list, per pi-support's "opt-in invisibility" requirement.
 */
export function getPickerPrograms(
  programs: ProgramOption[],
  piSupportEnabled: boolean,
): ProgramOption[] {
  if (piSupportEnabled) return programs;
  return programs.filter((p) => p.value !== "pi");
}

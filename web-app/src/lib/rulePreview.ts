// Client-side port of the key subset of CommandCriteria.Matches() from pkg/classifier.
// Skips: SafePythonImportsOnly (requires AST), RedirectionPattern (not in UI form).

export interface RuleCriteria {
  programs: string[];
  subcommands: string[];
  blockedSubcommands: string[];
  requiredFlags: string[];
  forbiddenFlags: string[];
  requiredFlagPrefixes: string[];
  pythonModes: string[];
}

export interface PreviewResult {
  matches: string[];
  nonMatches: string[];
}

// Programs whose subcommand extraction captures two tokens.
const DEEP_SUBCOMMAND_PROGRAMS = new Set([
  "gh", "docker", "kubectl", "aws", "gcloud", "az", "terraform", "ip",
  "git", "npm", "cargo", "go", "pip", "uv",
]);

function matchesProgram(programs: string[], program: string): boolean {
  if (programs.length === 0) return true;
  const lower = program.toLowerCase();
  return programs.some((p) => {
    const pl = p.toLowerCase();
    return lower === pl || lower.startsWith(pl + ".");
  });
}

function extractSubcommand(program: string, args: string[]): string {
  const filtered = args.filter((a) => !a.startsWith("-") || a.includes(" "));
  const depth = DEEP_SUBCOMMAND_PROGRAMS.has(program.toLowerCase()) ? 2 : 1;
  return filtered.slice(0, depth).join(" ");
}

function parseSimpleCommand(cmd: string): { program: string; args: string[] } {
  const parts = cmd.trim().split(/\s+/);
  return { program: parts[0] ?? "", args: parts.slice(1) };
}

function detectPythonMode(program: string, args: string[]): string {
  if (!program.toLowerCase().startsWith("python") && !program.toLowerCase().startsWith("pypy")) {
    return "";
  }
  if (args.includes("-c")) return "inline";
  if (args.includes("-m")) return "module";
  if (args.includes("-V") || args.includes("--version")) return "version";
  const scriptArg = args.find((a) => !a.startsWith("-") && a.endsWith(".py"));
  if (scriptArg) return "script";
  return "";
}

export function matchesCriteria(criteria: RuleCriteria, cmd: string): boolean {
  const { program, args } = parseSimpleCommand(cmd);
  if (!program) return false;

  if (!matchesProgram(criteria.programs, program)) return false;

  const sub = extractSubcommand(program, args);

  if (criteria.subcommands.length > 0) {
    const found = criteria.subcommands.some(
      (s) => sub === s || sub.startsWith(s + " ")
    );
    if (!found) return false;
  }

  for (const bs of criteria.blockedSubcommands) {
    if (sub === bs) return false;
  }

  if (criteria.requiredFlags.length > 0) {
    const found = criteria.requiredFlags.some((rf) => args.includes(rf));
    if (!found) return false;
  }

  if (criteria.requiredFlagPrefixes.length > 0) {
    const found = criteria.requiredFlagPrefixes.some((prefix) =>
      args.some((a) => a.startsWith(prefix))
    );
    if (!found) return false;
  }

  for (const ff of criteria.forbiddenFlags) {
    if (args.includes(ff)) return false;
  }

  if (criteria.pythonModes.length > 0) {
    const mode = detectPythonMode(program, args);
    if (!criteria.pythonModes.includes(mode)) return false;
  }

  return true;
}

// Static example bank — one entry per (program, common-usage) pair.
const EXAMPLE_BANK: string[] = [
  // git
  "git log --oneline", "git status", "git diff HEAD", "git push origin main",
  "git push --force", "git reset --hard HEAD~1", "git checkout main",
  "git commit -m 'fix'", "git pull", "git branch -d old-branch",
  // python
  "python3 script.py", "python3 -m pytest", "python3 -c \"import json; print(json.dumps({}))\"",
  "python3 -c \"import requests; requests.get('http://example.com')\"", "python3 -V",
  "python script.py --arg value",
  // npm / yarn
  "npm install", "npm test", "npm publish", "npm run build", "yarn install",
  "npx jest", "pnpm install",
  // docker
  "docker ps", "docker logs my-container", "docker run -it ubuntu bash",
  "docker build -t my-image .", "docker exec -it container bash",
  // go
  "go build ./...", "go test ./...", "go run main.go", "go fmt ./...",
  // gh
  "gh pr list", "gh pr view 123", "gh pr create", "gh pr merge 123",
  "gh issue create", "gh api repos/owner/repo", "gh auth login",
  // curl
  "curl https://api.example.com", "curl -X POST https://api.example.com -d '{}'",
  "curl -o output.txt https://example.com",
  // misc
  "make build", "make test", "rm -rf /tmp/old", "sed -i 's/foo/bar/g' file.txt",
  "brew install jq", "pip install requests", "cargo build",
  "kubectl get pods", "terraform plan",
];

export function computePreview(criteria: RuleCriteria): PreviewResult {
  const hasAnyCriteria =
    criteria.programs.length > 0 ||
    criteria.subcommands.length > 0 ||
    criteria.blockedSubcommands.length > 0 ||
    criteria.requiredFlags.length > 0 ||
    criteria.forbiddenFlags.length > 0 ||
    criteria.requiredFlagPrefixes.length > 0 ||
    criteria.pythonModes.length > 0;

  if (!hasAnyCriteria) return { matches: [], nonMatches: [] };

  const matches: string[] = [];
  const nonMatches: string[] = [];

  for (const cmd of EXAMPLE_BANK) {
    if (matches.length >= 5 && nonMatches.length >= 3) break;
    if (matchesCriteria(criteria, cmd)) {
      if (matches.length < 5) matches.push(cmd);
    } else {
      if (nonMatches.length < 3) nonMatches.push(cmd);
    }
  }

  return { matches, nonMatches };
}

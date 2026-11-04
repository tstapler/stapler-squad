/**
 * Codemod: route every e2e file's server-origin / data-dir read through helpers/base-url.ts so it
 * resolves to the worker's own server (see that file). Idempotent: a migrated file has nothing left to rewrite.
 *
 *   bun tests/e2e/scripts/migrate-base-url.ts          # rewrite in place, print a report
 *   bun tests/e2e/scripts/migrate-base-url.ts --check  # no writes; exit 1 if any raw read remains
 *
 * Rewritten (AST, not regex):
 *   process.env.TEST_SERVER_URL || process.env.BASE_URL || 'http://localhost:8544'  (any ||/?? chain
 *   of those reads and string literals, parenthesised or not)            -> baseUrl()
 *   process.env.TEST_SERVER_TESTDIR (bare)                               -> testDir()
 * Reported, never rewritten (need a human): a TEST_SERVER_URL/BASE_URL read that is not part of a
 * literal-fallback chain, because baseUrl() would silently change `undefined` into a URL.
 */
import * as fs from 'fs';
import * as path from 'path';
import ts from 'typescript';

const E2E_ROOT = path.resolve(__dirname, '..');
const HELPER = path.join(E2E_ROOT, 'helpers', 'base-url');
const URL_VARS = new Set(['TEST_SERVER_URL', 'BASE_URL']);
const DIR_VARS = new Set(['TEST_SERVER_TESTDIR']);

// Files that legitimately read the raw env vars: they define or publish the values.
const SKIP = [
  'node_modules/',
  'allure-results/',
  'scripts/',
  'helpers/base-url.ts',
  'helpers/test-server.ts',
  'global-setup',
  'global-teardown',
  'playwright.config.ts',
  'playwright.dev-mode.config.ts',
  'worker-server-isolation.spec.ts',
  '.dev-mode.spec.ts',
  'baseline/',
  'perf/', // own harness, separate project
];

function listFiles(dir: string, out: string[] = []): string[] {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) listFiles(full, out);
    else if (full.endsWith('.ts')) out.push(full);
  }
  return out;
}

/** `process.env.X` -> 'X', else undefined. */
function envName(node: ts.Node): string | undefined {
  if (
    ts.isPropertyAccessExpression(node) &&
    ts.isPropertyAccessExpression(node.expression) &&
    node.expression.name.text === 'env' &&
    ts.isIdentifier(node.expression.expression) &&
    node.expression.expression.text === 'process'
  ) {
    return node.name.text;
  }
  return undefined;
}

function isChainLeaf(node: ts.Node): boolean {
  return ts.isStringLiteralLike(node) || (envName(node) !== undefined && URL_VARS.has(envName(node)!));
}

/** An ||/?? tree whose leaves are URL env reads or string literals, containing at least one string literal. */
function isChain(node: ts.Node): boolean {
  return chainShape(node) && hasLiteral(node);
}

function chainShape(node: ts.Node): boolean {
  if (ts.isParenthesizedExpression(node)) return chainShape(node.expression);
  if (!ts.isBinaryExpression(node)) return false;
  const op = node.operatorToken.kind;
  if (op !== ts.SyntaxKind.BarBarToken && op !== ts.SyntaxKind.QuestionQuestionToken) return false;
  return [node.left, node.right].every((s) => chainShape(s) || isChainLeaf(s));
}

function hasLiteral(node: ts.Node): boolean {
  if (ts.isStringLiteralLike(node)) return true;
  if (ts.isParenthesizedExpression(node)) return hasLiteral(node.expression);
  if (ts.isBinaryExpression(node)) return hasLiteral(node.left) || hasLiteral(node.right);
  return false;
}

interface Edit {
  start: number;
  end: number;
  text: string;
}

interface Result {
  edits: Edit[];
  needs: Set<'baseUrl' | 'testDir'>;
  manual: string[];
}

function plan(file: string, source: ts.SourceFile): Result {
  const result: Result = { edits: [], needs: new Set(), manual: [] };
  const claimed = new Set<ts.Node>();

  const visit = (node: ts.Node) => {
    if (ts.isBinaryExpression(node) && isChain(node) && !claimed.has(node)) {
      // Outermost chain only: inner binary nodes are skipped via `claimed`.
      let top: ts.Node = node;
      while (ts.isParenthesizedExpression(top.parent) || (ts.isBinaryExpression(top.parent) && isChain(top.parent))) top = top.parent;
      const outer = ts.isParenthesizedExpression(top) ? (top as ts.Node) : top;
      if (!claimed.has(outer) && referencesUrlEnv(node)) {
        claim(outer);
        result.edits.push({ start: outer.getStart(source), end: outer.getEnd(), text: 'baseUrl()' });
        result.needs.add('baseUrl');
      }
    }
    const name = envName(node);
    if (name && DIR_VARS.has(name) && !insideClaimed(node)) {
      result.edits.push({ start: node.getStart(source), end: node.getEnd(), text: 'testDir()' });
      result.needs.add('testDir');
    }
    ts.forEachChild(node, visit);
  };

  const claim = (n: ts.Node) => {
    claimed.add(n);
    const mark = (c: ts.Node) => {
      claimed.add(c);
      ts.forEachChild(c, mark);
    };
    mark(n);
  };
  const insideClaimed = (n: ts.Node) => claimed.has(n);
  const referencesUrlEnv = (n: ts.Node): boolean => {
    let found = false;
    const walk = (c: ts.Node) => {
      const nm = envName(c);
      if (nm && URL_VARS.has(nm)) found = true;
      ts.forEachChild(c, walk);
    };
    walk(n);
    return found;
  };

  visit(source);

  // Any URL env read not covered by a rewritten chain needs a human.
  const sweep = (node: ts.Node) => {
    const name = envName(node);
    if (name && URL_VARS.has(name) && !claimed.has(node)) {
      const line = source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1;
      result.manual.push(`${path.relative(E2E_ROOT, file)}:${line} process.env.${name} outside a literal-fallback chain`);
    }
    ts.forEachChild(node, sweep);
  };
  sweep(source);
  return result;
}

function importLine(file: string, needs: Set<string>): string {
  let rel = path.relative(path.dirname(file), HELPER);
  if (!rel.startsWith('.')) rel = `./${rel}`;
  const names = ['baseUrl', 'testDir'].filter((n) => needs.has(n));
  return `import { ${names.join(', ')} } from '${rel}';`;
}

function applyEdits(text: string, edits: Edit[]): string {
  return [...edits].sort((a, b) => b.start - a.start).reduce((acc, e) => acc.slice(0, e.start) + e.text + acc.slice(e.end), text);
}

function main(): void {
  const check = process.argv.includes('--check');
  const files = listFiles(E2E_ROOT).filter((f) => !SKIP.some((s) => path.relative(E2E_ROOT, f).includes(s)));
  const changed: string[] = [];
  const manual: string[] = [];

  for (const file of files) {
    const text = fs.readFileSync(file, 'utf8');
    if (!/process\.env\.(TEST_SERVER_URL|BASE_URL|TEST_SERVER_TESTDIR)/.test(text)) continue;
    const source = ts.createSourceFile(file, text, ts.ScriptTarget.Latest, true);
    const { edits, needs, manual: m } = plan(file, source);
    manual.push(...m);
    if (edits.length === 0) continue;
    changed.push(path.relative(E2E_ROOT, file));
    if (check) continue;

    let out = applyEdits(text, edits);
    // Insert after the last top-level import so a leading `// @feature` comment stays first.
    const imports = source.statements.filter(ts.isImportDeclaration);
    const insertAt = imports.length ? imports[imports.length - 1].getEnd() : 0;
    // applyEdits only changes text after the last import in practice; recompute offset defensively.
    const shift = out.length - text.length;
    const lastEditBeforeImport = edits.some((e) => e.start < insertAt);
    if (lastEditBeforeImport) throw new Error(`${file}: env read precedes the last import; migrate by hand`);
    void shift;
    const line = importLine(file, needs);
    out = out.slice(0, insertAt) + (imports.length ? '\n' : '') + line + (imports.length ? '' : '\n') + out.slice(insertAt);
    fs.writeFileSync(file, out);
  }

  console.log(`${check ? 'would change' : 'changed'} ${changed.length} files`);
  if (manual.length) console.log(`needs a human (${manual.length}):\n  ${manual.join('\n  ')}`);
  if (check && (changed.length || manual.length)) process.exit(1);
}

main();

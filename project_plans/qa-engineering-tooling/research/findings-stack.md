# Findings: Stack

## Summary

This QA engineering tooling project requires stack decisions across five dimensions: Go AST analysis for backend feature discovery, TypeScript/React AST analysis for frontend discovery, Playwright video recording for CI-compatible capture, Claude API integration for intelligent UX analysis, and GitHub Actions artifact workflow. The Stapler Squad codebase already uses Go 1.25, TypeScript/React 19, ConnectRPC for RPC transport, Playwright for E2E testing, and protobuf for service definitions.

Language-native solutions dominate the analysis layer: Go's standard `go/ast` for the backend scanner, TypeScript Compiler API for the frontend scanner. Playwright-native APIs handle video capture (already configured). Claude API vision provides the intelligence layer for UX analysis. GitHub Actions + gh CLI handles PR artifact attachment.

Integration complexity is moderate. The critical friction point is CI video artifact upload and PR attachment. All other decisions have clear leading options with low adoption risk.

---

## Options Surveyed

### 1. Go AST Analysis (Backend Feature Discovery)

#### Option A: `go/ast` + `golang.org/x/tools` (Recommended)
- Zero external dependencies beyond Go SDK
- Parses full Go AST; walks function definitions, method receivers, struct types
- `golang.org/x/tools` provides type checking, call graph analysis
- Handles comments and build tags natively
- Used by `go vet`, `go fmt`, Go LSP — battle-tested
- Can cross-reference between `_pb.go` (generated protobuf) and handler implementations
- Weaknesses: custom logic needed to match ConnectRPC handler signatures; no pre-built "find all RPC handlers"
- Maintenance: Low — Go stdlib is stable

#### Option B: grpc-gateway + protoc plugins
- Purpose-built for RPC method discovery from proto files; generates OpenAPI/Swagger
- **Rejected**: gRPC-centric; ConnectRPC requires adaptation; adds build-time code generation dependency; overkill for internal feature registry

#### Option C: ast-grep (External AST Query Tool)
- Tree-sitter-based structural code search; human-readable pattern queries
- **Secondary**: useful for cross-language consistency if both scanners use same tool; slower than native Go AST; external binary dependency; 95% grammar accuracy

**Recommendation**: `go/ast` + `golang.org/x/tools` — language-native, zero deps, integrates with Go 1.25 toolchain.

---

### 2. TypeScript/React AST Analysis (Frontend Feature Discovery)

#### Option A: TypeScript Compiler API (Recommended)
- Official TypeScript compiler as library; full language understanding
- Extracts component props, exported types, JSX element trees
- Full cross-file analysis (imports, re-exports, interfaces)
- Used by VSCode, ts-node, most LSP implementations
- Can verify component prop types match backend API responses
- Weaknesses: steep API learning curve; verbose boilerplate; slower on large codebases (though Stapler Squad is medium-size)
- Maintenance: Low — TypeScript team maintains; stable API

#### Option B: ts-morph
- Wrapper around TS compiler API with higher-level abstractions (`.getExportedDeclarations()`, etc.)
- Cleaner ergonomics; less boilerplate
- **Backup**: use if raw TS compiler API becomes blocker; same underlying semantics

#### Option C: Babel Parser
- Fast; good JSX support
- **Rejected**: no TypeScript type information; can't verify component prop types; no import/export cross-referencing

#### Option D: ast-grep (Structural Query)
- Declarative component signature queries; consistent with Go scanner
- **Secondary**: loses TypeScript type information; 95% accuracy; useful for pattern-based discovery on top of TS scanner

**Recommendation**: TypeScript Compiler API — full type system, cross-file analysis, battle-tested. Use ts-morph if ergonomics become a problem.

---

### 3. Playwright Video Capture (CI-Compatible Recording)

#### Option A: Playwright Native Video Recorder (Recommended)
- Built into Playwright test runner; zero additional dependencies
- Already configured in `playwright.config.ts` — recording is ready to use [TRAINING_ONLY - verify current config]
- Works in headless mode; uses VP8 codec internally
- Granular control: record per-test, per-context, on-failure only
- Weaknesses: 10-30% CPU/disk overhead during test runs; ffmpeg must be available in CI (GitHub Actions includes it); videos ~50-500 MB depending on duration
- CI compatibility: High — tested on GitHub Actions
- Maintenance: None — Playwright maintains

#### Option B: Puppeteer + FFmpeg
- **Rejected**: Stapler Squad already uses Playwright; adding Puppeteer duplicates dependencies; manual FFmpeg coordination is complex

#### Option C: DevTools Protocol (Manual)
- **Not viable**: extreme complexity; no community best practices; 1% of value vs Playwright native

**Recommendation**: Playwright native video — already configured, zero deps, CI-ready.

---

### 4. Claude API for UX Analysis (Vision Model)

#### Option A: Claude API Vision (Recommended)
- Use `claude-opus-4-6` or `claude-sonnet-4-6` for visual reasoning
- Vision input via `image/webp` base64-encoded; tool use for structured JSON output
- Handles color contrast, layout consistency, typography hierarchy, accessibility heuristics
- Can cross-reference design system tokens (from `globals.css`) against screenshots
- Cost: ~$0.01-0.05 per screenshot analyzed [TRAINING_ONLY - verify current pricing]
- Weakness: hallucination risk on UI analysis; non-deterministic; pair with deterministic tools (Axe, Lighthouse)
- Maintenance: Low — Anthropic maintains API; stable contract

Integration pattern:
```typescript
import Anthropic from "@anthropic-ai/sdk";
import fs from "fs";

const client = new Anthropic();

async function analyzeUX(screenshotPath: string): Promise<object> {
  const imageData = fs.readFileSync(screenshotPath);
  const base64Image = imageData.toString("base64");

  const response = await client.messages.create({
    model: "claude-opus-4-6",
    max_tokens: 2048,
    tools: [{
      name: "report_ux_findings",
      description: "Report structured UX analysis findings",
      input_schema: {
        type: "object",
        properties: {
          accessibility_issues: { type: "array", items: { type: "string" } },
          contrast_problems: { type: "array", items: { type: "string" } },
          layout_issues: { type: "array", items: { type: "string" } },
        },
      },
    }],
    messages: [{
      role: "user",
      content: [
        { type: "image", source: { type: "base64", media_type: "image/webp", data: base64Image } },
        { type: "text", text: "Analyze this UI for accessibility, contrast, and design consistency issues." },
      ],
    }],
  });

  const toolUse = response.content.find((c) => c.type === "tool_use");
  return toolUse?.input || {};
}
```

#### Option B: GPT-4V
- Similar capabilities; adds vendor lock-in; no cost advantage — **not recommended**

#### Option C: Open-source vision models (LLAMA Vision, etc.)
- 20-30% accuracy below commercial models; requires GPU infrastructure — **not recommended for MVP**

#### Option D: SaaS tools (Percy, Chromatic, Applitools)
- Turnkey but $100-500+/month; overkill for solo developer — **not recommended**

**Recommendation**: Claude API vision + Axe Core + Lighthouse. Claude handles subjective quality; Axe/Lighthouse handle objective compliance.

---

### 5. GitHub PR Workflow (Video + Report Attachment)

#### Option A: GitHub Actions + gh CLI (Recommended)
- `actions/upload-artifact` stores videos; `gh pr comment` attaches links to PR
- Already used in Stapler Squad codebase (CLAUDE.md references gh CLI)
- Can filter on file changes: `paths: ['web-app/**']` in workflow triggers
- GitHub token auto-available in Actions context; no credential management needed
- Weakness: artifact retention limits (~5GB free tier, 30-day default); fork PRs require special handling
- Maintenance: Low — GitHub-maintained

Example workflow:
```yaml
name: E2E Video Capture
on:
  pull_request:
    paths: ['web-app/**']
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
      - run: npm install
      - run: npm run test:e2e
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: e2e-videos
          path: web-app/test-results/videos/
          retention-days: 7
      - run: |
          gh pr comment ${{ github.event.pull_request.number }} \
            --body "E2E videos: [Actions run](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }})"
        env:
          GITHUB_TOKEN: ${{ github.token }}
```

#### Option B: S3 + Presigned URLs
- Unlimited storage; flexible retention — **not recommended for MVP**; adds AWS credential overhead

#### Option C: Slack/Discord
- **Not recommended**: solo developer; duplicates PR information

#### Option D: GitHub Pages
- Persistent index; complex setup — **future option** after workflow proven

**Recommendation**: GitHub Actions + gh CLI — native, low complexity, sufficient for MVP.

---

## Trade-off Matrix

| Dimension | Option | Fit | Key Trade-off | Decision |
|-----------|--------|-----|---------------|----------|
| Go AST | `go/ast` + `x/tools` | Language-native, zero deps | Custom handler matching logic | ✅ PICK |
| Go AST | grpc-gateway | Purpose-built | gRPC-specific; not ConnectRPC | ❌ |
| Go AST | ast-grep | Cross-language | Slower; external binary; 95% accuracy | 🟡 Secondary |
| TS/React | TS Compiler API | Full type system, native | Steep API | ✅ PICK |
| TS/React | ts-morph | Cleaner abstractions | Wrapper indirection | 🟡 Backup |
| TS/React | Babel Parser | Fast, JSX native | No types; TypeScript blind | ❌ |
| Video | Playwright native | Already configured, CI-ready | CPU/disk overhead | ✅ PICK |
| Video | Puppeteer + FFmpeg | Full control | Manual; external deps; duplicate dep | ❌ |
| UX Analysis | Claude API vision | Best accuracy; tool use | Cost per request; hallucination risk | ✅ PICK |
| UX Analysis | SaaS (Percy, etc.) | Turnkey | $100-500+/month; overkill | ❌ |
| UX Analysis | Open-source vision | Private; no cost | 20-30% accuracy loss | ❌ MVP |
| PR Workflow | GitHub Actions + gh | Native; already used | Artifact retention limits | ✅ PICK |
| PR Workflow | S3 + presigned | Unlimited storage | AWS overhead; cost | 🟡 Future |

---

## Risk and Failure Modes

### 1. Go AST Handler Matching False Positives/Negatives
- ConnectRPC handler edge cases: variadic args, generated code, embedded interfaces
- Mitigation: narrow pattern matching (methods on types named `*Server`); validate against actual handler registration in `server/server.go`; manual review phase before declaring production-ready

### 2. TypeScript Compiler API Performance
- Running TS compiler on large codebase may slow CI
- Mitigation: profile first (expect 2-5s on Stapler Squad's ~50k TS LOC); cache compiled AST; scan only changed files via `git diff` filter; incremental analysis

### 3. Playwright Video CI Flakiness
- FFmpeg availability in GitHub Actions runners; silent recording failures
- Mitigation: test locally first; explicitly check for `.webm` files in artifacts; fail job if missing; monitor CI logs for FFmpeg errors; GitHub Actions Ubuntu includes FFmpeg

### 4. Claude API Vision Accuracy
- Model misidentifies elements; false positives on valid contrast/layout
- Mitigation: narrow, focused screenshots + specific prompts; validate against Axe/Lighthouse; require manual review before action; use tool use for structured JSON; test on sample screenshots first

### 5. CI Artifact Storage Quota
- Long test suite + high-res videos can exceed 5GB
- Mitigation: compress to VP8 at reduced bitrate; retention-days: 7; upload only on failure; monitor Actions storage usage

### 6. PR Comment Permissions on Fork PRs
- `GITHUB_TOKEN` lacks comment permissions on external fork PRs
- Mitigation: restrict video/comment workflow to non-fork PRs; document limitation

### 7. AST Scanner Maintenance with Language Evolution
- Go 1.26+, TypeScript 5.5+ may introduce syntax changes
- Mitigation: language-native parsers evolve with language; add version pin tests verifying scanner works on Go 1.25+, TypeScript 5.3+

---

## Migration and Adoption Cost

| Phase | Effort | Adoption Risk | Rollback |
|-------|--------|---------------|----------|
| Go AST scanner | 1-2 weeks | Low (read-only output) | Easy |
| TS Compiler scanner | 1-2 weeks | Low (read-only output) | Easy |
| E2E harness (Playwright) | 2-3 weeks | Medium (CI gate) | Disable tests |
| Claude API UX analysis | 1 week | Low (advisory) | Disable API calls |
| PR video workflow (Actions) | 3-5 days | Medium (CI complexity) | Remove workflow |

**Total**: 6-10 weeks sequential; 3-4 weeks if parallelized.
**Ongoing**: ~$10-100/month Claude API depending on PR volume and screenshot count.

---

## Operational Concerns

**Scanner drift**: Scanner patterns must evolve with codebase. Mitigation: include scanner tests in `make pre-commit`; verify known features still discovered after refactors.

**CI cost**: Video recording 10-30% CI time overhead (acceptable for 5-10 min suites). Claude API ~$0.10-0.50/PR for 10 screenshots. At 100 PRs/month = $10-50/month. Acceptable; use Batch API at >10 screenshots/PR.

**Disk space in CI**: Typical video 50-200MB; GitHub Actions runners have ~14GB available — sufficient for 50+ videos. Set `retention-days: 7`.

**Cross-tool data flow**: Each tool is independent with file-based outputs:
- Scanners → JSON registry files (committed to repo)
- E2E → Videos (uploaded as CI artifacts)
- UX analysis → JSON report (posted to PR comment)
- No circular dependencies; each stage independently iterable

---

## Prior Art and Lessons Learned

1. **Go AST scanning**: Language-native AST walkers are more reliable than external tools. Regex-based handler discovery is fragile. Feature discovery works best tied to service definitions (proto, OpenAPI schema).

2. **Frontend component catalogs**: Type-driven discovery (TS interfaces for props) is more accurate than heuristics. Manual component tagging still needed; scanners identify candidates, humans validate.

3. **Playwright in CI**: Video recording overhead is acceptable; improves debugging significantly. CI-first design critical — tests must pass reliably in headless mode.

4. **AI-assisted UX review** [TRAINING_ONLY]: Vision models (GPT-4V, Claude) ~80-90% accurate for design system compliance; ~40-50% for subjective UX quality. Hallucination rate drops with specific prompts and tool use. Human review remains critical.

5. **GitHub Actions artifact workflows**: Native CI integration is best; avoids external dependencies. Artifact retention must be tuned. PR comments for visibility are critical for adoption.

---

## Open Questions

- [ ] **Handler discovery scope**: Top-level RPC handlers only, or also middleware/interceptors? Recommend: handlers only (80/20 rule). Blocks: Go scanner scope definition.
- [ ] **Component prop validation**: Should frontend scanner verify prop types match backend API response types? Recommend: optional cross-referencing in Phase 2. Blocks: frontend scanner schema design.
- [ ] **Video resolution in CI**: 1280x720 sufficient, or also mobile viewport (375x667)? Recommend: 1280x720 MVP. Blocks: Playwright config changes.
- [ ] **UX analysis frequency**: Every UI PR vs. on-demand only? Recommend: PRs touching `web-app/` only. Blocks: workflow trigger configuration.
- [ ] **Feature registry persistence**: JSON file in repo (version-controlled) vs. live service? Recommend: JSON file in repo — simpler, diffable, no service needed.
- [ ] **Batch API threshold**: When to switch from on-demand to Batch API? Recommend: >10 screenshots/PR or >100/day.

---

## Recommendation

**Stack selection:**

| Layer | Tool | Rationale |
|-------|------|-----------|
| Backend scanner | `go/ast` + `golang.org/x/tools` | Language-native, zero deps, battle-tested |
| Frontend scanner | TypeScript Compiler API | Full type system, cross-file analysis |
| Video recording | Playwright native | Already configured; CI-ready; zero extra deps |
| UX analysis | Claude API Opus 4.6 | Best visual reasoning; tool use for structured JSON |
| PR workflow | GitHub Actions + gh CLI | Native integration; already used in codebase |

---

## Pending Web Searches

1. `"ConnectRPC handler AST parsing Go golang.org/x/tools 2024"` — verify go/ast patterns for ConnectRPC specifically; confirm no built-in reflection API
2. `"TypeScript compiler API component discovery react props ts-morph 2025"` — compare ergonomics vs raw TS API; verify API stability in TS 5.3+
3. `"Playwright video recording headless CI GitHub Actions ffmpeg configuration"` — confirm FFmpeg availability in latest runners; verify `.webm` vs `.mp4` options
4. `"Claude API vision model UI accessibility analysis tool use structured output 2025"` — verify Opus 4.6 supports tool use for vision; confirm pricing
5. `"GitHub Actions gh pr comment artifact upload permissions fork PRs 2025"` — verify permissions in fork PR contexts; best practices for secure artifact handling
6. `"Playwright test video recording performance overhead CPU memory impact"` — quantify overhead; settings for low-overhead recording
7. `"ast-grep Tree-sitter Go TypeScript grammar accuracy false positives edge cases"` — confirm grammar accuracy rates; document known edge cases (generics, complex types, JSX)
8. `"Claude Batch API cost savings vision tasks 2025"` — verify Batch API availability for vision; confirm cost savings threshold

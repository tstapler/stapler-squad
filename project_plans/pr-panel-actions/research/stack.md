# PR Panel Actions — Technology Stack Research

## 1. GitHub GraphQL Mutations for Draft Toggle

GitHub's GraphQL API exposes two mutations for draft state:

- **`convertPullRequestToDraft(input: { pullRequestId: ID! })`** — converts a ready PR back to draft. Returns `pullRequest { isDraft }`.
- **`markPullRequestReadyForReview(input: { pullRequestId: ID! })`** — promotes a draft to ready. Returns `pullRequest { isDraft }`.

Both take the PR's node ID (not the number). The node ID must be fetched via the existing `graphQLPRInfoQuery` — it is not currently in `prInfoGraphQLNode`; a new field `id` must be added to the query and struct.

**Current state of `github/client_graphql.go`:** Only contains `graphQLPRInfoQuery` (a query, not a mutation). No mutations exist yet. The infrastructure for issuing GraphQL calls (`newGHGraphQLRequestForHostWithToken`, `ghHTTPClient`, `graphQLError` from `user_pr_cache.go`, `graphQLErrorsToError`) is already in place and reusable.

Pattern to follow: same POST-to-graphql-endpoint approach as `GetPRInfoGraphQL` in `github/client_graphql.go` — marshal `{"query": mutationString, "variables": {...}}`, call `newGHGraphQLRequestForHostWithToken`, decode response.

**`github/client.go` MergePR / ClosePR:** Both shell out via `gh pr merge` / `gh pr close` (subprocess via `safeexec`). The new `MergePRDirect` and `ClosePRDirect` RPCs can either reuse these directly from the service layer (no new Go function needed) or wrap them in thin service-layer calls.

## 2. Protobuf / buf Generation Setup

- Config: `buf.gen.yaml` at repo root (version v2).
- Command: `make proto-gen` → runs `buf generate proto`.
- **Go output:** `gen/proto/go/` (plugin: `buf.build/protocolbuffers/go` + `buf.build/connectrpc/go:v1.20.0`, both with `paths=source_relative`).
- **TypeScript output:** `web-app/src/gen/` (plugin: `web-app/node_modules/.bin/protoc-gen-es`, target=ts).
- Generated files are **gitignored** — commit only `.proto` source changes; `make build` regenerates automatically via the `proto-gen` stamp dependency.
- New RPCs in `proto/session/v1/github_user.proto` and new fields in `proto/session/v1/types.proto` are picked up by a single `make proto-gen` run.

## 3. Frontend ConnectRPC Client Pattern

Two transport singletons in `web-app/src/lib/api/transport.ts`:

- **`getConnectTransport()`** — HTTP ConnectRPC (`createConnectTransport`), 30 s default timeout, auth interceptor. Used for all unary RPCs.
- **`getWatchTransport()`** — WebSocket-bridge or native streaming transport (`createSessionWatchTransport`). Used for server-streaming Watch* RPCs.

Hook pattern (from `useNudgePR.ts`):
```ts
const client = createClient(GitHubUserService, getConnectTransport());
const req = create(SomeRequestSchema, { ... });
const res = await client.someRpc(req);
```

Imports come from `@/gen/session/v1/github_user_pb` (generated) and `@bufbuild/protobuf` (`create`). Error handling uses `ConnectError.from(err)` and `Code` from `@connectrpc/connect`.

The new action RPCs (MergePRDirect, ClosePRDirect, ConvertToDraft, MarkReadyForReview) are unary and should use `getConnectTransport()`.

## 4. Vanilla-Extract CSS Patterns for Action Buttons

`PRCard.css.ts` defines two action button styles that new PR action buttons should match:

- **`openSessionButton`** — primary action: `vars.color.accentBg` background, `vars.color.inputFocusBorder` color/border, hover → `vars.color.accentHover`.
- **`createSessionButton`** — secondary/ghost action: transparent background, `vars.color.borderColor` border, hover → `vars.color.primary` border+color.

Both share: `minHeight: INTERACTIVE_MIN` (44 px WCAG 2.5.5 touch target), `padding: vars.space["1"] vars.space["3"]`, `borderRadius: vars.radii.sm`, `fontSize: vars.fontSize.xs`, `fontWeight: 600`, `display: inline-flex`, `alignItems: center`, `whiteSpace: nowrap`.

They live inside `prActions` (flex row, wraps on narrow screens to column + stretch). New destructive actions (close, merge) should follow the same structural pattern but reference appropriate semantic color tokens (e.g. a danger variant — check `vars.color` for any existing `danger`/`error` token pair, or add one following `CHIP_TOKEN_PAIRS` conventions in `prChipTokens.ts`).

`chipInteractive` (a chip that is also a button) adds `:focus-visible` outline; action buttons should do the same.

## 5. Summary: What Already Exists vs. What Needs to Be Built

| Concern | Status |
|---|---|
| GraphQL query for PR info | Exists (`client_graphql.go`) |
| GraphQL mutation infrastructure | Exists (request builder, HTTP client, error types) |
| `convertPullRequestToDraft` mutation | **Missing — needs new function** |
| `markPullRequestReadyForReview` mutation | **Missing — needs new function** |
| PR node ID in query/struct | **Missing — add `id` field to `graphQLPRInfoQuery` and `prInfoGraphQLNode`** |
| `MergePR` / `ClosePR` Go functions | Exist (`client.go`), shell out via `gh` CLI |
| `GitHubUserService` RPCs for actions | **Missing — add to `github_user.proto`** |
| `UserPR` extended fields (body, labels, stats, reviewers) | Partially exists (`check_conclusion`, `approved_count`, `changes_req_count`, `failing_checks`) — body, reviewer list not yet in proto |
| Frontend transport | Exists (`getConnectTransport`) |
| Action button CSS tokens | Exist (`openSessionButton`, `createSessionButton` in `PRCard.css.ts`) |

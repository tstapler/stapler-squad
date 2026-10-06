# jules/testdata fixtures

Golden fixtures for `jules/golden_test.go`, decoded through the real DTOs in
`jules/client.go` so a Jules API schema change is caught by a failing test
instead of silently absorbed by a hand-written mock.

| File | Endpoint | Recorded |
|---|---|---|
| `session_created.json` | `POST /v1alpha/sessions` | 2026-09-01 (hand-authored from research/stack.md §Sessions and architecture.md §0 field shapes — see note below) |
| `session_completed.json` | `GET /v1alpha/sessions/{session}` | 2026-09-01 (as above) |
| `session_failed.json` | `GET /v1alpha/sessions/{session}` | 2026-09-01 (as above) |
| `sources_list.json` | `GET /v1alpha/sources` | 2026-09-01 (as above) |

**Note on provenance**: these fixtures were authored from the documented
field shapes in `project_plans/google-jules-integration/research/stack.md`
and `research/architecture.md` (`name`, `id`, `state`, `title`,
`outputs[].pullRequest.{url,title,description}`, `createTime`, `updateTime`,
`url`; source names as `sources/github-{owner}-{repo}`), not from a live API
call — an alpha API key with a connected GitHub source was not available at
implementation time. This closes the two Unresolved Questions the plan asked
this task to close, with the answer "unknown, not yet recorded from a live
response":

- **Cost field**: no cost/usage field is documented in stack.md or
  architecture.md, so none is included in these fixtures.
- **Multiple `outputs[]` entries**: not observed; each fixture here has at
  most one `outputs[]` entry.

**Re-recording against the real API** (once a Jules account + connected
GitHub source is available), replacing the note above with the real
observation:

```bash
curl -H "x-goog-api-key: $JULES_API_KEY" https://jules.googleapis.com/v1alpha/sessions/<id>
curl -H "x-goog-api-key: $JULES_API_KEY" https://jules.googleapis.com/v1alpha/sources
```

Save the raw response body as the matching fixture file, update the table
above with the actual recording date, and update this note to record what
was actually observed for the cost field and `outputs[]` cardinality.

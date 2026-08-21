# Merge Plan: personal fork → upstream — 2026-04-20

## Summary

| | |
|---|---|
| Fork (personal) | `personal` → `tstapler/stapler-squad` |
| Upstream (work) | `origin` → `TylerStaplerAtFanatics/stapler-squad` |
| Merge base | `6a592084` (chore(bench): update go tier2 baseline) |
| Base date | ~2026-04-18 |
| Fork ahead by | 50+ commits (substantive + 24 baselines) |
| Upstream ahead by | **0 commits** — origin/main IS the merge base |
| Conflict files | Resolved via personal/main checkout for conflicted CSS files |
| Complexity | **MEDIUM** — CSS migration (257 files) required careful ordering |

**Direction**: Upstream changes from `personal/main` → `origin/main`.

---

## Commit Classification

### Substantive commits cherry-picked (33 total)

#### Frontend: vanilla-extract CSS migration (PR #22)
| SHA | Message | Bucket |
|---|---|---|
| `9bbb9158` | feat(web): complete front-end refactor phases 1–4 | FEATURE |
| `ae964dd1` | fix(web): fix invalid vanilla-extract selectors and CI lint step | FIX |
| `e72e1686` | fix(css): replace invalid vanilla-extract child selectors with globalStyle | FIX |
| `29c2b55b` | fix(css): replace all remaining invalid child selectors with globalStyle | FIX |
| `68b90225` | fix(css): fix remaining invalid selectors in ActionBar, NotificationToast, VcsPanel | FIX |
| `520f1e47` | fix(css): force-track logs/ CSS files hidden by global gitignore rule | FIX |
| `697886e3` | chore: override global gitignore to track src/logs/ source files | INFRA |
| `1587b72f` | fix(css): track remaining gitignored logs CSS files; fix skeleton module.css imports | FIX |
| `9e12f68d` | fix(web): cast styles to Record for dynamic badge class lookup | FIX |
| `d3d67600` | fix(web): cast styles to Record for dynamic CSS class lookups | FIX |
| `43b1f79c` | fix(web): add missing debugActive export to TerminalOutput.css.ts | FIX |
| `5c61cac6` | fix(web): cast boxSizing !important values to satisfy TypeScript | FIX |
| `9062e567` | fix(web): rename suggestions CSS import to avoid prop name collision | FIX |
| `38c14d81` | fix(web): rename CSS imports in KeyboardHint to avoid prop name collisions | FIX |
| `46463bfb` | fix(web): rename steps CSS import in Wizard to avoid prop name collision | FIX |
| `89f0b0f0` | fix(web): double-cast toPlainObject result through unknown for PlainApproval | FIX |

#### Mobile terminal UX
| SHA | Message | Bucket |
|---|---|---|
| `7a8bb003` | feat(mobile): touch gestures, Termux keyboard, image paste, large-paste chunking | FEATURE |

#### MCP server (9 commits)
| SHA | Message | Bucket |
|---|---|---|
| `2fd96745` | feat(mcp): add MCP server with 15 tools for session automation | FEATURE |
| `c19b06fc` | fix: address all Copilot review comments | FIX |
| `4e969705` | fix(mcp): resolve staticcheck lint failures in test files | FIX |
| `630dd567` | style: gofmt image_upload_handler.go | REFACTOR |
| `12e7b58b` | fix(mcp): apply gofmt formatting to MCP server files | REFACTOR |
| `3bad3a31` | style: gofmt image_upload_handler_test.go | REFACTOR |
| `e13da341` | fix(mcp): address Copilot review bugs | FIX |
| `06e011ba` | feat(mcp): add HTTP/SSE transport and auto-inject MCP URL into sessions | FEATURE |

#### System service + infra
| SHA | Message | Bucket |
|---|---|---|
| `309856b4` | feat(service): add system service installation for Linux and macOS | FEATURE |
| `c2a083f3` | chore: untrack built binaries and dist artifacts | INFRA |

#### tmux robustness
| SHA | Message | Bucket |
|---|---|---|
| `67375bda` | fix(tmux): drain PTY output and pre-size attach connections to fix stuck resize | FIX |
| `bdb860d3` | fix(tmux): remove reap goroutine from pty.go to eliminate data race | FIX |
| `627ff86a` | fix(tmux): remove drain goroutines and store verified resize dimensions | FIX |

#### Planning docs + cleanup
| SHA | Message | Bucket |
|---|---|---|
| `c65ba52c` | docs: add MDD planning artifacts for MCP server and llm-omnibar | DOCS |
| `71127707` | style: gofmt server/server.go, session files | REFACTOR |
| `03457b6c` | fix(session): protect exitTail writes with mutex to prevent data race | FIX |

### Skipped (BASELINE — 24 commits)
All `chore(bench): update * baseline [skip ci]` — regenerate automatically after merge.

### Skipped (PRIVATE — 1 commit)
- `29cdba48` — docs: Add feature plan for llm-omnibar (future unimplemented feature)

---

## Conflict Resolution Notes

**CSS migration (9bbb9158)** had 39 conflicts:
- 18 `UD` conflicts: `.module.css` files deleted by migration → accepted deletion
- 1 `DU` conflict: `server/web/dist/index.html` deleted by upstream → kept deleted
- 20 `UU` conflicts: `.tsx` files where migration didn't know about newer upstream features (useTerminalSnapshot, useFocusTrap) → resolved by taking `personal/main` final version, which has both migration AND newer features

**CSS fix commits**: All applied cleanly after migration was in place.

**Mobile feature (7a8bb003)**: Tried to modify `.module.css` files deleted by migration → took `personal/main` CSS final versions (`.css.ts`) which correctly incorporate mobile styles.

---

## Post-Merge Checklist

- [ ] `make build` passes
- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] No `chore(bench): update * baseline` commits included
- [ ] `29cdba48` (llm-omnibar plan) NOT included
- [ ] PR semver label applied (`minor`)
- [ ] CI green on origin before merge
- [ ] After CI runs, benchmark baselines auto-regenerate

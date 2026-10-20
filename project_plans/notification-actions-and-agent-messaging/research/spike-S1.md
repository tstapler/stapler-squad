# Spike S1: real AskUserQuestion payload and hook-answer behavior in the interactive TUI

**Result: PASS (S1a payload captured, S1b hook answer, S1c navigation).** Results await owner review (plan owner item 8); the plan default proceeds meanwhile: ADR-005 rung 1 (`hook_reply`) stays in scope and Task 3.4c / Story 3.5 are enabled.

- Date: 2026-10-18. Claude Code `2.1.303` (`command claude --version`), model `haiku` (Haiku 5.5), permission mode `auto` (shown in the hook payload as `"permission_mode":"auto"`).
- Harness: private tmux server `tmux -L nam-spikeA` (200x50), scratch project outside the repo with `.claude/settings.local.json` registering a `PreToolUse` hook, matcher `AskUserQuestion`, command = a throwaway python script that logs stdin and returns output chosen by a mode file. The ssq manual instance and the live service were not touched, so `ssq-hooks check` was not used; the script logs the raw JSON instead (same stdin).
- Scope limit: interactive TUI only, one model, auto permission mode. Other permission modes were not tried.

## S1a: captured payloads (VERIFIED)

Raw PreToolUse stdin for a 2-question call (sanitized in fixtures: session id, transcript path, cwd, scratchpad path, tool_use_id, prompt_id dropped):

```json
{"session_id":"...","transcript_path":"...","cwd":"...","scratchpad_dir":"...","prompt_id":"...","permission_mode":"auto","effort":{"level":"medium"},"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Which cache?","header":"Cache","options":[{"label":"Redis","description":"In-memory data store ..."},{"label":"Memcached","description":"..."},{"label":"None","description":"Do not use a cache"}],"multiSelect":false},{"question":"Which letters?","header":"Letters","options":[{"label":"A","description":"Option A"},{"label":"B","description":"Option B"},{"label":"C","description":"Option C"}],"multiSelect":true}]},"tool_use_id":"toolu_..."}
```

Fixtures (all under `server/services/testdata/askuserquestion/`, each holds `hook_event_name`, `tool_name`, `permission_mode`, `tool_input`):

| File | Shape |
|---|---|
| `single_select.json` | 1 question, `multiSelect:false`, 3 options |
| `multi_select.json` | 1 question, `multiSelect:true`, options A/B/C |
| `two_questions_single_then_multi.json` | Q1 single (Redis/Memcached/None), Q2 multi (A/B/C) |
| `multi_two_questions.json` | the story's AC fixture: `questions[0].options[0].label == "A"`, `questions[1].multiSelect == true` (checked: prints `A True`) |

**Divergences from `docs/tasks/askuserquestion-ui.md:13-22`** (that doc is wrong for 2.1.303):
- It shows `tool_input: {prompt, options: ["PostgreSQL", ...]}`. Real: `tool_input.questions[]`, each `{question, header, options:[{label, description}], multiSelect}`. There is no top-level `prompt` and options are objects, not strings. `tool_input` has only the key `questions` (verified).
- The envelope also carries `permission_mode`, `effort`, `prompt_id`, `scratchpad_dir`, `transcript_path`, `tool_use_id`.
- Up to 2 questions were exercised; the 1-4 bound is from the official schema, not measured here.
- Every option carries a `description` even when the prompt did not ask for one (the model fills it).

## S1b: hook answer (VERIFIED, PASS)

Hook output: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","updatedInput":{"questions":[...],"answers":{"<question text>":"<label>"}}}}` (multi-select answer is the labels joined with `, `).

| Case | Hook picked | Tool result shown in TUI | Agent text |
|---|---|---|---|
| single | option 2 | `User answered Claude's questions: · Which cache? → Memcached` | "The answer is Memcached." |
| multi (1 question) | options 2-3 | `· Which letters? → B, C` | "The answer is B and C." |
| 2 questions | Memcached; B, C | `· Which cache? → Memcached` / `· Which letters? → B, C` | "Cache: Memcached. Letters: B and C." |
| single via HTTP hook | option 2 | `· Which db? → SQLite` | "SQLite" |

Option 2 was chosen on purpose so the answer cannot be confused with the dialog default (option 1). Dialog suppression: after sending the prompt the pane was polled every 0.2 s for 100 polls (~20 s) and grepped for `Enter to select`: `frames_with_dialog=0 of 100`. No numbered selector was ever drawn; the spinner read `running PreToolUse hooks`.

## S1c: navigation sequence (VERIFIED, PASS)

Frames are in `session/testdata/reply_dialogs/` beside `question_*.txt`: `claude_auq_twoq_step1_single_select.txt`, `claude_auq_twoq_step2_multiselect_initial.txt`, `..._AB_toggled.txt`, `..._submit_focused.txt`, `claude_auq_twoq_review_submit.txt`, plus the key list `claude_auq_twoq_keys.txt`.

Sequence (tmux `send-keys`, 0.7 s apart): `Enter Enter Down Enter Down Down Down Enter Enter`.
- Q1 (single): `Enter` selects the focused option 1 and advances to Q2 (no digit needed; a digit key was not tested here).
- Q2 (multi): `Enter` toggles the focused row (`[ ]` to `[✔]`) and does not advance; `Down` moves; after the last option the focus order is `Type something` then an unnumbered `Submit` row; `Enter` on `Submit` opens a `Review your answers` screen whose default `1. Submit answers` takes one more `Enter`.
- Replayed in a fresh `claude` process (new session) with a Q1 A/B/C + Q2 multi A/B/C dialog: result `· Which letters first? → A` / `· Which letters also? → A, B`. The tool result carries both answers, so the Story 0.1 S1c criterion is met. (The first pass in the original process gave Redis / A, B.)

**Check against ADR-010 `DialogMatch` (`session/pane_reply.go:69-110`, read, not executed against these frames: INFERRED):**
- Footer: prefix `Enter to select` still holds. Text varies by shape (`↑/↓ to navigate` single; `Tab/Arrow keys to navigate` in the tabbed dialog; `ctrl+g to edit in Nvim` appears when the Submit row is focused), so only the prefix may be matched. It is the last non-blank line in every captured frame, so the bottom anchor holds.
- Header: a multi-question dialog draws a tab bar `←  ☐ Cache  ☐ Letters  ✔ Submit  →` instead of a bare `☐ <header>` line. `DialogMatch` requires a line equal to `☐ <header>`, so it would not match: fails closed. Story 3.5 needs a separate matcher for the tabbed layout.
- Rows: `5. Chat about this` is drawn below a separator but above the footer; multi-select rows read `1. [ ] A` and the `Submit` row is unnumbered. The `n+2` numbered-row count of the single-select matcher happens to hold for multi, but row text includes the checkbox.
- Single-select keystroke (ADR-010) is unchanged by this spike; the existing 2.1.296 fixtures still parse by eye against 2.1.303 frames.

## Plan consequences

- ADR-005 rung 1 stays; Task 3.4c enabled; Story 3.5 (stepwise multi-select / multi-question) enabled, with its own tab-bar matcher.
- Caveat for the hook route: the answers object is keyed by the full question text, so `updatedInput.questions` must be echoed back unchanged.
- Conditional-story tests stay written; no skip needed for S1/S1c.

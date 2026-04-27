# Requirements: One-Off Sessions

## Problem Statement

Users want to quickly spin up a Claude session in a fresh, isolated directory for exploratory or one-off work on any project — without needing a git repo, without picking a branch, and without having to manually create and name a directory first. Today, all managed sessions require a path to an existing directory (or repo). There's no friction-free "just start working" path.

## Goal

Add a new **one-off session** creation mode that automatically creates a fresh directory with a generated name and starts a `SessionTypeDirectory` session inside it. The user provides a session title; everything else is generated or configured.

---

## User Stories

### US-1: Create a one-off session from the web UI
**As a** user,  
**I want to** click a "One-off session" option in the new-session dialog,  
**So that** I get a fresh Claude session in a new directory immediately — without having to pick a path or name the directory myself.

**Acceptance Criteria:**
- A "One-off session" option is visible and selectable in the session creation UI (new session page / omnibar).
- When selected, the directory field is hidden; only a session title field is shown.
- On submit, a new directory is created at `<one_off_base_dir>/<YYYYMMDD>-<adjective>-<noun>-<number>` (e.g. `~/oneoff/20260424-witty-penguin-42`).
- A `SessionTypeDirectory` session is started with `Path` set to that generated directory.
- The session title is whatever the user typed (not the directory name).
- The generated directory path is visible in the session detail view.

### US-2: Configure the one-off base directory
**As a** user,  
**I want to** set a base directory for one-off sessions in the app config,  
**So that** all one-off directories are created in a predictable location I control.

**Acceptance Criteria:**
- A new `one_off_base_dir` field in `config.Config` (JSON key: `"one_off_base_dir"`).
- Default value: `~/oneoff` (expanded at runtime).
- The base directory is created automatically if it doesn't exist when the first one-off session is created.
- If `one_off_base_dir` is empty or not set, use the default.

### US-3: Unique, sortable directory name generation
**As a** user,  
**I want** generated directory names to be date-prefixed and human-readable,  
**So that** I can sort them chronologically in Finder/terminal and still identify them by name.

**Acceptance Criteria:**
- Directory name format: `YYYYMMDD-<adjective>-<noun>-<number>` (e.g. `20260424-brave-falcon-07`).
- Date portion uses the creation date in local time.
- Adjective and noun are drawn from a fixed curated word list (≥ 50 adjectives, ≥ 50 nouns) embedded in the Go binary.
- Number is a zero-padded two-digit random number (00–99).
- If the generated path already exists (collision), regenerate until a unique name is found (max 10 attempts; fail with an error after that).

### US-4: Directory persists after session ends
**As a** user,  
**I want** the generated directory to remain on disk after the session is paused, stopped, or destroyed,  
**So that** I don't lose any work the AI produced.

**Acceptance Criteria:**
- Destroying a one-off session does NOT delete the generated directory (same behavior as `SessionTypeDirectory`).
- No special cleanup hook is registered for one-off sessions.

---

## Out of Scope

- CLI (`stapler-squad create --one-off`) — web UI only for this iteration.
- Git initialization inside the one-off directory — just a plain directory.
- Auto-cleanup / TTL expiry of old one-off directories.
- Configuring name format — format is fixed at `YYYYMMDD-adjective-noun-number`.

---

## Non-Functional Requirements

- Word list embedded in binary (no external file dependency).
- Name generation is O(1) per attempt (no network, no disk reads beyond mkdir check).
- Generated directory name is URL-safe and shell-safe (lowercase letters, digits, hyphens only).
- Backend validation: reject creation if `one_off_base_dir` resolves to a non-existent and non-creatable path.

---

## Constraints

- Uses existing `SessionTypeDirectory` — no new session type needed.
- `InstanceOptions.Path` is set to the generated directory path; `InstanceOptions.Title` is the user-supplied title.
- Must wire through the existing `CreateSession` RPC path — no new RPC endpoint.
- Word lists must produce names ≤ 32 characters total (to stay within session title display limits — though the *directory* name has no such limit).

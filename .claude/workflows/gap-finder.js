export const meta = {
  name: 'gap-finder',
  description: 'Find unwired/incomplete features in stapler-squad until no new gaps emerge',
  whenToUse: 'Run periodically or after large feature additions to surface half-baked wiring.',
  phases: [
    { title: 'Scan', detail: 'Parallel agents search each subsystem for new gaps' },
    { title: 'Deduplicate', detail: 'Filter findings against already-known gaps' },
    { title: 'Report', detail: 'Append confirmed new gaps to docs/gap-analysis.md' },
  ],
}

// ─── Config ──────────────────────────────────────────────────────────────────

// Use relative paths — agents inherit the session's working directory (repo root).
// Pass REPO_ROOT via args if running from a non-root directory: Workflow({args: {repo: '/abs/path'}})
const REPO = (args && args.repo) ? args.repo : '.'
const GAP_FILE = `${REPO}/docs/gap-analysis.md`
const INDEX_FILE = `${REPO}/docs/gap-analysis-index.json`
// Stop after DRY_THRESHOLD consecutive rounds with no new gaps. Two rounds guards
// against a single agent mis-firing while still terminating promptly when converged.
const DRY_THRESHOLD = 2
// Hard ceiling — prevents infinite loops if agents consistently paraphrase known gaps
// below the 60% word-overlap dedup threshold.
const MAX_ROUNDS = 12

// ─── Schema for scanner agents ───────────────────────────────────────────────

const GAP_SCHEMA = {
  type: 'object',
  properties: {
    findings: {
      type: 'array',
      items: {
        type: 'object',
        required: ['title', 'severity', 'file', 'detail', 'fixHint'],
        properties: {
          title:    { type: 'string', description: 'One-line problem description, unique enough to identify the gap' },
          severity: { type: 'string', enum: ['high', 'medium', 'low'] },
          file:     { type: 'string', description: 'Most relevant file path, relative to repo root' },
          detail:   { type: 'string', description: '2-4 sentences: what is wired vs what is missing' },
          fixHint:  { type: 'string', description: 'Concrete first step to fix it' },
        },
      },
    },
  },
  required: ['findings'],
}

// ─── Subsystem scanner agents ─────────────────────────────────────────────────

function makeAgentPrompt(subsystem, description, knownTitles) {
  const KNOWN_TITLE_LIMIT = 300
  const known = knownTitles.length
    ? knownTitles.slice(0, KNOWN_TITLE_LIMIT).map((t, i) => `  ${i + 1}. ${t}`).join('\n')
    : '  (none yet)'

  return `
You are a code gap-finder agent for the stapler-squad project at ${REPO}.

Your scope: **${subsystem}** — ${description}

Already-known gap titles (skip these — do not re-report problems described by any of these titles):
${known}

Your job: find NEW implementation gaps — things that are:
- Defined in proto but never registered in server.go
- Registered in server.go but return stub / unimplemented errors
- Implemented in Go but never called from the frontend (no hook, no component)
- Frontend components that reference RPCs that don't exist yet
- Proto message fields that are declared but never populated by any handler
- Filter/sort parameters accepted by a request but silently ignored
- UI surfaces gated behind hidden flags or debug-only URL params
- Duplicate implementations where only one is live (dead code)
- Config fields that exist in Go structs but have no proto field, no RPC, and no UI

Do NOT report:
- Anything whose topic is already covered by a title in the known list above
- HeadlessService (intentionally CLI-only, no frontend needed)
- Planned/TODO items (only report things broken/unwired RIGHT NOW in the code)
- Test-only code or dead code that was already removed

For each NEW gap, produce a finding with:
- title: concise unique problem description (e.g. "Session.project_id never set in InstanceToProto")
- severity: high / medium / low
- file: most relevant file path
- detail: 2-4 sentences explaining what's wired vs what's missing
- fixHint: concrete first step to fix it

Return ONLY findings you verified by reading the actual code. Empty array if nothing new.
`.trim()
}

const SUBSYSTEMS = [
  {
    key: 'proto-backend',
    label: 'Proto → Go handler registration',
    description: `
      Check proto/session/v1/*.proto against server/server.go and server/services/*.go.
      Look for: unregistered services, handlers returning connect.CodeUnimplemented,
      proto message fields set in the request but never read by the handler,
      proto response fields never populated by the handler.
      Key files: server/server.go, server/services/session_service.go,
      server/services/project_service.go, server/services/backlog_service.go,
      server/adapters/instance_adapter.go, proto/session/v1/session.proto,
      proto/session/v1/unfinished.proto, proto/session/v1/insights.proto,
      proto/session/v1/backlog.proto.
    `.trim(),
  },
  {
    key: 'frontend-rpc-usage',
    label: 'Frontend → RPC call coverage',
    description: `
      Check web-app/src/ for RPCs defined in the generated clients
      (web-app/src/gen/session/v1/) that are never called.
      Also check for hooks that wrap RPCs but are never imported by any component.
      Key dirs: web-app/src/lib/hooks/, web-app/src/lib/contexts/,
      web-app/src/components/ (all subdirs), web-app/src/app/.
    `.trim(),
  },
  {
    key: 'frontend-nav',
    label: 'Frontend nav + routing completeness',
    description: `
      Check web-app/src/app/ for pages with no nav entry (unreachable),
      nav entries pointing to non-existent pages, features gated behind feature flags
      with no UI toggle, pages only reachable via hidden URL params like ?debug=1,
      route constants defined in routes.ts but never imported, and nav components
      (Header, BottomNav, DrawerNav) that skip feature-flag filtering for some items.
      Key files: web-app/src/lib/routes.ts, web-app/src/lib/nav-pages.ts,
      web-app/src/components/layout/, web-app/src/app/settings/.
    `.trim(),
  },
  {
    key: 'db-schema',
    label: 'DB schema → service → proto completeness',
    description: `
      Check session/ent/schema/ for entity schemas. For each entity verify:
      storage CRUD exists, service layer calls storage, and proto layer surfaces the data.
      Look for entities with schema + generated code but no service methods,
      entities whose DB-stored fields are not mapped in proto response converters,
      and fields stored in Go Instance but omitted in InstanceToProto.
      Key files: session/ent/schema/, session/ent_repository.go,
      server/adapters/instance_adapter.go, server/services/*.go.
    `.trim(),
  },
  {
    key: 'config-wiring',
    label: 'Config fields → proto → UI wiring',
    description: `
      Check config/config.go for fields that: have no proto equivalent, or exist in proto
      but no UI control, or are read by Go code but ignored/bypassed in the service layer.
      Also check session defaults: are all ResolveDefaultsResponse fields shown in the UI?
      Are all GlobalDefaults form fields saved and reloaded correctly?
      Key files: config/config.go, server/services/defaults_service.go,
      web-app/src/components/settings/GlobalDefaultsForm.tsx,
      web-app/src/components/sessions/OmnibarCreationPanel.tsx.
    `.trim(),
  },
  {
    key: 'workflow-session-creation',
    label: 'Workflow scheduler → session creation wiring',
    description: `
      Check the Workflow feature: server/workflows/scheduler.go FireNow() function,
      workflow fields in WorkflowProto vs what FireNow() actually uses,
      WorkflowForm.tsx fields vs CreateWorkflowRequest proto fields,
      and whether cron sessions run correctly unattended (permission mode, one_shot,
      allowed_tools, session_type mapping).
      Key files: server/workflows/scheduler.go, server/services/workflow_service.go,
      web-app/src/components/workflows/WorkflowForm.tsx,
      web-app/src/lib/hooks/useWorkflows.ts.
    `.trim(),
  },
  {
    key: 'session-lifecycle',
    label: 'Session lifecycle + state machine completeness',
    description: `
      Check session lifecycle for wiring gaps:
      - ForkSession goroutine: does it wire all callbacks (rate limit, status change, etc.)?
      - HibernateSession/ResumeHibernatedSession: does resume re-add to pollers and rewire callbacks?
      - Autonomous mode: is it persisted across restarts?
      - One-shot sessions: do they clean up correctly?
      - UpdateSession: are all request fields handled, or are some silently dropped?
      Key files: server/services/session_service.go, session/instance_hibernate.go,
      server/services/session_service_shells.go, session/ent/schema/session.go.
    `.trim(),
  },
]

// ─── Index helpers ─────────────────────────────────────────────────────────────

// Normalize a title for comparison: lowercase, collapse whitespace, strip punctuation
function normalizeTitle(t) {
  return t.toLowerCase().replace(/[^a-z0-9\s]/g, ' ').replace(/\s+/g, ' ').trim()
}

// Check if a new title is "close enough" to any known title
// Uses word-overlap heuristic: if 60%+ of content words overlap, treat as duplicate
function isDuplicate(newTitle, knownTitlesNorm) {
  const nw = new Set(normalizeTitle(newTitle).split(' ').filter(w => w.length > 3))
  if (nw.size === 0) return false
  for (const kt of knownTitlesNorm) {
    const kw = new Set(kt.split(' ').filter(w => w.length > 3))
    if (kw.size === 0) continue
    let overlap = 0
    for (const w of nw) if (kw.has(w)) overlap++
    const ratio = overlap / Math.min(nw.size, kw.size)
    if (ratio >= 0.60) return true
  }
  return false
}

// Read the index file (returns {titles: string[]} or empty)
async function readIndex() {
  const raw = await agent(
    `Read the file ${INDEX_FILE} and return its exact text content. Return ONLY the raw file text with no commentary. If the file does not exist, return the string "NOT_FOUND".`,
    { label: 'read-index', effort: 'low' }
  )
  if (!raw || raw.trim() === 'NOT_FOUND' || raw.trim() === '') return { titles: [] }
  try {
    const start = raw.indexOf('{')
    const end = raw.lastIndexOf('}')
    if (start === -1 || end === -1) return { titles: [] }
    return JSON.parse(raw.slice(start, end + 1))
  } catch (e) {
    log(`WARNING: gap-analysis-index.json is corrupt (${e.message}). Treating as empty — all previously found gaps may be re-reported.`)
    return { titles: [] }
  }
}

// Write updated index back to disk
async function writeIndex(titles, round) {
  const content = JSON.stringify({ titles }, null, 2)
  await agent(
    `Write the following JSON content to the file ${INDEX_FILE}. Overwrite it completely. Content:\n\n${content}`,
    { label: `write-index-r${round}`, effort: 'low' }
  )
}

function nextGapId(content) {
  // Anchor to line start to avoid matching agent preamble or inline code
  const ids = [...content.matchAll(/^###\s+GAP-(\d+)\b/gm)].map(m => parseInt(m[1], 10))
  const max = ids.length ? Math.max(...ids) : 0
  return max + 1
}

// ─── Main loop ────────────────────────────────────────────────────────────────

phase('Scan')

let dry = 0
let totalNewGaps = 0
let round = 0

while (dry < DRY_THRESHOLD && round < MAX_ROUNDS) {
  round++
  log(`Round ${round} — reading known gap index…`)

  const index = await readIndex()
  const knownTitles = index.titles || []
  const knownNorm = knownTitles.map(normalizeTitle)

  log(`Round ${round} — ${knownTitles.length} known gaps. Scanning ${SUBSYSTEMS.length} subsystems in parallel…`)

  phase('Scan')

  const results = await parallel(
    SUBSYSTEMS.map(sub => () => agent(
      makeAgentPrompt(sub.label, sub.description, knownTitles),
      { label: `scan:${sub.key}:r${round}`, phase: 'Scan', schema: GAP_SCHEMA, effort: 'medium' }
    ))
  )

  phase('Deduplicate')

  // Collect findings and deduplicate by title similarity
  const seenThisRound = new Set()
  const freshGaps = []

  for (const result of results.filter(Boolean)) {
    for (const g of (result.findings ?? [])) {
      if (!g || !g.title) continue
      const norm = normalizeTitle(g.title)
      // Skip if too similar to anything already known or already seen this round
      if (isDuplicate(g.title, knownNorm)) continue
      if (seenThisRound.has(norm)) continue
      seenThisRound.add(norm)
      freshGaps.push(g)
    }
  }

  log(`Round ${round} — ${freshGaps.length} new gap(s) after deduplication`)

  if (freshGaps.length === 0) {
    dry++
    log(`Round ${round} — dry (${dry}/${DRY_THRESHOLD})`)
    continue
  }

  dry = 0
  totalNewGaps += freshGaps.length

  phase('Report')

  // Read current gap file to get the next ID
  const gapFileContent = await agent(
    `Read the file ${GAP_FILE} and return its full text content. Return ONLY the raw text.`,
    { label: `read-gap-file-r${round}`, effort: 'low' }
  )

  let nextId = nextGapId(gapFileContent || '')

  // Format new entries
  const newEntries = freshGaps.map(g => {
    const id = `GAP-${String(nextId++).padStart(3, '0')}`
    const fp = `GAP-${g.file.replace(/[^a-zA-Z0-9]/g, '-').replace(/-+/g, '-').slice(0, 40)}-${g.title.split(' ').slice(0, 3).join('-').replace(/[^a-zA-Z0-9-]/g, '')}`
    return `
### ${id} — ${g.title}
- **status**: open
- **severity**: ${g.severity}
- **fingerprint**: ${fp}
- **file**: \`${g.file}\`
- **detail**: ${g.detail}
- **fix hint**: ${g.fixHint}`.trim()
  }).join('\n\n')

  // Append to gap file
  await agent(
    `Append the following text to the END of ${GAP_FILE}. Do not modify any existing content — only add to the end. Text to append:\n\n${newEntries}`,
    { label: `append-gaps-r${round}`, phase: 'Report', effort: 'low' }
  )

  // Update index with new titles
  const updatedTitles = [...knownTitles, ...freshGaps.map(g => g.title)]
  await writeIndex(updatedTitles, round)

  log(`Round ${round} — appended ${freshGaps.length} new gap(s). Index now has ${updatedTitles.length} entries.`)
}

if (round >= MAX_ROUNDS) {
  log(`WARNING: hit MAX_ROUNDS (${MAX_ROUNDS}) without converging — agents may be paraphrasing known gaps. Check dedup threshold.`)
}
log(`Loop complete after ${round} rounds. Total new gaps found: ${totalNewGaps}.`)

return {
  rounds: round,
  totalNewGaps,
  message: totalNewGaps === 0
    ? 'No new gaps found — gap analysis is up to date.'
    : `Found and documented ${totalNewGaps} new gap(s) across ${round} round(s). See docs/gap-analysis.md.`,
}

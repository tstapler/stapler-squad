/**
 * Seeds the pending entities behind inline notification actions through seams the server already
 * exposes: the real PermissionRequest hook (a pending approval), CreateGuidanceRequest, the e2e-local
 * backlog debug endpoints and SendNotification for the history row that carries the metadata keys the
 * descriptor builder reads (server/services/notification_actions.go).
 */
import { expect, type APIRequestContext } from '@playwright/test';
import { sendNotification } from '../pages/NotificationPanel';
import { createBacklogItemDirect } from '../pages/BacklogMutations';

/** POSTs one Connect JSON call. */
function rpc(request: APIRequestContext, baseUrl: string, method: string, data: object, service = 'SessionService') {
  return request.post(`${baseUrl}/api/session.v1.${service}/${method}`, { headers: { 'Content-Type': 'application/json' }, data });
}

/** The shape protojson gives a notification history record (only what these specs read). */
interface HistoryRecord {
  id: string;
  sessionId?: string;
  isRead?: boolean;
  metadata?: Record<string, string>;
}

async function history(request: APIRequestContext, baseUrl: string): Promise<HistoryRecord[]> {
  const response = await rpc(request, baseUrl, 'GetNotificationHistory', { limit: 500 });
  const body = (await response.json()) as { notifications?: HistoryRecord[] };
  return body.notifications ?? [];
}

/** The history subscriber coalesces before persisting, so a row sent just now is not yet listed. */
async function untilPersisted(request: APIRequestContext, baseUrl: string, key: string, value: string): Promise<void> {
  await expect
    .poll(async () => (await history(request, baseUrl)).some((n) => n.metadata?.[key] === value), {
      message: `history row with ${key}=${value}`,
      timeout: 15_000,
    })
    .toBe(true);
}

export interface SeededApproval {
  approvalId: string;
  command: string;
  /** Settles with the hook's answer when the approval is resolved; never rejects. */
  decision: Promise<{ behavior?: string; message?: string } | null>;
}

/**
 * Raises a real pending approval: POSTs the PermissionRequest hook with a command no classifier rule
 * matches (the handler then blocks until a decision) and waits for its history row.
 */
export async function seedApproval(
  request: APIRequestContext,
  baseUrl: string,
  sessionId: string,
  command: string,
): Promise<SeededApproval> {
  const decision = request
    .post(`${baseUrl}/api/hooks/permission-request`, {
      headers: { 'X-CS-Session-ID': sessionId },
      timeout: 0,
      data: {
        session_id: `claude-session-${command}`,
        cwd: '/tmp',
        permission_mode: 'default',
        hook_event_name: 'PermissionRequest',
        tool_name: 'Bash',
        tool_input: { command },
      },
    })
    .then(async (r) => ((await r.json()) as { hookSpecificOutput?: { decision?: { behavior?: string; message?: string } } }).hookSpecificOutput?.decision ?? null)
    .catch(() => null);

  let approvalId = '';
  await expect
    .poll(
      async () => {
        const hit = (await history(request, baseUrl)).find((n) => n.metadata?.tool_input_command === command);
        approvalId = hit?.metadata?.approval_id ?? '';
        return approvalId;
      },
      { message: `approval history row for ${command}`, timeout: 15_000 },
    )
    .not.toBe('');
  return { approvalId, command, decision };
}

export interface SeededGuidance {
  id: string;
  question: string;
}

/** A standalone guidance request with `options` as one-tap answers, plus the history row that offers it. */
export async function seedGuidance(
  request: APIRequestContext,
  baseUrl: string,
  question: string,
  options: string[],
): Promise<SeededGuidance> {
  const created = await rpc(
    request,
    baseUrl,
    'CreateGuidanceRequest',
    { scope: 'standalone', questionText: question, questionType: 'multiple-choice', options },
    'GuidanceRequestService',
  );
  if (!created.ok()) throw new Error(`CreateGuidanceRequest failed: ${created.status()} ${await created.text()}`);
  const { request: row } = (await created.json()) as { request: { id: string } };
  await sendNotification(request, baseUrl, {
    sessionId: `e2e-guidance-${question}`,
    type: 'INFO',
    title: question,
    metadata: { guidance_request_id: row.id },
  });
  await untilPersisted(request, baseUrl, 'guidance_request_id', row.id);
  return { id: row.id, question };
}

export async function answerGuidanceViaRpc(request: APIRequestContext, baseUrl: string, id: string, answer: string): Promise<void> {
  const response = await rpc(request, baseUrl, 'AnswerGuidanceRequest', { id, answer }, 'GuidanceRequestService');
  if (!response.ok()) throw new Error(`AnswerGuidanceRequest failed: ${response.status()} ${await response.text()}`);
}

export interface SeededDecision {
  itemId: string;
  title: string;
}

const DECISION_SCHEMA = JSON.stringify({
  choices: [
    { value: 'accept', label: 'Mark done', rpc: 'TransitionBacklogItemStatus' },
    { value: 'rework', label: 'Send back', rpc: 'TransitionBacklogItemStatus' },
    { value: 'merge', label: 'Merge', rpc: 'MergePR' },
  ],
});

/** A backlog item in `review` and the notification that asks the operator to decide it. */
export async function seedBacklogDecision(request: APIRequestContext, baseUrl: string, title: string): Promise<SeededDecision> {
  const itemId = await createBacklogItemDirect(request, { title, status: 'review', acCriteria: ['Decision is recorded'] });
  await sendNotification(request, baseUrl, {
    sessionId: `e2e-decision-${title}`,
    type: 'INFO',
    title,
    metadata: { item_id: itemId, decision_schema: DECISION_SCHEMA },
  });
  await untilPersisted(request, baseUrl, 'item_id', itemId);
  return { itemId, title };
}

export async function historyHasUnread(request: APIRequestContext, baseUrl: string, predicate: (m: Record<string, string>) => boolean): Promise<boolean> {
  return (await history(request, baseUrl)).some((n) => !n.isRead && predicate(n.metadata ?? {}));
}

export async function backlogItemStatus(request: APIRequestContext, baseUrl: string, itemId: string): Promise<string> {
  const response = await rpc(request, baseUrl, 'GetBacklogItem', { itemId }, 'BacklogService');
  const body = (await response.json()) as { item?: { status?: string } };
  return body.item?.status ?? '';
}

/** Settles an approval the way Settings or Slack would, so a test never leaves a hook request pending. */
export async function resolveApprovalViaRpc(request: APIRequestContext, baseUrl: string, approvalId: string, decision: 'allow' | 'deny'): Promise<void> {
  const response = await rpc(request, baseUrl, 'ResolveApproval', { approvalId, decision });
  if (!response.ok()) throw new Error(`ResolveApproval failed: ${response.status()} ${await response.text()}`);
}

export interface SeedQuestion {
  text: string;
  header?: string;
  multiSelect?: boolean;
  options: string[];
}

export interface SeededQuestionCard {
  rowId: string;
  question: string;
}

/**
 * Raises a durable question for a live session through the e2e-local seed endpoint and the history
 * row that carries it. `hook` rows are registered as the on-screen dialog (a keystroke answer; the
 * single-select digit, or the measured single-then-multi pair typed as a key sequence);
 * `agent_mcp` rows are stored answers (multi-select, several questions, free text).
 */
export async function seedQuestionCard(
  request: APIRequestContext,
  baseUrl: string,
  opts: { sessionId: string; source: 'ask_user_question_hook' | 'agent_mcp'; questions: SeedQuestion[] },
): Promise<SeededQuestionCard> {
  const hookCallId = opts.source === 'ask_user_question_hook' ? `toolu_${Date.now()}_${Math.floor(Math.random() * 1e6)}` : '';
  let seeded = await request.post(`${baseUrl}/api/debug/guidance/seed-question`, { headers: { 'Content-Type': 'application/json' }, data: { sessionId: opts.sessionId, source: opts.source, hookCallId, questions: opts.questions } });
  // A session just created is not live until its pane has started.
  for (let attempt = 0; attempt < 30 && seeded.status() === 404; attempt++) {
    await new Promise((resolve) => setTimeout(resolve, 500));
    seeded = await request.post(`${baseUrl}/api/debug/guidance/seed-question`, { headers: { 'Content-Type': 'application/json' }, data: { sessionId: opts.sessionId, source: opts.source, hookCallId, questions: opts.questions } });
  }
  if (!seeded.ok()) throw new Error(`seed-question failed: ${seeded.status()} ${await seeded.text()}`);
  const { id, sessionUuid } = (await seeded.json()) as { id: string; sessionUuid: string };
  const first = opts.questions[0];
  const hook = opts.source === 'ask_user_question_hook';
  const digit = hook && opts.questions.length === 1 && !first.multiSelect; // one digit answers it
  await sendNotification(request, baseUrl, {
    sessionId: sessionUuid,
    type: 'INPUT_REQUIRED',
    title: 'Claude has a question',
    message: first.text,
    metadata: {
      guidance_request_id: id,
      question_shape: digit ? 'single' : 'multi',
      // A hook's measured tabbed dialog is addressed by its row id too, but has no single option list.
      ...(hook ? { question_id: id } : {}),
      ...(digit ? { question_options: JSON.stringify(first.options) } : {}),
    },
  });
  await untilPersisted(request, baseUrl, 'guidance_request_id', id);
  return { rowId: id, question: first.text };
}

/** Reads a durable question back, the way the card does. */
export async function readGuidanceRow(request: APIRequestContext, baseUrl: string, id: string) {
  const response = await rpc(request, baseUrl, 'GetGuidanceRequest', { id }, 'GuidanceRequestService');
  const body = (await response.json()) as { request: { status: string; answer?: string; deliveryState?: string; cancelReason?: string } };
  return body.request;
}

/** What the session's PostToolUse hook sends when the operator answered the dialog in the terminal. */
export async function postToolUseInTerminal(request: APIRequestContext, baseUrl: string, sessionTitle: string): Promise<void> {
  const response = await request.post(`${baseUrl}/api/hooks/post-tool-use`, {
    headers: { 'X-CS-Session-ID': sessionTitle, 'X-CS-Tool-Name': 'AskUserQuestion' },
    data: {},
  });
  if (!response.ok()) throw new Error(`post-tool-use failed: ${response.status()}`);
}

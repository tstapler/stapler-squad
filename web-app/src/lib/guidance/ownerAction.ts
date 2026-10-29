import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";

/** Mirrors domain.OwnerActionQuestionPrefix: request_owner_action stamps it on the question text. */
export const OWNER_ACTION_PREFIX = "[Owner action] ";

export const isOwnerActionRequest = (r: Pick<PlainGuidanceRequest, "questionText">): boolean =>
  r.questionText.startsWith(OWNER_ACTION_PREFIX);

export const ownerActionQuestion = (r: Pick<PlainGuidanceRequest, "questionText">): string =>
  isOwnerActionRequest(r) ? r.questionText.slice(OWNER_ACTION_PREFIX.length) : r.questionText;

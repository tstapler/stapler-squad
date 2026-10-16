/**
 * Composes a Q:/A: feedback string for a single answered triage question,
 * preserving the question↔answer link without requiring a stable question
 * ID (none exists — see architecture.md §2.1). Handed as-is to the
 * existing triggerTriage(id, feedback) call.
 */
export function composeQuestionAnswerFeedback(questionText: string, answerText: string): string {
  return `Q: ${questionText.trim()}\nA: ${answerText.trim()}`;
}

// +feature: workflow-detector
/**
 * WorkflowDetector detects @slug [arg] syntax for quick workflow invocation.
 * Priority 25 — after GitHub URL detectors (10/20/30) and before NewSessionDetector (35).
 */

import { Detector } from "../detector";
import { DetectionResult, InputType } from "../types";

/**
 * WorkflowEntry is the lean interface WorkflowDetector needs for detection.
 * Only fields used by detection logic are included to keep the interface minimal.
 */
export interface WorkflowEntry {
  slug: string;
  name: string;
  description?: string;
  targetDirectory?: string;
  sessionType?: string;
  inputTemplate?: string;
}

/**
 * WorkflowDetector matches "@slug [arg]" input patterns.
 *
 * Registration: NOT in createDefaultRegistry(). Dynamically registered/unregistered
 * by OmnibarContext when workflows load, so the list is always current.
 *
 * Priority 25 is between GitHubRepoDetector (30) and NewSessionDetector (35).
 * "@" is unclaimed by any other detector.
 */
export class WorkflowDetector implements Detector {
  name = "WorkflowDetector";
  priority = 25;

  private readonly AT_PATTERN = /^@([a-zA-Z0-9_-]+)(?:\s+(.+))?$/;
  private workflows: WorkflowEntry[];

  constructor(workflows: WorkflowEntry[]) {
    this.workflows = workflows;
  }

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();
    const match = trimmed.match(this.AT_PATTERN);
    if (!match) return null;

    const rawSlug = match[1];
    const arg = match[2]?.trim() ?? "";
    const slug = rawSlug.toLowerCase();

    // Look up workflow by slug (case-insensitive).
    const wf = this.workflows.find((w) => w.slug.toLowerCase() === slug);

    if (!wf) {
      // Unknown slug — return null so AliasDetector (priority 36) can claim @unknown input.
      // WorkflowDetector only claims slugs that match known workflows.
      return null;
    }

    // Known slug — interpolate prompt template if available.
    let interpolatedPrompt = wf.inputTemplate ?? "";
    if (interpolatedPrompt && arg) {
      interpolatedPrompt = interpolatedPrompt.replace(/\{\{input\}\}/g, arg);
    } else if (!interpolatedPrompt) {
      interpolatedPrompt = arg;
    }

    return {
      type: InputType.Workflow,
      confidence: 1.0,
      parsedValue: trimmed,
      suggestedName: wf.name,
      metadata: {
        workflowFound: true,
        slug: wf.slug,
        workflowArg: arg,
        interpolatedPrompt,
        workflow: wf,
      },
    };
  }
}

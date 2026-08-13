#!/usr/bin/env ts-node
// Story 5: UX Analysis Automation
// Uses Claude API vision to analyze screenshots and produce UX findings.
//
// Usage: ts-node tools/ux-analysis/analyze.ts <screenshot1.png> [screenshot2.png] [screenshot3.png]
//   Options:
//     --pr <number>       PR number for output filename
//     --feature <id>      Feature ID being analyzed
//     --output <dir>      Output directory (default: docs/qa/)

import * as fs from 'fs';
import * as path from 'path';

const MAX_SCREENSHOTS = 3;
const MAX_COST_USD = 1.0;
// Approximate cost per screenshot with claude-sonnet-4-6 vision
// A typical screenshot is ~200KB, base64 encoded ~267KB, ~67K tokens worst case
// Actual cost is much lower. We use a conservative estimate.
const COST_PER_SCREENSHOT_USD = 0.02;

interface UXFinding {
  severity: 'critical' | 'serious' | 'moderate' | 'minor';
  category: string;
  finding: string;
  recommendation: string;
  confidence: number;
}

interface UXAnalysisResult {
  featureId: string;
  prNumber: number | null;
  screenshotsAnalyzed: number;
  findings: UXFinding[];
  generatedAt: string;
}

function parseArgs(): {
  screenshots: string[];
  prNumber: number | null;
  featureId: string;
  outputDir: string;
} {
  const args = process.argv.slice(2);
  const screenshots: string[] = [];
  let prNumber: number | null = null;
  let featureId = 'unknown';
  let outputDir = 'docs/qa';

  for (let i = 0; i < args.length; i++) {
    if (args[i] === '--pr' && args[i + 1]) {
      prNumber = parseInt(args[++i], 10);
    } else if (args[i] === '--feature' && args[i + 1]) {
      featureId = args[++i];
    } else if (args[i] === '--output' && args[i + 1]) {
      outputDir = args[++i];
    } else if (!args[i].startsWith('--')) {
      screenshots.push(args[i]);
    }
  }

  return { screenshots, prNumber, featureId, outputDir };
}

async function analyzeScreenshots(
  screenshots: string[],
  featureId: string,
  designSystemContext: string,
): Promise<UXFinding[]> {
  const apiKey = process.env.ANTHROPIC_API_KEY;
  if (!apiKey) {
    console.log('ANTHROPIC_API_KEY not set, skipping UX analysis');
    return [];
  }

  // Cap at MAX_SCREENSHOTS
  const screenshotsToAnalyze = screenshots.slice(0, MAX_SCREENSHOTS);
  if (screenshots.length > MAX_SCREENSHOTS) {
    console.log(`Capped screenshots at ${MAX_SCREENSHOTS} (${screenshots.length - MAX_SCREENSHOTS} skipped)`);
  }

  // Cost guard
  const estimatedCost = screenshotsToAnalyze.length * COST_PER_SCREENSHOT_USD;
  if (estimatedCost > MAX_COST_USD) {
    console.log(`Estimated cost $${estimatedCost.toFixed(2)} exceeds cap $${MAX_COST_USD}, skipping`);
    return [];
  }

  // Build image content blocks
  const imageBlocks: object[] = [];
  for (const screenshotPath of screenshotsToAnalyze) {
    if (!fs.existsSync(screenshotPath)) {
      console.warn(`Screenshot not found: ${screenshotPath}`);
      continue;
    }
    const imageData = fs.readFileSync(screenshotPath);
    const base64 = imageData.toString('base64');
    const mediaType = screenshotPath.endsWith('.png') ? 'image/png' : 'image/jpeg';

    imageBlocks.push({
      type: 'image',
      source: {
        type: 'base64',
        media_type: mediaType,
        data: base64,
      },
    });
  }

  if (imageBlocks.length === 0) {
    console.log('No valid screenshots to analyze');
    return [];
  }

  const prompt = `You are reviewing screenshots of Stapler Squad, an internal developer tool that manages AI coding agent sessions in tmux.

IMPORTANT CONTEXT:
- This application renders terminal output in monospace \`<pre>\` elements. Terminal color contrast is intentional and should NOT be flagged as an accessibility violation.
- This is an internal developer tool, not a consumer product. Design choices prioritize developer productivity.
- Feature being reviewed: ${featureId}

DESIGN SYSTEM TOKENS (from globals.css):
${designSystemContext}

Please analyze the provided screenshot(s) and identify the TOP 3 UX/accessibility findings.

For each finding, provide:
1. severity: critical | serious | moderate | minor
2. category: accessibility | usability | visual | interaction | content
3. finding: specific observation (1-2 sentences)
4. recommendation: actionable fix (1-2 sentences)
5. confidence: 0-100 (how confident you are this is a real issue)

Exclude:
- Terminal rendering areas (pre elements, ANSI colored output)
- Intentional monospace/terminal aesthetics
- Missing PWA features (not applicable)

Return ONLY a JSON array with exactly 3 objects in the format above.`;

  try {
    const response = await fetch('https://api.anthropic.com/v1/messages', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'x-api-key': apiKey,
        'anthropic-version': '2023-06-01',
      },
      body: JSON.stringify({
        model: 'claude-sonnet-4-6',
        max_tokens: 1024,
        messages: [
          {
            role: 'user',
            content: [
              ...imageBlocks,
              { type: 'text', text: prompt },
            ],
          },
        ],
      }),
    });

    if (!response.ok) {
      const errorText = await response.text();
      console.error(`API error ${response.status}: ${errorText}`);
      return [];
    }

    const data = (await response.json()) as { content: Array<{ type: string; text: string }> };
    const textContent = data.content.find(c => c.type === 'text')?.text || '[]';

    // Extract JSON from response (may be wrapped in markdown code blocks)
    const jsonMatch = textContent.match(/\[[\s\S]*\]/);
    if (!jsonMatch) {
      console.warn('Could not extract JSON from Claude response');
      return [];
    }

    return JSON.parse(jsonMatch[0]) as UXFinding[];
  } catch (err) {
    console.error(`Error calling Claude API: ${err}`);
    return [];
  }
}

function loadDesignSystemContext(): string {
  const globalsPath = path.join(process.cwd(), 'web-app/src/app/globals.css');
  if (!fs.existsSync(globalsPath)) return '';

  const content = fs.readFileSync(globalsPath, 'utf-8');
  // Extract CSS custom properties (first 80 lines for brevity)
  const lines = content.split('\n').slice(0, 80).join('\n');
  return lines;
}

function formatMarkdown(result: UXAnalysisResult): string {
  const lines: string[] = [
    `# UX Analysis: ${result.featureId}`,
    '',
    `**Generated**: ${result.generatedAt}`,
    result.prNumber ? `**PR**: #${result.prNumber}` : '',
    `**Screenshots analyzed**: ${result.screenshotsAnalyzed}`,
    '',
    '## Findings',
    '',
  ];

  if (result.findings.length === 0) {
    lines.push('_No findings (analysis skipped or no issues detected)_');
  } else {
    for (let i = 0; i < result.findings.length; i++) {
      const f = result.findings[i];
      lines.push(
        `### Finding ${i + 1}: [${f.severity.toUpperCase()}] ${f.category}`,
        '',
        `**Observation**: ${f.finding}`,
        '',
        `**Recommendation**: ${f.recommendation}`,
        '',
        `**Confidence**: ${f.confidence}%`,
        '',
      );
    }
  }

  return lines.filter(l => l !== '').join('\n');
}

async function main(): Promise<void> {
  const { screenshots, prNumber, featureId, outputDir } = parseArgs();

  if (screenshots.length === 0) {
    console.log('Usage: analyze.ts <screenshot.png> [--pr <number>] [--feature <id>] [--output <dir>]');
    console.log('No screenshots provided - exiting with success');
    process.exit(0);
  }

  const designSystemContext = loadDesignSystemContext();
  const findings = await analyzeScreenshots(screenshots, featureId, designSystemContext);

  const result: UXAnalysisResult = {
    featureId,
    prNumber,
    screenshotsAnalyzed: Math.min(screenshots.length, MAX_SCREENSHOTS),
    findings,
    generatedAt: new Date().toISOString(),
  };

  // Write markdown output
  fs.mkdirSync(path.resolve(process.cwd(), outputDir), { recursive: true });
  const suffix = prNumber ? `-${prNumber}` : `-${Date.now()}`;
  const outputFile = path.join(
    path.resolve(process.cwd(), outputDir),
    `ux-findings${suffix}.md`,
  );
  fs.writeFileSync(outputFile, formatMarkdown(result));
  console.log(`UX findings written to: ${outputFile}`);

  // Also print summary to stdout for CI comment
  console.log('\n## UX Analysis Summary');
  if (findings.length === 0) {
    console.log('No findings (API key not set or no issues detected)');
  } else {
    console.log(`${findings.length} finding(s) for feature: ${featureId}`);
    findings.forEach((f, i) => {
      console.log(`${i + 1}. [${f.severity.toUpperCase()}] ${f.finding}`);
    });
  }
}

main().catch(err => {
  console.error(err);
  process.exit(1);
});

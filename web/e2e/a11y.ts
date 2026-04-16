import { type Page, expect } from '@playwright/test';
import AxeBuilder from '@axe-core/playwright';

/**
 * Accessibility helpers built on axe-core. A thin wrapper around AxeBuilder so
 * individual specs can run `await scanA11y(page, '/login')` without re-declaring
 * rule exclusions or severity thresholds.
 *
 * Why only "serious" / "critical" by default?
 *   Axe ships ~90 rules spanning four severity tiers. Gating CI on every "minor"
 *   or "moderate" violation would block merges for trivia (e.g. a link color
 *   marginally off-spec, a hint that a region *could* use a landmark). We keep
 *   those visible in the report but only fail the build on violations that
 *   would actually prevent someone from using the app with a screen reader
 *   or keyboard.
 */

export type A11ySeverity = 'minor' | 'moderate' | 'serious' | 'critical';

export const BLOCKING_IMPACT: A11ySeverity[] = ['serious', 'critical'];

/**
 * Rules we intentionally disable for our context:
 *  - `color-contrast`: our design uses Tailwind's zinc-on-zinc for muted text
 *    at roughly 4.3:1 (AA large-text), but axe evaluates it against the 4.5:1
 *    AA normal-text threshold. Revisit when we finalize the design tokens.
 *  - `region`: landing / login pages deliberately keep focus on a single
 *    form card without a surrounding `<main>`. Not a real usability issue.
 */
export const DEFAULT_DISABLED_RULES = ['color-contrast', 'region'] as const;

export interface ScanOptions {
  /** Extra axe rule IDs to disable for this call only. */
  disableRules?: string[];
  /** Override the default blocking severities (unlikely — tighten in review). */
  blockingImpact?: A11ySeverity[];
  /** Restrict the scan to a CSS selector (useful for dialog/sheet components). */
  include?: string;
}

/**
 * Run axe against the current page and fail the test if any violation at
 * `blockingImpact` severity is found. Returns the full axe result so callers
 * can surface non-blocking violations in logs or assertions of their own.
 */
export async function scanA11y(page: Page, options: ScanOptions = {}) {
  const disabled = [...DEFAULT_DISABLED_RULES, ...(options.disableRules ?? [])];
  const blocking = options.blockingImpact ?? BLOCKING_IMPACT;

  let builder = new AxeBuilder({ page }).disableRules(disabled);
  if (options.include) {
    builder = builder.include(options.include);
  }

  const results = await builder.analyze();

  const blockingViolations = results.violations.filter((v) =>
    v.impact && (blocking as string[]).includes(v.impact),
  );

  // Emit a compact summary so regressions show up in CI logs at a glance,
  // even on green runs — an increasing non-blocking count is an early warning.
  const summary = {
    url: results.url,
    violations: results.violations.length,
    blocking: blockingViolations.length,
    byImpact: tallyByImpact(results.violations),
  };
  console.log('[a11y]', JSON.stringify(summary));

  // Human-readable failure message when we block. Listing rule id + impact +
  // first node selector is usually enough to go find the offender in devtools.
  if (blockingViolations.length > 0) {
    const detail = blockingViolations
      .map((v) => {
        const first = v.nodes[0]?.target?.join(' > ') ?? '(no node)';
        return `  - [${v.impact}] ${v.id}: ${v.help} — ${first}`;
      })
      .join('\n');
    expect.soft(blockingViolations, `a11y blocking violations at ${results.url}:\n${detail}`).toHaveLength(0);
  }

  return results;
}

function tallyByImpact(violations: { impact?: string | null }[]) {
  const out: Record<string, number> = { minor: 0, moderate: 0, serious: 0, critical: 0 };
  for (const v of violations) {
    const key = v.impact ?? 'minor';
    out[key] = (out[key] ?? 0) + 1;
  }
  return out;
}

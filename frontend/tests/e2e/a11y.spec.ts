// Phase 5 §8.D — Accessibility scan with axe-core.
//
// Runs axe over the key public pages. The target says "zero serious violations"
// — we treat anything with severity `serious` or `critical` as a fail; minor
// and moderate are recorded but informational.

import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const PAGES = [
  { name: 'home',     path: '/' },
  { name: 'products', path: '/products' },
] as const

for (const p of PAGES) {
  test(`${p.name} — no serious or critical a11y violations`, async ({ page }, testInfo) => {
    await page.goto(p.path, { waitUntil: 'networkidle' })

    const results = await new AxeBuilder({ page })
      .withTags(['wcag2a', 'wcag2aa'])
      .analyze()

    // Attach the full report for triage.
    await testInfo.attach(`axe-${p.name}.json`, {
      body: JSON.stringify(results, null, 2),
      contentType: 'application/json',
    })

    const blocking = results.violations.filter(
      (v) => v.impact === 'serious' || v.impact === 'critical',
    )
    const summary = blocking.map((v) => `${v.impact}:${v.id}(${v.nodes.length})`).join(', ') || 'none'
    test.info().annotations.push({ type: 'axe-blocking', description: summary })

    expect.soft(
      blocking,
      `blocking a11y violations on ${p.name}: ${summary}`,
    ).toHaveLength(0)
  })
}

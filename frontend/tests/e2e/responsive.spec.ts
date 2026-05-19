// Phase 5 §8.D — Responsive (320px) — no horizontal overflow on key pages.
//
// Asserts: at viewport width 320px, document.documentElement.scrollWidth must
// not exceed the viewport width by more than 1px (rounding tolerance).

import { test, expect } from '@playwright/test'

const PAGES = [
  { name: 'home',     path: '/' },
  { name: 'products', path: '/products' },
  { name: 'cart',     path: '/cart' },
] as const

test.beforeEach(async ({ page }) => {
  await page.setViewportSize({ width: 320, height: 800 })
})

for (const p of PAGES) {
  test(`${p.name} — no horizontal overflow at 320px`, async ({ page }) => {
    await page.goto(p.path, { waitUntil: 'networkidle' })

    // Allow the page a moment to settle (web fonts, lazy components).
    await page.waitForTimeout(300)

    const overflow = await page.evaluate(() => {
      const doc = document.documentElement
      return {
        scrollWidth: doc.scrollWidth,
        clientWidth: doc.clientWidth,
        diff: doc.scrollWidth - doc.clientWidth,
      }
    })

    expect.soft(overflow.diff, `horizontal overflow on ${p.name}`).toBeLessThanOrEqual(1)
  })
}

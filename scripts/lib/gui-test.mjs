// Shared browser test plumbing; use the project's pinned Playwright by default.
import { createRequire } from 'node:module'
import { mkdir } from 'node:fs/promises'
import { join } from 'node:path'
import { setTimeout } from 'node:timers/promises'

const require = createRequire(new URL('../../web/package.json', import.meta.url))

export function loadPlaywright() {
  return require(process.env.PLAYWRIGHT_MODULE || 'playwright')
}

export async function until(check, description = 'GUI state', timeout = 15000) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    const value = await check()
    if (value) return value
    await setTimeout(100)
  }
  throw new Error(`Timed out: ${description}`)
}

export async function takeScreenshot(page, directory, name) {
  if (!page || !directory) return
  await mkdir(directory, { recursive: true })
  await page.screenshot({ path: join(directory, `${name}.png`), fullPage: true, animations: 'disabled' })
}

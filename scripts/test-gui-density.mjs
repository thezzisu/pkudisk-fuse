// Visual and geometric regressions: full names, aligned icons, compact rows,
// and independent touch targets in a real authenticated GUI.
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { loadPlaywright, until, takeScreenshot } from './lib/gui-test.mjs'

if (!process.argv[2]) throw new Error('Usage: node scripts/test-gui-density.mjs GUI-TEST-BINARY [SCREENSHOTS]')
const temporary = await mkdtemp(join(tmpdir(), 'pkudisk-density-'))
const capture = join(temporary, 'activation')
const screenshots = process.argv[3] && resolve(process.argv[3])
const baseline = process.env.PKUDISK_LAYOUT_BASELINE === '1'
const child = spawn(resolve(process.argv[2]), ['-test.run=^TestGUIBrowserFixtureProcess$', '-test.timeout=180s'], {
  env: { ...process.env, PKUDISK_GUI_BROWSER_CAPTURE: capture }, stdio: ['ignore', 'pipe', 'pipe']
})
let output = ''
child.stdout.on('data', value => { output += value })
child.stderr.on('data', value => { output += value })
const exited = once(child, 'exit')
let browser, page
const names = ['安全', '备份', '老资料', '录屏', '课程资料', '照片', '2026年秋季学期课程资料与项目研究归档',
  '年度研究报告_最终审核版本_请保留完整文件名.pdf',
  'Design review and implementation notes for October 2026.docx',
  '20261014_abcdef0123456789abcdef0123456789abcdef0123456789.png']
const errors = []
try {
  const address = await until(async () => {
    if (child.exitCode !== null) throw new Error(output)
    return readFile(capture, 'utf8').catch(() => null)
  })
  browser = await loadPlaywright().chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined, args: ['--no-sandbox'] })
  const context = await browser.newContext({ locale: 'zh-CN', colorScheme: 'dark', viewport: { width: 390, height: 780 } })
  page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', error => errors.push(String(error)))
  await page.goto(address)
  await page.getByRole('button', { name: '个人文档库', exact: true }).click()
  await page.getByRole('button', { name: '研究报告.txt', exact: true }).waitFor()
  const folderURL = page.url()
  await page.route('**/api/files?*', async route => {
    const response = await route.fetch()
    const body = await response.json()
    if (body.folder) {
      body.entries = names.map((name, i) => ({ ...body.entries[i % body.entries.length], name,
        id: `density-${i}`, directory: i < 7, size: 481024, modified_at: '2026-10-14T06:30:00Z', thumbnail: false }))
    }
    await route.fulfill({ response, json: body })
  })
  await page.reload()
  await page.getByRole('button', { name: '安全', exact: true }).waitFor()
  for (const width of baseline ? [390] : [320, 360, 390, 430, 640, 768, 1024, 1440]) {
    await page.setViewportSize({ width, height: 780 })
    const row = page.getByRole('row').filter({ has: page.getByRole('button', { name: '安全', exact: true }) })
    await row.scrollIntoViewIfNeeded()
    const height = (await row.boundingBox()).height
    const overflow = await page.getByRole('table').evaluate(table => table.scrollWidth > table.parentElement.clientWidth + 1)
    if (!baseline) {
      assert.equal(overflow, false, `table overflows at ${width}px`)
      if (width < 768) {
        assert.ok(height <= 68, `short row is ${height}px tall at ${width}px`)
        const file = page.getByRole('button', { name: '安全', exact: true })
        const icon = await file.locator('img').boundingBox()
        const line = await file.getByText('安全', { exact: true }).boundingBox()
        assert.ok(Math.abs(icon.y + icon.height / 2 - line.y - line.height / 2) <= 2, 'icon and first filename line are misaligned')
        const action = page.getByRole('button', { name: '操作: 安全', exact: true })
        const box = await action.boundingBox()
        assert.ok(box.width >= 44 && box.height >= 44, 'menu touch target is too small')
        assert.ok((await file.boundingBox()).height >= 44, 'file touch target is too small')
        await action.click()
        await page.getByRole('menuitem', { name: '下载', exact: true }).waitFor()
        await page.keyboard.press('Escape')
      }
      for (const name of names.slice(6)) {
        const text = page.getByRole('button', { name, exact: true }).getByText(name, { exact: true })
        assert.equal(await text.evaluate(element => element.scrollWidth <= element.clientWidth + 1 && element.scrollHeight <= element.clientHeight + 1), true, `clipped name at ${width}px: ${name}`)
      }
    }
    await page.getByRole('main').getByRole('heading', { name: '我的文件', exact: true }).click()
    await takeScreenshot(page, screenshots, `files-dark-${width}`)
    console.log(`Files ${width}px: short row ${height}px, overflow ${overflow}`)
  }
  if (!baseline) {
    await page.setViewportSize({ width: 390, height: 780 })
    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto(folderURL)
    await page.getByRole('button', { name: '安全', exact: true }).waitFor()
    await takeScreenshot(page, screenshots, 'files-light-390')

    // A library keeps quota and permission visible in the compact metadata.
    await page.goto(new URL(address).origin)
    await page.getByRole('button', { name: '个人文档库', exact: true }).waitFor()
    await takeScreenshot(page, screenshots, 'libraries-light-390')

    await page.route('**/api/tasks*', route => route.fulfill({ json: [
      { id: 'density-task', name: names[8], account: 'campus', kind: 'upload', state: 'running',
        created: '2026-10-14T06:30:00Z', source: names[8], destination: '/课程资料', phase: 'upload', percent: 50, total: 481024, done: 240512 },
      { id: 'density-complete', name: '研究报告.pdf', account: 'campus', kind: 'download', state: 'completed',
        created: '2026-10-14T06:30:00Z', source: '/课程资料/研究报告.pdf', destination: 'Downloads', percent: 100, total: 1024, done: 1024 }
    ] }))
    await page.goto(`${new URL(address).origin}/transfers`)
    await page.getByText(names[8], { exact: true }).waitFor()
    for (const width of [320, 390]) {
      await page.setViewportSize({ width, height: 780 })
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
      const task = page.locator('article').filter({ has: page.getByText(names[8], { exact: true }) })
      const name = task.getByText(names[8], { exact: true })
      assert.equal(await name.evaluate(e => e.scrollWidth <= e.clientWidth && e.scrollHeight <= e.clientHeight), true)
      assert.equal(await task.getByRole('button', { name: '取消操作', exact: true }).count(), 1)
      await takeScreenshot(page, screenshots, `transfers-light-${width}`)
    }
  }
  assert.deepEqual(errors, [])
} catch (error) {
  await takeScreenshot(page, screenshots, 'failure').catch(() => {})
  console.error(output, errors)
  throw error
} finally {
  await browser?.close()
  if (child.exitCode === null) {
    child.kill('SIGTERM')
    await exited
  }
  await rm(temporary, { recursive: true, force: true })
}

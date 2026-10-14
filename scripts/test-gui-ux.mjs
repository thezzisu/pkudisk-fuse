// Real GUI fixture with deterministic service failures and narrow-screen checks.
// These regressions cover misleading empty states, recovery, and reachable actions.
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { loadPlaywright, until, takeScreenshot } from './lib/gui-test.mjs'

if (!process.argv[2]) throw new Error('Usage: node scripts/test-gui-ux.mjs GUI-TEST-BINARY [SCREENSHOTS]')
const temporary = await mkdtemp(join(tmpdir(), 'pkudisk-gui-ux-'))
const capture = join(temporary, 'activation')
const screenshots = process.argv[3] && resolve(process.argv[3])
const child = spawn(resolve(process.argv[2]), ['-test.run=^TestGUIBrowserFixtureProcess$', '-test.timeout=180s'], {
  env: { ...process.env, PKUDISK_GUI_BROWSER_CAPTURE: capture }, stdio: ['ignore', 'pipe', 'pipe']
})
let output = ''
child.stdout.on('data', value => { output += value })
child.stderr.on('data', value => { output += value })
const exited = once(child, 'exit')
let browser
let page
let releaseDriver
const errors = []
const failure = {
  status: 500,
  contentType: 'application/json',
  body: JSON.stringify({ error: { code: 'ERR_INTERNAL', message: 'Controlled UX regression fixture failure' } })
}

try {
  const address = await until(async () => {
    if (child.exitCode !== null) throw new Error(output)
    return readFile(capture, 'utf8').catch(() => null)
  }, 'fixture startup')
  const origin = new URL(address).origin
  const { chromium } = loadPlaywright()
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined, args: ['--no-sandbox'] })
  const context = await browser.newContext({ locale: 'zh-CN', viewport: { width: 390, height: 844 } })
  page = await context.newPage()
  page.setDefaultTimeout(15000)
  page.on('pageerror', error => errors.push(String(error)))
  await page.goto(address)
  await page.getByRole('button', { name: '个人文档库', exact: true }).waitFor()

  await page.goto(`${origin}/recycle`)
  await page.getByRole('cell', { name: '已删除的笔记.txt', exact: true }).waitFor()
  await page.getByRole('textbox', { name: '筛选当前目录…', exact: true }).fill('no-such-recycled-item')
  await page.getByRole('heading', { name: '没有匹配的文件', exact: true }).waitFor()
  assert.equal(await page.getByRole('button', { name: '清空当前列表', exact: true }).isDisabled(), true)
  await takeScreenshot(page, screenshots, 'recycle-filter-empty')
  await page.getByRole('button', { name: '清除', exact: true }).click()
  await page.getByRole('cell', { name: '已删除的笔记.txt', exact: true }).waitFor()

  for (const resource of ['recovery', 'files', 'recycle']) {
    const pattern = `**/api/${resource}*`
    await page.route(pattern, route => route.fulfill(failure))
    await page.goto(origin + (resource === 'files' ? '/' : `/${resource}`))
    await page.getByRole('button', { name: '错误详情', exact: true }).waitFor()
    for (const text of ['未发现待恢复写入', '此目录为空', '回收站为空']) {
      assert.equal(await page.getByRole('heading', { name: text, exact: true }).count(), 0, `${resource}: misleading empty state after failure`)
    }
    assert.equal(await page.getByText('没有数据', { exact: true }).count(), 0)
    await takeScreenshot(page, screenshots, `${resource}-error`)
    await page.unroute(pattern)
    if (resource === 'recycle') {
      await page.getByRole('button', { name: '刷新', exact: true }).click()
      await page.getByRole('cell', { name: '已删除的笔记.txt', exact: true }).waitFor()
    }
  }

  let driverRequested = false
  const pendingDriver = new Promise(resolve => { releaseDriver = resolve })
  await page.route('**/api/mounts/support', async route => {
    driverRequested = true
    await pendingDriver
    await route.fulfill(failure)
  })
  await page.goto(`${origin}/settings`)
  await until(() => driverRequested, 'driver status request')
  await page.getByLabel('正在加载…', { exact: true }).waitFor()
  assert.equal(await page.getByText('尚未安装或不可用', { exact: true }).count(), 0, 'loading falsely marked missing driver')
  releaseDriver()
  await page.getByRole('button', { name: '错误详情', exact: true }).waitFor()
  await page.unroute('**/api/mounts/support')
  await page.getByRole('button', { name: '刷新', exact: true }).click()
  await page.getByText('已就绪', { exact: true }).waitFor()

  await page.goto(`${origin}/recovery`)
  await page.getByRole('heading', { name: '未发现待恢复写入', exact: true }).waitFor()
  assert.equal(await page.getByText('复制这些文件前，请停止对应挂载。确认恢复副本完整后，再处理原缓存文件。', { exact: true }).count(), 0)
  await page.getByRole('button', { name: '浏览…', exact: true }).click()
  const picker = page.getByRole('dialog')
  await picker.getByRole('button', { name: '主目录', exact: true }).waitFor()
  assert.equal(await picker.getByRole('button', { name: '主目录', exact: true }).count(), 1)
  await picker.getByRole('button', { name: '取消', exact: true }).click()

  await page.goto(origin)
  await page.getByRole('button', { name: '个人文档库', exact: true }).click()
  await page.getByRole('button', { name: '操作: 研究报告.txt', exact: true }).click()
  await page.getByRole('menuitem', { name: '历史版本', exact: true }).click()
  const history = page.getByRole('dialog', { name: '历史版本', exact: true })
  const restore = history.getByRole('button', { name: '恢复此版本', exact: true }).last()
  await restore.waitFor()
  await until(async () => {
    const dialog = await history.boundingBox()
    const button = await restore.boundingBox()
    return dialog && button && button.x >= dialog.x && button.x + button.width <= dialog.x + dialog.width
  }, 'mobile restore action visible without horizontal scrolling')
  await restore.click()
  await page.getByRole('dialog').last().getByRole('button', { name: '取消', exact: true }).click()
  await takeScreenshot(page, screenshots, 'history-mobile')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true)
  assert.deepEqual(errors, [])
  console.log('GUI UX passed: filtered empty recovery, API error states, retry, driver loading, picker shortcuts, and visible mobile history actions.')
} catch (error) {
  console.error('GUI UX fixture:', output)
  console.error('GUI UX browser errors:', errors)
  console.error('GUI UX page:', await page?.locator('body').innerText({ timeout: 1000 }).catch(() => 'unavailable'))
  await takeScreenshot(page, screenshots, 'ux-failure').catch(() => {})
  throw error
} finally {
  releaseDriver?.()
  await browser?.close()
  if (child.exitCode === null) {
    child.kill('SIGTERM')
    await Promise.race([exited, new Promise(resolve => setTimeout(resolve, 3000))])
    if (child.exitCode === null) child.kill('SIGKILL')
    await exited
  }
  await rm(temporary, { recursive: true, force: true })
}

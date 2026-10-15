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

  // A sidebar navigation must push a history entry even when its link omits
  // the account query. Canonicalizing that URL must not replace the folder.
  await page.getByRole('button', { name: '个人文档库', exact: true }).click()
  await page.getByRole('button', { name: '课程资料', exact: true }).click()
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()
  const beforeSidebar = page.url()
  await page.getByRole('button', { name: '打开侧边栏', exact: true }).click()
  await page.getByRole('link', { name: /^传输任务/ }).click()
  await page.getByRole('main', { name: '传输与操作', exact: true }).waitFor()
  await page.goBack()
  await until(() => new URL(page.url()).pathname === '/', 'sidebar Back returns to files')
  assert.equal(page.url(), beforeSidebar, 'sidebar navigation replaced the current folder history')
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()
  await page.goto(origin)
  await page.getByRole('button', { name: '个人文档库', exact: true }).waitFor()

  // Route state is the source of truth across fresh WebViews and browser history.
  // Restore a nondefault account before issuing any account-scoped file requests.
  await page.getByLabel('账号', { exact: true }).click()
  await page.getByRole('option', { name: 'work', exact: true }).click()
  await until(() => new URL(page.url()).searchParams.get('account') === 'work', 'account reflected in URL')
  await page.getByRole('button', { name: '个人文档库', exact: true }).click()
  await page.getByRole('button', { name: '工作报告.txt', exact: true }).waitFor()
  const libraryPath = new URL(page.url()).searchParams.get('path')
  await page.getByRole('button', { name: '课程资料', exact: true }).click()
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()
  const folderPath = new URL(page.url()).searchParams.get('path')
  assert.notEqual(folderPath, libraryPath)
  await page.reload()
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()
  assert.equal(new URL(page.url()).searchParams.get('account'), 'work')
  assert.equal(new URL(page.url()).searchParams.get('path'), folderPath)
  await page.goBack()
  await page.getByRole('button', { name: '工作报告.txt', exact: true }).waitFor()
  assert.equal(new URL(page.url()).searchParams.get('path'), libraryPath)
  await page.goForward()
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()

  await page.getByRole('button', { name: '高级选项', exact: true }).click()
  await page.getByRole('checkbox', { name: '仅搜索当前目录', exact: true }).check()
  await page.getByRole('spinbutton').fill('25')
  await page.keyboard.press('Escape')
  await page.getByRole('textbox', { name: '搜索', exact: true }).fill('读书')
  await page.getByRole('button', { name: '搜索', exact: true }).click()
  await until(() => new URL(page.url()).searchParams.get('q') === '读书', 'search reflected in URL')
  assert.equal(new URL(page.url()).searchParams.get('scope'), 'current')
  assert.equal(new URL(page.url()).searchParams.get('limit'), '25')
  await page.reload()
  await page.getByRole('button', { name: '读书笔记.md', exact: true }).waitFor()
  assert.equal(await page.getByRole('textbox', { name: '搜索', exact: true }).inputValue(), '读书')
  await page.goBack()
  await until(async () => (await page.getByRole('textbox', { name: '搜索', exact: true }).inputValue()) === '', 'back restores search input')
  await page.goForward()
  await until(async () => (await page.getByRole('textbox', { name: '搜索', exact: true }).inputValue()) === '读书', 'forward restores search input')

  await page.goto(`${origin}/transfers?account=work`)
  await page.getByRole('tab', { name: '进行中', exact: true }).click()
  await until(() => new URL(page.url()).searchParams.get('status') === 'active', 'task tab reflected in URL')
  await page.reload()
  await page.getByRole('tab', { name: '进行中', exact: true, selected: true }).waitFor()
  assert.equal(await page.getByLabel('账号', { exact: true }).innerText(), 'work')

  await page.goto(`${origin}/recycle?account=work&library=${encodeURIComponent(libraryPath)}`)
  await page.getByRole('cell').getByText('已删除的笔记.txt', { exact: true }).waitFor()
  assert.equal(await page.getByLabel('账号', { exact: true }).innerText(), 'work')
  assert.equal(new URL(page.url()).searchParams.get('library'), libraryPath)
  await page.reload()
  await page.getByRole('cell').getByText('已删除的笔记.txt', { exact: true }).waitFor()
  assert.equal(new URL(page.url()).searchParams.get('account'), 'work')
  await takeScreenshot(page, screenshots, 'route-restored-mobile')

  await page.goto(`${origin}/?account=removed-account&path=${encodeURIComponent(folderPath)}&q=stale`)
  await page.getByRole('button', { name: '个人文档库', exact: true }).waitFor()
  assert.equal(new URL(page.url()).searchParams.get('account'), 'campus')
  assert.equal(new URL(page.url()).searchParams.has('path'), false)
  assert.equal(new URL(page.url()).searchParams.has('q'), false)

  // Folder grouping is independent of the chosen within-group sort direction.
  // Distinct dates make the mobile sorting control observable without a hidden
  // desktop-only table column.
  const datedFiles = async route => {
    const response = await route.fetch()
    const body = await response.json()
    for (const entry of body.entries || []) {
      entry.modified_at = entry.name === '资料.zip' ? '2026-10-14T12:00:00Z' : '2020-01-01T00:00:00Z'
    }
    await route.fulfill({ response, json: body })
  }
  await page.route('**/api/files?*', datedFiles)
  await page.getByRole('button', { name: '个人文档库', exact: true }).click()
  await page.getByRole('button', { name: '研究报告.txt', exact: true }).waitFor()
  const fileOrder = () => page.locator('tbody tr td:first-child button[title]').evaluateAll(nodes => nodes.map(node => node.getAttribute('title')))
  assert.deepEqual(await fileOrder(), ['归档', '课程资料', '研究报告.txt', '资料.zip'])
  const reportIcon = page.getByRole('button', { name: '研究报告.txt', exact: true }).locator('img')
  const archiveIcon = page.getByRole('button', { name: '资料.zip', exact: true }).locator('img')
  assert.match(await reportIcon.getAttribute('src'), /\/material-icons\/document\.svg$/)
  assert.match(await archiveIcon.getAttribute('src'), /\/material-icons\/zip\.svg$/)
  await page.getByRole('button', { name: '排序', exact: true }).click()
  await page.getByRole('combobox', { name: '排序', exact: true }).click()
  await page.getByRole('option', { name: '修改时间：最新在前', exact: true }).click()
  await until(async () => JSON.stringify(await fileOrder()) === JSON.stringify(['归档', '课程资料', '资料.zip', '研究报告.txt']), 'newest first within folder groups')
  await page.getByRole('switch', { name: '文件夹优先', exact: true }).uncheck()
  await until(async () => (await fileOrder())[0] === '资料.zip', 'newest file can precede folders')
  await page.keyboard.press('Escape')
  await takeScreenshot(page, screenshots, 'files-sorted-mobile')
  await page.reload()
  await page.getByRole('button', { name: '资料.zip', exact: true }).waitFor()
  assert.equal((await fileOrder())[0], '资料.zip', 'sort preference was lost after reload')
  await page.getByRole('button', { name: '排序', exact: true }).click()
  await page.getByRole('combobox', { name: '排序', exact: true }).click()
  await page.getByRole('option', { name: '名称：A → Z', exact: true }).click()
  await page.getByRole('switch', { name: '文件夹优先', exact: true }).check()
  await page.keyboard.press('Escape')
  await page.unroute('**/api/files?*', datedFiles)

  // Narrow rows keep the filename as the primary line and move size/date and
  // actions to a second line. Exercise long CJK, ASCII, and unbroken names at
  // a compact phone width so the table never introduces horizontal scrolling.
  const longNames = [
    '2026年秋季学期课程资料与项目研究归档',
    '年度研究报告_最终审核版本_请保留完整文件名.pdf',
    'Design review and implementation notes for October 2026.docx',
    '20261014_abcdef0123456789abcdef0123456789abcdef0123456789.png'
  ]
  const longFiles = async route => {
    const response = await route.fetch()
    const body = await response.json()
    body.entries = longNames.map((name, index) => ({
      ...body.entries[index % body.entries.length],
      id: `ux-long-${index}`,
      name,
      directory: index === 0,
      size: 481024,
      modified_at: '2026-10-14T06:30:00Z',
      thumbnail: false
    }))
    await route.fulfill({ response, json: body })
  }
  await page.route('**/api/files?*', longFiles)
  await page.setViewportSize({ width: 320, height: 844 })
  await page.reload()
  for (const name of longNames) await page.locator(`button[title="${name}"]`).waitFor()
  assert.equal(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    true,
    'long mobile filenames introduced horizontal overflow'
  )
  for (const name of longNames) {
    const action = page.locator(`button[aria-label="操作: ${name}"]`).filter({ visible: true })
    assert.equal(await action.count(), 1, `mobile action menu is missing for ${name}`)
    const rightEdge = await page.locator(`button[title="${name}"]`).evaluate(node =>
      node.getBoundingClientRect().right
    )
    assert.ok(rightEdge <= 320, `mobile filename row overflows for ${name}`)
  }
  await takeScreenshot(page, screenshots, 'long-filenames-mobile')
  await page.setViewportSize({ width: 390, height: 844 })
  await page.unroute('**/api/files?*', longFiles)

  await page.goto(`${origin}/recycle`)
  await page.getByRole('cell').getByText('已删除的笔记.txt', { exact: true }).waitFor()
  await page.getByRole('textbox', { name: '筛选当前目录…', exact: true }).fill('no-such-recycled-item')
  await page.getByRole('heading', { name: '没有匹配的文件', exact: true }).waitFor()
  assert.equal(await page.getByRole('button', { name: '清空当前列表', exact: true }).isDisabled(), true)
  await takeScreenshot(page, screenshots, 'recycle-filter-empty')
  await page.getByRole('button', { name: '清除', exact: true }).click()
  await page.getByRole('cell').getByText('已删除的笔记.txt', { exact: true }).waitFor()

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
      await page.getByRole('cell').getByText('已删除的笔记.txt', { exact: true }).waitFor()
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
  console.log('GUI UX passed: URL account/folder/search/tab restoration, browser history, filtered empty recovery, API error states, retry, driver loading, picker shortcuts, and visible mobile history actions.')
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

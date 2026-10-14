// Integrated development test: real Nuxt/Vite HMR behind the Go GUI origin.
// Only a temporary attribute in LocaleControls.vue changes, and finally removes
// that exact attribute so concurrent unrelated source edits are preserved.
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { once } from 'node:events'
import { mkdtemp, mkdir, readFile, writeFile, rm } from 'node:fs/promises'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { until, takeScreenshot, loadPlaywright } from './lib/gui-test.mjs'

if (process.platform !== 'linux' || !process.argv[2]) {
  throw new Error('Usage (Linux): node scripts/test-gui-hmr.mjs BINARY [SCREENSHOTS]')
}
const root = fileURLToPath(new URL('../', import.meta.url))
const web = join(root, 'web')
const source = join(web, 'app/components/LocaleControls.vue')
const binary = resolve(process.argv[2])
const screenshots = process.argv[3] && resolve(process.argv[3])
const temporary = await mkdtemp(join(tmpdir(), 'pkudisk-gui-hmr-'))
const capture = join(temporary, 'activation')
const marker = `hmr-${randomUUID()}`
const initial = ` data-pkudisk-hmr-probe="${marker}-initial"`
const updated = ` data-pkudisk-hmr-probe="${marker}-updated"`
const original = await readFile(source, 'utf8')
assert.equal(original.includes('data-pkudisk-hmr-probe'), false, 'another HMR probe is active')
assert.ok(original.includes('<div class='), 'LocaleControls root element changed')
const children = []
let browser
let page
let changed = false
const errors = []
const violations = []
const sockets = []

function start(command, args, env, cwd = root) {
  const child = spawn(command, args, { cwd, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  const record = { child, output: '', error: null, exited: null }
  child.stdout.on('data', data => { record.output += data })
  child.stderr.on('data', data => { record.output += data })
  record.exited = new Promise(resolve => {
    child.on('error', error => { record.error = error; resolve() })
    child.on('exit', resolve)
  })
  children.push(record)
  return record
}
function running(record) {
  if (record.error) throw record.error
  if (record.child.exitCode !== null || record.child.signalCode !== null) throw new Error(`Process exited early: ${record.output}`)
}
async function freePort() {
  const server = createServer()
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const port = server.address().port
  await new Promise(resolve => server.close(resolve))
  return port
}
async function stop(record) {
  if (record.child.exitCode !== null || !record.child.pid) return
  try { process.kill(-record.child.pid, 'SIGTERM') } catch { /* already stopped */ }
  await Promise.race([record.exited, new Promise(resolve => setTimeout(resolve, 4000))])
  if (record.child.exitCode === null) {
    try { process.kill(-record.child.pid, 'SIGKILL') } catch { /* already stopped */ }
    await record.exited
  }
}

try {
  await writeFile(source, original.replace('<div class=', `<div${initial} class=`))
  changed = true
  const port = await freePort()
  const target = `http://127.0.0.1:${port}`
  const nuxt = start(join(web, 'node_modules/.bin/nuxt'), ['dev', '--cwd', web, '--host', '127.0.0.1', '--port', String(port)], {
    ...process.env, NUXT_TELEMETRY_DISABLED: '1', NO_COLOR: '1'
  }, web)
  await until(async () => {
    running(nuxt)
    return fetch(target, { signal: AbortSignal.timeout(2000) }).then(response => response.ok, () => false)
  }, 'Nuxt dev startup', 90000)
  const helpers = join(temporary, 'helpers')
  await mkdir(helpers, { mode: 0o700 })
  await writeFile(join(helpers, 'xdg-open'), '#!/bin/sh\numask 077\nprintf "%s" "$1" > "$PKUDISK_TEST_URL"\n', { mode: 0o700 })
  const gui = start(binary, ['gui', '--foreground'], {
    ...process.env,
    PATH: `${helpers}:${process.env.PATH}`,
    XDG_CONFIG_HOME: join(temporary, 'config'),
    XDG_STATE_HOME: join(temporary, 'state'),
    XDG_CACHE_HOME: join(temporary, 'cache'),
    DBUS_SESSION_BUS_ADDRESS: '', DISPLAY: ':gui-hmr-test', WAYLAND_DISPLAY: '',
    PKUDISK_TEST_URL: capture, PKUDISK_DESKTOP_LAUNCH: '', PKUDISK_GUI_DEV_PROXY: target
  })
  const address = await until(async () => {
    running(gui)
    return readFile(capture, 'utf8').catch(() => null)
  }, 'GUI activation')
  const origin = new URL(address).origin
  const { chromium } = loadPlaywright()
  browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE || undefined, args: ['--no-sandbox'] })
  const context = await browser.newContext({ locale: 'zh-CN', viewport: { width: 1280, height: 900 } })
  page = await context.newPage()
  page.setDefaultTimeout(30000)
  page.on('pageerror', error => errors.push(String(error)))
  page.on('websocket', socket => sockets.push(socket.url()))
  await page.exposeFunction('recordHMRViolation', directive => violations.push(directive))
  await page.addInitScript(() => document.addEventListener('securitypolicyviolation', event => window.recordHMRViolation(event.violatedDirective)))
  await page.goto(address, { timeout: 60000 })
  await page.getByRole('main').waitFor()
  await page.locator(`[data-pkudisk-hmr-probe="${marker}-initial"]`).first().waitFor({ state: 'attached' })
  await until(() => sockets.some(value => new URL(value).host === new URL(origin).host), 'same-origin Vite websocket')
  const session = await page.evaluate(() => sessionStorage.getItem('pkudisk-gui-session'))
  assert.equal(session?.length, 43)
  await page.evaluate(value => { window.__pkudiskHmrDocument = value }, marker)
  const before = await readFile(source, 'utf8')
  assert.ok(before.includes(initial), 'HMR marker changed before the update')
  await writeFile(source, before.replace(initial, updated))
  await page.locator(`[data-pkudisk-hmr-probe="${marker}-updated"]`).first().waitFor({ state: 'attached' })
  assert.equal(await page.evaluate(() => window.__pkudiskHmrDocument), marker, 'HMR reloaded the document')
  assert.equal(await page.evaluate(() => sessionStorage.getItem('pkudisk-gui-session')), session, 'HMR replaced the GUI session')
  assert.equal((await context.request.get(`${origin}/api/status`, { headers: { Authorization: `Bearer ${session}` } })).status(), 200)
  assert.ok(sockets.every(value => new URL(value).host === new URL(origin).host), 'Vite bypassed the Go websocket proxy')
  assert.deepEqual(errors, [])
  assert.deepEqual(violations, [])
  assert.equal(gui.output.includes('ticket='), false)
  await takeScreenshot(page, screenshots, 'gui-hmr')
  console.log('GUI HMR passed: real Nuxt update, same-origin websocket, preserved document/session, authenticated Go API, no CSP violations.')
} catch (error) {
  console.error('HMR browser errors:', errors, 'CSP:', violations)
  for (const record of children) console.error(record.output.slice(-12000))
  await takeScreenshot(page, screenshots, 'gui-hmr-failure').catch(() => {})
  throw error
} finally {
  try {
    if (changed) {
      const current = await readFile(source, 'utf8')
      await writeFile(source, current.replace(initial, '').replace(updated, ''))
    }
  } finally {
    await browser?.close()
    for (const record of children.reverse()) await stop(record)
    await rm(temporary, { recursive: true, force: true })
  }
}

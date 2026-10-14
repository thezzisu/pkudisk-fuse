// Run only on a disposable emulator. No fixture hooks are compiled into the APK.
// Usage: node test/e2e.mjs emulator-5580 [path/to/app-debug.apk]
import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { mkdir, writeFile } from 'node:fs/promises'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { loadPlaywright, until } from '../../../scripts/lib/gui-test.mjs'
import { startAnyShareFixture } from '../../../scripts/lib/anyshare-fixture.mjs'

const run = promisify(execFile)
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const { _android: android } = loadPlaywright()
const serial = process.argv[2]
assert.match(serial || '', /^emulator-\d+$/, 'Pass the serial of a disposable emulator')
const apk = resolve(process.argv[3] || `${root}/app/build/outputs/apk/debug/app-debug.apk`)
const adb = process.env.ADB || `${process.env.ANDROID_HOME}/platform-tools/adb`
const pkg = 'su.zzi.pkudiskfuse'
const artifacts = resolve(root, 'build/e2e')
await mkdir(artifacts, { recursive: true })
async function device(...args) { return (await run(adb, ['-s', serial, ...args], { maxBuffer: 4 * 1024 * 1024, timeout: 30000 })).stdout.trim() }
async function nativeXML() {
  await device('shell', 'uiautomator', 'dump', '/sdcard/pkudisk-test-window.xml')
  return device('shell', 'cat', '/sdcard/pkudisk-test-window.xml')
}
async function nativeTap(text) {
  const xml = await nativeXML()
  const node = [...xml.matchAll(/<node\b[^>]+/g)].find(([node]) => node.toLowerCase().includes(`text="${text.toLowerCase()}"`) || node.toLowerCase().includes(`content-desc="${text.toLowerCase()}"`))?.[0]
  assert.ok(node, `Native control not found: ${text}`)
  const [, x1, y1, x2, y2] = node.match(/bounds="\[(\d+),(\d+)\]\[(\d+),(\d+)\]"/).map(Number)
  await device('shell', 'input', 'tap', String(Math.round((x1+x2)/2)), String(Math.round((y1+y2)/2)))
}
async function launch() { await device('shell', 'am', 'start', '-W', '-n', `${pkg}/.MainActivity`) }
async function appPID() { return device('shell', 'pidof', pkg).catch(() => '') }
async function instance() {
  try { return JSON.parse(await device('shell', 'run-as', pkg, 'cat', 'files/service/gui/instance.json')) }
  catch { return null }
}
async function screenshot(name) {
  const result = await run(adb, ['-s', serial, 'exec-out', 'screencap', '-p'], { encoding: 'buffer', maxBuffer: 8*1024*1024, timeout: 15000 })
  await writeFile(`${artifacts}/${name}.png`, result.stdout)
}
async function taskNotifications() {
  const dump = await device('shell', 'dumpsys', 'notification', '--noredact')
  const active = dump.split('  Notification attention state:')[0]
  return active.split(/(?=    NotificationRecord\()/).filter(record => record.startsWith('    NotificationRecord(') && record.split('\n')[0].includes(`pkg=${pkg} `))
}
async function download(name) {
  await page.getByRole('button', { name: new RegExp(`^(操作|Actions): ${name.replace('.', '\\.')}$`) }).click()
  await page.getByRole('menuitem', { name: /^(下载|Download)$/ }).click()
  await page.getByRole('dialog').getByRole('button', { name: /^(开始传输|Start transfer)$/ }).click()
  await page.getByRole('dialog').waitFor({ state: 'hidden' })
}
let androidDevice
let page
const upstream = await startAnyShareFixture({ profile: 'android' })
const upstreamPort = new URL(upstream.origin).port
try {
  assert.equal(await device('shell', 'getprop', 'ro.kernel.qemu'), '1', 'Never run against a real phone')
  const packaged = (await run('unzip', ['-Z1', apk])).stdout
  assert.ok(packaged.includes('lib/x86_64/libgojni.so'), 'gomobile JNI library missing')
  assert.ok(!packaged.includes('libpkudisk.so'), 'obsolete executable was packaged')
  await device('install', '-r', apk)
  await device('shell', 'am', 'force-stop', pkg)
  await device('shell', 'pm', 'clear', pkg)
  await device('shell', 'pm', 'grant', pkg, 'android.permission.POST_NOTIFICATIONS')
  // Prove the app has no dependency on the external Chrome application.
  await device('shell', 'pm', 'disable-user', '--user', '0', 'com.android.chrome')
  await device('reverse', `tcp:${upstreamPort}`, `tcp:${upstreamPort}`)
  await device('shell', 'rm', '-f', '/sdcard/Download/android-report.txt', '/sdcard/Download/移动端下载.txt', '/sdcard/Download/background-transfer.bin', '/sdcard/Download/background-second.bin')
  await device('shell', 'run-as', pkg, 'mkdir', '-p', 'files/service')
  const configPath = `${artifacts}/test-accounts.json`
  await writeFile(configPath, JSON.stringify(upstream.config), { mode: 0o600 })
  await device('push', configPath, '/data/local/tmp/pkudisk-test-accounts.json')
  await device('shell', 'run-as', pkg, 'cp', '/data/local/tmp/pkudisk-test-accounts.json', 'files/service/accounts.json')
  await device('shell', 'run-as', pkg, 'chmod', '700', 'files/service')
  await device('shell', 'run-as', pkg, 'chmod', '600', 'files/service/accounts.json')
  await launch()
  const running = await until(instance, 'gomobile service listening')
  const pid = await appPID()
  assert.ok(pid, 'Application process was not started')
  assert.equal(await device('shell', 'pidof', 'libpkudisk.so').catch(() => ''), '')
  androidDevice = (await android.devices()).find(value => value.serial() === serial)
  assert.ok(androidDevice, 'Playwright could not connect to the emulator')
  const webview = await androidDevice.webView({ pkg })
  page = await webview.page()
  await until(() => page.url().startsWith(running.origin + '/'), 'embedded activation')
  page.setDefaultTimeout(20000)
  const errors = []
  page.on('pageerror', error => errors.push(error.message))
  await page.getByRole('button', { name: 'Android test library', exact: true }).click()
  await page.getByRole('button', { name: 'android-report.txt', exact: true }).waitFor()
  assert.equal(new URL(page.url()).hash, '', 'activation ticket retained in address bar')
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true, 'mobile horizontal overflow')
  await screenshot('01-files')
  let origin = new URL(page.url()).origin
  const info = await instance()
  assert.equal(info.origin, origin)
  const headers = await page.evaluate(async () => {
    const res = await fetch('/api/status', { headers: { Authorization: `Bearer ${sessionStorage.getItem('pkudisk-gui-session')}` } })
    return { status: res.status, data: await res.json() }
  })
  assert.equal(headers.status, 200)
  assert.equal(headers.data.tray, false)
  assert.equal(headers.data.version, '0.5.0')
  await until(async () => (await taskNotifications()).length === 0, 'idle foreground has no persistent notification')
  assert.deepEqual(headers.data.flags, { isMobile: true, canMount: false, canBrowseLocalFS: false })
  const localAccess = await page.evaluate(async () => {
    const headers = { Authorization: `Bearer ${sessionStorage.getItem('pkudisk-gui-session')}`, 'Content-Type': 'application/json' }
    const results = await Promise.all([
      fetch('/api/local-fs', { headers }),
      fetch('/api/local-fs/mkdir', { method: 'POST', headers, body: JSON.stringify({ path: '/', name: 'test' }) }),
      fetch('/api/recovery', { headers }),
      fetch('/api/tasks', { method: 'POST', headers, body: JSON.stringify({ kind: 'upload', account: 'campus', source: '/', destination: '/' }) })
    ])
    return results.map(response => response.status)
  })
  assert.deepEqual(localAccess, [403, 403, 403, 403], 'mobile must reject local filesystem operations')
  const mountSupport = await page.evaluate(async () => (await fetch('/api/mounts/support', { headers: { Authorization: `Bearer ${sessionStorage.getItem('pkudisk-gui-session')}` } })).json())
  assert.equal(mountSupport.platform, 'android')
  assert.equal(mountSupport.available, false)
  const mountRequests = []
  const trackMount = request => { if (new URL(request.url()).pathname.startsWith('/api/mounts')) mountRequests.push(request.url()) }
  page.on('request', trackMount)
  await page.goto(`${origin}/settings`)
  await page.getByRole('main').waitFor()
  assert.doesNotMatch(await page.getByRole('main').innerText(), /FUSE|WinFsp|Mount driver|挂载驱动|Desktop service|桌面服务/)
  await screenshot('01-mobile-settings')
  await page.goto(`${origin}/mounts`)
  await until(() => new URL(page.url()).pathname === '/', 'mobile mount route is hidden')
  assert.deepEqual(mountRequests, [], 'mobile UI requested mount functionality')
  page.off('request', trackMount)
  await page.getByRole('button', { name: 'Android test library', exact: true }).click()
  assert.ok((await upstream.snapshot()).calls.includes('/anyshare/oauth2/login/refreshToken'))
  assert.equal(await page.evaluate(() => typeof window.pkudiskDownloads), 'undefined', 'custom JavaScript bridge was injected')
  // WebView's system document chooser and Android DownloadManager.
  const uploadPath = `${artifacts}/android-upload.txt`
  await writeFile(uploadPath, 'Android native picker upload\n')
  await device('push', uploadPath, '/sdcard/Download/android-upload.txt')
  await device('shell', 'am', 'broadcast', '-a', 'android.intent.action.MEDIA_SCANNER_SCAN_FILE', '-d', 'file:///sdcard/Download/android-upload.txt')
  await page.getByRole('button', { name: /^(上传|Upload)$/ }).click()
  await page.getByRole('dialog').getByRole('button', { name: /^(选择文件|Select files)$/ }).click()
  let xml = await nativeXML()
  if (xml.includes('text="Files"')) await nativeTap('Files')
  xml = await nativeXML()
  if (!xml.includes('android-upload.txt')) {
    await nativeTap('Show roots')
    await nativeTap('Downloads')
  }
  await nativeTap('android-upload.txt')
  await page.getByRole('dialog').getByText('android-upload.txt', { exact: true }).waitFor()
  await screenshot('02-native-upload')
  await page.getByRole('dialog').getByRole('button', { name: /^(开始传输|Start transfer)$/ }).click()
  await page.getByRole('button', { name: 'android-upload.txt', exact: true }).waitFor()
  const uploadedState = await upstream.snapshot()
  const uploaded = [...uploadedState.entries.values()].find(e => e.name === 'android-upload.txt')
  assert.equal(uploadedState.data.get(uploaded.id).toString(), 'Android native picker upload\n')
  for (const name of ['android-report.txt', '移动端下载.txt']) {
    await page.getByRole('button', { name: new RegExp(`^(操作|Actions): ${name.replace('.', '\\.')}$`) }).click()
    await page.getByRole('menuitem', { name: /^(下载|Download)$/ }).click()
    await page.getByRole('dialog').getByRole('button', { name: /^(开始传输|Start transfer)$/ }).click()
    await page.getByRole('dialog').waitFor({ state: 'hidden' })
    await until(async () => (await device('shell', 'cat', `/sdcard/Download/${name}`).catch(() => '')) === 'Android download test', `DownloadManager attachment ${name}`, 20000)
  }
  await page.goto(`${origin}/transfers`)
  for (const name of ['android-report.txt', '移动端下载.txt']) {
    const task = page.getByRole('article').filter({ has: page.getByText(name, { exact: true }) })
    await task.getByText(/^(Completed|已完成)$/).waitFor()
    assert.equal(await task.count(), 1, 'WebView handoff must remain a single visible task')
  }
  await screenshot('03-attachment-download')
  const removed = page.getByRole('article').filter({ has: page.getByText('移动端下载.txt', { exact: true }) })
  await removed.getByRole('button', { name: /^(Remove from history|移除历史记录)$/ }).click()
  await removed.waitFor({ state: 'hidden' })
  const remaining = await page.evaluate(async () => (await fetch('/api/tasks', { headers: { Authorization: `Bearer ${sessionStorage.getItem('pkudisk-gui-session')}` } })).json())
  assert.equal(remaining.filter(task => task.name === '移动端下载.txt').length, 0, 'clearing a download must remove both handoff records')
  // Idle background releases the Go service; returning resumes automatically.
  await device('shell', 'input', 'keyevent', 'KEYCODE_HOME')
  await until(async () => !(await instance()), 'idle background release')
  assert.equal((await taskNotifications()).length, 0)
  await launch()
  const resumed = await until(instance, 'automatic foreground resume')
  assert.notEqual(resumed.control, running.control, 'idle service was not recreated')
  origin = resumed.origin
  await until(() => page.url().startsWith(origin + '/'), 'fresh embedded activation after idle release')
  await page.getByRole('button', { name: 'Android test library', exact: true }).click()
  await screenshot('04-idle-resumed')
  // Two genuine throttled transfers exercise grouped native progress and lifetime.
  await download('background-transfer.bin')
  await download('background-second.bin')
  const progress = record => Number(record.match(/android\.progress=Integer \((\d+)\)/)?.[1] || 0)
  const firstProgress = await until(async () => {
    const records = await taskNotifications()
    return records.length >= 3 && records.some(record => progress(record) > 0) ? records : null
  }, 'native per-task progress notifications')
  const beforeProgress = Math.max(...firstProgress.map(progress))
  await writeFile(`${artifacts}/native-task-notifications.txt`, firstProgress.join('\n'))
  await device('shell', 'input', 'keyevent', 'KEYCODE_HOME')
  await until(async () => (await taskNotifications()).some(record => progress(record) > beforeProgress), 'progress advances in background')
  assert.equal((await instance()).control, resumed.control, 'active background tasks lost their service')
  await device('shell', 'cmd', 'statusbar', 'expand-notifications')
  // Let the shade animation finish before capturing it. UIAutomator waits for
  // global idleness, which progress notifications intentionally never provide.
  await new Promise(resolve => setTimeout(resolve, 600))
  await device('shell', 'input', 'swipe', '540', '1800', '540', '650', '450')
  await new Promise(resolve => setTimeout(resolve, 600))
  await screenshot('05-task-notifications')
  await device('shell', 'cmd', 'statusbar', 'collapse')
  // DownloadManager preallocates destination files, so size is not completion.
  const expectedHash = createHash('sha256').update(uploadedState.data.get('gns://library/slow-second')).digest('hex')
  await until(async () => (await device('shell', 'sha256sum', '/sdcard/Download/background-second.bin').catch(() => '')).startsWith(expectedHash), 'background transfer bytes complete', 45000)
  await until(async () => !(await instance()), 'completed background service releases itself', 15000)
  assert.equal((await taskNotifications()).length, 0, 'completed task notifications remained ongoing')
  await launch()
  origin = (await until(instance, 'resume after background transfers')).origin
  await until(() => page.url().startsWith(origin + '/'), 'resumed interface')
  await page.getByRole('main').waitFor()
  // Quit through the actual mobile UI, then verify Android removes the task.
  assert.equal(await page.title(), 'PKUDisk App')
  await page.getByRole('button', { name: /^(Open sidebar|打开侧边栏)$/ }).click()
  assert.equal(await page.getByRole('button', { name: /^(Quit desktop service|退出桌面服务)$/ }).count(), 0)
  await page.getByRole('button', { name: /^(Quit|退出)$/, exact: true }).click()
  const quitDialog = page.getByRole('dialog', { name: /^(Quit|退出)$/, exact: true })
  await quitDialog.getByRole('button', { name: /^(Confirm|确认)$/, exact: true }).waitFor()
  await screenshot('05-quit-confirm')
  await quitDialog.getByRole('button', { name: /^(Confirm|确认)$/, exact: true }).click()
  await until(async () => !(await instance()), 'Quit stops the shared service')
  await until(async () => {
    const recentTasks = (await device('shell', 'dumpsys', 'activity', 'recents')).split('  Recent tasks:')[1]
    return recentTasks !== undefined && !recentTasks.includes(pkg + '/')
  }, 'Quit removes the app task from Recents')
  await until(async () => !/ResumedActivity[=:][^\n]*su\.zzi\.pkudiskfuse/.test(await device('shell', 'dumpsys', 'activity', 'activities')), 'Quit closes the app window')
  await screenshot('06-quit-closed')
  // Restart, then force-stop the Java package. The Go library dies with the host.
  await launch()
  await until(instance, 'restart')
  const beforeForce = await instance()
  await device('shell', 'am', 'force-stop', pkg)
  await until(async () => !(await appPID()), 'force-stop ends the host process')
  // Set the test budget before the next service starts so Android schedules it.
  await device('shell', 'device_config', 'put', 'activity_manager', 'data_sync_fgs_timeout_duration', '2000')
  await launch()
  await until(async () => { const info = await instance(); return info && info.control !== beforeForce.control ? info : null }, 'restart after force-stop')
  assert.ok(await appPID())
  page = await (await androidDevice.webView({ pkg })).page()
  await page.getByRole('button', { name: 'Android test library', exact: true }).click()
  await device('shell', 'rm', '-f', '/sdcard/Download/background-transfer.bin')
  await download('background-transfer.bin')
  await until(async () => (await taskNotifications()).length > 0, 'active task starts Android foreground service')
  // Exercise Android's real dataSync timeout callback with a short test budget.
  await device('shell', 'input', 'keyevent', 'KEYCODE_HOME')
  await until(async () => !(await instance()), 'Android foreground-service timeout', 30000)
  // An unexpected timeout keeps the window available for the exit overlay.
  await launch()
  await page.getByRole('dialog', { name: /^(The service is not running\. Please exit the app\.|服务未运行，请退出应用)$/ }).waitFor()
  assert.equal(await page.getByRole('main').count(), 0, 'terminated workspace remained interactive')
  assert.equal(await instance(), null, 'returning to the timeout overlay restarted the service')
  await screenshot('07-unexpected-disconnect')
  assert.deepEqual(errors, [])
  console.log('PASS: Android APK install, in-process gomobile AAR, embedded WebView activation with Chrome disabled, saved login refresh, system picker upload, DownloadManager download, idle release/resume, grouped native background task progress, automatic release after tasks, mobile Quit closes app/Recents, restart, force-stop cleanup and unexpected Android timeout overlay.')
} catch (error) {
  await screenshot('failure').catch(() => {})
  if (page) await writeFile(`${artifacts}/jobs.json`, JSON.stringify(await page.evaluate(async () => (await fetch('/api/tasks', { headers: { Authorization: `Bearer ${sessionStorage.getItem('pkudisk-gui-session')}` } })).json()).catch(() => null), null, 2))
  await writeFile(`${artifacts}/upstream-calls.json`, JSON.stringify((await upstream.snapshot()).calls, null, 2))
  await writeFile(`${artifacts}/native-ui.xml`, await nativeXML().catch(() => ''))
  await writeFile(`${artifacts}/native.log`, await device('logcat', '-d', '-s', 'AndroidRuntime').catch(() => ''))
  throw error
} finally {
  await androidDevice?.close()
  await device('shell', 'am', 'force-stop', pkg).catch(() => {})
  await device('shell', 'pm', 'enable', 'com.android.chrome').catch(() => {})
  await device('shell', 'device_config', 'delete', 'activity_manager', 'data_sync_fgs_timeout_duration').catch(() => {})
  await device('reverse', '--remove', `tcp:${upstreamPort}`).catch(() => {})
  await upstream.close()
}

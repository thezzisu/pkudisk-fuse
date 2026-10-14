// All surfaces use the Go protocol fixture. This adapter only owns its process.
import { spawn, execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createInterface } from 'node:readline'

const run = promisify(execFile)
const root = fileURLToPath(new URL('../../', import.meta.url))
export async function startAnyShareFixture({ profile = 'desktop' } = {}) {
  const directory = await mkdtemp(join(tmpdir(), 'pkudisk-mockserver-'))
  const binary = join(directory, process.platform === 'win32' ? 'mockserver.exe' : 'mockserver')
  try {
    await run('go', ['build', '-o', binary, './internal/anyshare/mockserver/cmd/mockserver'], { cwd: root })
  } catch (error) {
    await rm(directory, { recursive: true, force: true })
    throw error
  }
  const child = spawn(binary, ['--profile', profile], { cwd: root, stdio: ['pipe', 'pipe', 'pipe'] })
  const pending = []
  const lines = []
  let failure
  let stderr = ''
  child.stderr.on('data', data => { stderr += data })
  const reader = createInterface({ input: child.stdout })
  reader.on('line', line => {
    try {
      const value = JSON.parse(line)
      if (pending.length) pending.shift().resolve(value)
      else lines.push(value)
    } catch (error) { fail(error) }
  })
  function fail(error) {
    failure = error
    for (const request of pending.splice(0)) request.reject(error)
  }
  const exited = new Promise(resolve => {
    child.on('error', error => { fail(error); resolve() })
    child.on('exit', (code, signal) => { fail(new Error(`AnyShare fixture exited (${code ?? signal}): ${stderr}`)); resolve() })
  })
  function next() {
    if (lines.length) return Promise.resolve(lines.shift())
    if (failure) return Promise.reject(failure)
    return new Promise((resolve, reject) => pending.push({ resolve, reject }))
  }
  async function close() {
    child.stdin.end()
    await exited
    reader.close()
    await rm(directory, { recursive: true, force: true })
  }
  try {
    const ready = await next()
    return {
      ...ready,
      async snapshot() {
        const response = next()
        child.stdin.write(`${JSON.stringify({ action: 'snapshot' })}\n`)
        const state = await response
        return { ...state, entries: new Map(state.entries.map(entry => [entry.id, entry])), data: new Map(Object.entries(state.data).map(([id, bytes]) => [id, Buffer.from(bytes || '', 'base64')])) }
      },
      close
    }
  } catch (error) { await close(); throw error }
}

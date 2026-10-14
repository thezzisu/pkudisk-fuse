import { createHash } from 'node:crypto'
import { readdir, readFile } from 'node:fs/promises'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('../', import.meta.url))

export async function sourceHash() {
  const files = []
  async function walk(directory) {
    for (const entry of await readdir(join(root, directory), { withFileTypes: true })) {
      const path = join(directory, entry.name)
      if (entry.isDirectory()) await walk(path)
      else if (entry.isFile()) files.push(path)
    }
  }
  for (const directory of ['app', 'i18n', 'public', 'scripts', 'licenses', 'patches']) await walk(directory)
  files.push('package.json', 'package-lock.json', 'nuxt.config.ts', 'tsconfig.json')
  const hash = createHash('sha256')
  for (const file of files.sort()) {
    hash.update(relative(root, join(root, file)).split(sep).join('/'))
    hash.update('\0')
    hash.update(await readFile(join(root, file)))
    hash.update('\0')
  }
  return hash.digest('hex') + '\n'
}

if (process.argv.includes('--check')) {
  const expected = await readFile(new URL('../../internal/gui/web/source.sha256', import.meta.url), 'utf8')
  if (expected !== await sourceHash()) throw new Error('Embedded GUI assets are stale. Run make ui and include the generated assets.')
  console.log('Embedded GUI source fingerprint matches.')
}

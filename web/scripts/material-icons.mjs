import { mkdir, readFile, readdir, unlink, writeFile } from 'node:fs/promises'
import { basename, dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { generateManifest } from 'material-icon-theme'

const root = fileURLToPath(new URL('../', import.meta.url))
const packageRoot = dirname(fileURLToPath(import.meta.resolve('material-icon-theme/package.json')))

async function writeChanged(path, content) {
  const previous = await readFile(path).catch(error => {
    if (error.code !== 'ENOENT') throw error
    return null
  })
  const data = Buffer.from(content)
  if (!previous?.equals(data)) await writeFile(path, data)
}

// The upstream library is a Node build dependency only. Its VS Code manifest
// supplies every association; the client receives plain JSON and local SVGs.
export async function generateMaterialIcons() {
  const manifest = generateManifest()
  const directory = join(root, 'public/material-icons')
  const generated = join(root, 'app/generated')
  await Promise.all([mkdir(directory, { recursive: true }), mkdir(generated, { recursive: true })])
  const expected = new Set(['LICENSE.txt'])
  for (const definition of Object.values(manifest.iconDefinitions || {})) {
    const source = resolve(packageRoot, 'dist', definition.iconPath)
    const name = basename(source)
    if (!name.endsWith('.svg') || !source.startsWith(join(packageRoot, 'icons/'))) {
      throw new Error(`Unexpected Material Icon Theme asset: ${definition.iconPath}`)
    }
    expected.add(name)
    await writeChanged(join(directory, name), await readFile(source))
    definition.iconPath = `/material-icons/${name}`
  }
  for (const name of await readdir(directory)) {
    if (!expected.has(name)) await unlink(join(directory, name))
  }
  await writeChanged(join(directory, 'LICENSE.txt'), await readFile(join(packageRoot, 'LICENSE')))
  await writeChanged(join(generated, 'material-icons.json'), JSON.stringify(manifest) + '\n')
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  await generateMaterialIcons()
}

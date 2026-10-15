import assert from 'node:assert/strict'
import { access, readFile } from 'node:fs/promises'
import { generateMaterialIcons } from './material-icons.mjs'

await generateMaterialIcons()
const { materialFileIcon } = await import('../app/utils/material-icons.ts')
const icon = (name, options) => materialFileIcon(name, options).replace('/material-icons/', '')

assert.equal(icon('report.PDF'), 'pdf.svg')
assert.equal(icon('report.docx'), 'word.svg')
assert.equal(icon('photo.png'), 'image.svg')
assert.equal(icon('photo.JPG'), 'image.svg')
assert.equal(icon('backup.zip'), 'zip.svg')
assert.equal(icon('notes.txt'), 'document.svg')
assert.equal(icon('script.js'), 'javascript.svg')
assert.equal(icon('PACKAGE.JSON'), 'nodejs.svg', 'exact names outrank extensions')
assert.equal(icon('README.md'), 'readme.svg')
assert.equal(icon('client.d.ts'), 'typescript-def.svg', 'longest extension wins')
assert.equal(icon('graphqlrc', { path: 'repo/.config/graphqlrc' }), 'graphql.svg', 'parent-name association')
assert.equal(icon('src', { directory: true }), 'folder-src.svg')
assert.equal(icon('SRC', { directory: true, expanded: true }), 'folder-src-open.svg')
assert.equal(icon('unknown-folder', { directory: true }), 'folder.svg')
assert.equal(icon('unknown-file.unknown'), 'file.svg')
assert.equal(icon('__proto__'), 'file.svg', 'arbitrary filenames cannot select object prototypes')
assert.equal(icon('constructor'), 'file.svg')
assert.equal(icon('bun.lock', { light: false }), 'bun.svg')
assert.equal(icon('bun.lock', { light: true }), 'bun_light.svg')

const manifest = JSON.parse(await readFile(new URL('../app/generated/material-icons.json', import.meta.url), 'utf8'))
for (const definition of Object.values(manifest.iconDefinitions)) {
  assert.match(definition.iconPath, /^\/material-icons\/[^/]+\.svg$/)
  await access(new URL(`../public${definition.iconPath}`, import.meta.url))
}
const initial = await readFile(new URL('../app/generated/material-icons.json', import.meta.url))
await generateMaterialIcons()
assert.deepEqual(await readFile(new URL('../app/generated/material-icons.json', import.meta.url)), initial)
console.log(`Material Icon Theme: association precedence, case, light/dark, folders, ${Object.keys(manifest.iconDefinitions).length} local SVGs and deterministic generation passed.`)

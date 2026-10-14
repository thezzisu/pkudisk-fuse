import { access, cp, mkdir, rm, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { sourceHash } from './source-hash.mjs'

const root = fileURLToPath(new URL('../', import.meta.url))
const target = fileURLToPath(new URL('../../internal/gui/web/', import.meta.url))
await access(`${root}.output/public/index.html`)
await access(`${root}.nuxt/THIRD_PARTY_LICENSES.txt`)
await rm(target, { recursive: true, force: true })
await mkdir(target, { recursive: true })
await cp(`${root}.output/public`, target, { recursive: true })
await cp(`${root}.nuxt/THIRD_PARTY_LICENSES.txt`, `${target}/THIRD_PARTY_LICENSES.txt`)
await cp(`${root}licenses/nuxt-ui-starter.txt`, `${target}/NUXT_UI_STARTER_LICENSE.txt`)
await cp(`${root}licenses/lucide.txt`, `${target}/LUCIDE_LICENSE.txt`)
await writeFile(`${target}/source.sha256`, await sourceHash())
console.log('Embedded static GUI assets updated.')

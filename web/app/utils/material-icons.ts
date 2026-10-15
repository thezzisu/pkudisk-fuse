import type { Manifest } from 'material-icon-theme'
import generated from '../generated/material-icons.json' with { type: 'json' }

const manifest: Manifest = generated

type Associations =
  | 'fileNames'
  | 'fileExtensions'
  | 'folderNames'
  | 'folderNamesExpanded'
  | 'rootFolderNames'
  | 'rootFolderNamesExpanded'
  | 'languageIds'

type Defaults = 'file' | 'folder' | 'folderExpanded' | 'rootFolder' | 'rootFolderExpanded'

export interface MaterialFileIconOptions {
  directory?: boolean
  expanded?: boolean
  root?: boolean
  light?: boolean
  /** Full display path, when available, enables upstream parent-folder rules. */
  path?: string
  language?: string
}

/** Resolve the official VS Code manifest; no application-specific extension map. */
export function materialFileIcon(name: string, options: MaterialFileIconOptions = {}) {
  const variant = options.light ? manifest.light : undefined
  const segments = (options.path || name).replaceAll('\\', '/').toLowerCase().split('/')
  const filename = segments.at(-1) || name.toLowerCase()
  const parent = segments.at(-2)
  const association = (kind: Associations, value: string) => {
    const overrides = variant?.[kind]
    const defaults = manifest[kind]
    if (overrides && Object.hasOwn(overrides, value)) return overrides[value]
    return defaults && Object.hasOwn(defaults, value) ? defaults[value] : undefined
  }
  const fallback = (kind: Defaults) => variant?.[kind] || manifest[kind]
  const named = (kind: Associations) =>
    (parent && association(kind, `${parent}/${filename}`)) || association(kind, filename)

  let icon: string | undefined
  if (options.directory) {
    if (options.root) {
      icon =
        association(options.expanded ? 'rootFolderNamesExpanded' : 'rootFolderNames', filename) ||
        (options.expanded && fallback('rootFolderExpanded')) ||
        fallback('rootFolder')
    } else {
      icon = named(options.expanded ? 'folderNamesExpanded' : 'folderNames')
    }
    icon ||= (options.expanded && fallback('folderExpanded')) || fallback('folder')
  } else {
    icon = named('fileNames')
    // Parent-specific associations outrank plain extensions. Within each group,
    // the first suffix is longest: lib.d.ts resolves d.ts before ts.
    const pieces = filename.split('.')
    const extensions = pieces.slice(1).map((_, index) => pieces.slice(index + 1).join('.'))
    if (!icon && parent) {
      icon = extensions
        .map((extension) => association('fileExtensions', `${parent}/${extension}`))
        .find(Boolean)
    }
    icon ||= extensions.map((extension) => association('fileExtensions', extension)).find(Boolean)
    if (!icon && options.language) icon = association('languageIds', options.language)
    icon ||= fallback('file')
  }
  return manifest.iconDefinitions?.[icon || 'file']?.iconPath || '/material-icons/file.svg'
}

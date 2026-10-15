export interface ApiError {
  code: string
  message: string
}
export interface Account {
  name: string
  username: string
  server: string
  default: boolean
  connected: boolean
}
export interface HostFlags {
  isMobile: boolean
  canMount: boolean
  canBrowseLocalFS: boolean
}
export interface LocalEntry {
  name: string
  path: string
  dir: boolean
  size: number
  modified: string
  empty: boolean
}
export interface LocalRoot {
  kind: string
  name: string
  path: string
  available: boolean
  free?: boolean
}
export interface LocalListing {
  path: string
  parent: string
  empty: boolean
  roots: LocalRoot[]
  entries: LocalEntry[]
}
export interface AccountsResponse {
  flags: HostFlags
  accounts: Account[]
  default: string
  cache: string
}
export interface Entry {
  thumbnail?: boolean
  id: string
  name: string
  directory: boolean
  size: number
  rev: string
  modified_at: string
  created_at: string
  type?: string
  path?: string
}
export interface Listing {
  entries: Entry[]
  folder: Entry | null
  writable: boolean
  account: string
  stats?: Record<string, { quota?: { allocated: number; used: number }; writable?: boolean }>
}
export interface SearchResponse {
  entries: Entry[]
  hits: number
  next: number
}
export interface Job {
  id: string
  clientId?: string
  browser?: boolean
  kind: string
  account: string
  allAccounts: boolean
  name: string
  source: string
  destination: string
  state: string
  phase: string
  done: number
  total: number
  percent: number
  created: string
  error?: ApiError
  result?: unknown
}
export interface LoginAttempt {
  id: string
  account: string
  state: string
  challenge?: { id: string; kind: string; image?: string }
  error?: ApiError
}
export interface Revision {
  rev: string
  name: string
  editor: string
  size: number
  modified_at: string
}
export interface ShareLink {
  id: string
  type: string
  url: string
  title: string
  permissions: string[]
  expires_at: string | null
  code?: string
  limited_times: number
  verify_mobile: boolean
}
export interface Recipient {
  id: string
  type: 'user' | 'department'
  name?: string
  account?: string
  path?: string
}
export interface Permission {
  id: string
  type: string
  name: string
  allow: string[]
  deny: string[]
  expires_at: string | null
  inherited_from?: string
  inherited_path?: string
}
export interface SharingResponse {
  links: ShareLink[]
  policy: {
    validity_period: { default: number; maximum: number }
    perm: { allow: string[]; default: string[] }
    access_times: { default: number; maximum: number }
    must_set_password: boolean
    verify_mobile: { enable: boolean; force: boolean }
  }
  policyError?: ApiError
  acl?: { inherit: boolean; permissions: Permission[] }
}
export interface RecoveryRecord {
  account?: string
  server?: string
  entry: Entry
  parent: string
  data: string
  updated: string
}
export interface MountSupport {
  available: boolean
  message: string
  platform: string
}
export interface SessionState {
  flags: HostFlags
  hostKnown: boolean
  phase: 'connecting' | 'ready' | 'expired' | 'stopped'
  connection: 'checking' | 'online' | 'offline'
  lastSeen: number | null
  version: string
  tray: boolean
}

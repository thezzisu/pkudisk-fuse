import { LRUCache } from 'lru-cache'
import pLimit from 'p-limit'
import type { Entry, SessionState } from '~/types/api'

export const useThumbnailsStore = defineStore('thumbnails', () => {
  const { $api } = useNuxtApp()
  const session = useState<SessionState>('local-session')
  const limit = pLimit(4)
  const cache = new LRUCache<string, Blob>({
    max: 100,
    maxSize: 16 * 1024 * 1024,
    sizeCalculation: (blob) => Math.max(1, blob.size),
    ttl: 120000
  })
  watch(
    () => session.value.phase,
    (phase) => {
      if (phase !== 'ready') cache.clear()
    }
  )
  async function get(account: string, entry: Entry, signal: AbortSignal) {
    const key = JSON.stringify([account, entry.id, entry.rev, entry.modified_at])
    return limit(async () => {
      signal.throwIfAborted()
      if (session.value.phase !== 'ready') return null
      const existing = cache.get(key)
      if (existing) return existing
      try {
        const blob = await $api<Blob>('/thumbnail', {
          query: { account, path: entry.id },
          responseType: 'blob',
          signal,
          timeout: 16000,
          // A newly requested thumbnail can still be rendering upstream.
          // Honor the endpoint's bounded 503/Retry-After generation handoff.
          retry: 2,
          retryDelay: 5000,
          retryStatusCodes: [503]
        })
        if (!['image/jpeg', 'image/png', 'image/gif', 'image/webp'].includes(blob.type)) return null
        if (session.value.phase === 'ready') cache.set(key, blob)
        return blob
      } catch {
        // Missing/pending previews do not block file browsing or show toasts.
        return null
      }
    })
  }
  return { get }
})

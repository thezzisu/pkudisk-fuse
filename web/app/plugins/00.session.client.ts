import type { HostFlags, SessionState } from '~/types/api'
import type { FetchError } from 'ofetch'

export default defineNuxtPlugin({
  name: 'local-session',
  // Run after payload revival (-30) but before the router's pre plugins (-20),
  // so Vue Router cannot capture and later restore the activation fragment.
  order: -25,
  async setup() {
    const state = useState<SessionState>('local-session', () => ({
      phase: 'connecting',
      connection: 'checking',
      lastSeen: null,
      version: '',
      tray: false,
      hostKnown: false,
      flags: { isMobile: false, canMount: false, canBrowseLocalFS: false }
    }))
    // Retain only explicit host metadata for an expired session after a reload.
    // It controls presentation; authorization remains entirely server-side.
    try {
      const flags = JSON.parse(sessionStorage.getItem('pkudisk-gui-flags') || 'null')
      if (
        flags &&
        ['isMobile', 'canMount', 'canBrowseLocalFS'].every((key) => typeof flags[key] === 'boolean')
      ) {
        state.value.flags = flags
        state.value.hostKnown = true
      }
    } catch {
      /* Missing or unavailable storage leaves the initial surface neutral. */
    }
    const ticket = new URLSearchParams(location.hash.slice(1)).get('ticket')
    if (ticket) history.replaceState(null, '', location.pathname + location.search)
    let token: string | null = null
    let checking: Promise<void> | null = null
    let healthController: AbortController | null = null
    useEventListener(document, 'visibilitychange', () => {
      // An idle mobile host deliberately suspends its local service while
      // hidden. A request from that old page must not latch a false failure.
      if (state.value.flags.isMobile && document.hidden) healthController?.abort()
    })
    function clearToken() {
      token = null
      try {
        sessionStorage.removeItem('pkudisk-gui-session')
      } catch {
        /* Storage can be disabled. */
      }
    }
    function endSession() {
      clearToken()
      state.value = { ...state.value, phase: 'stopped', connection: 'offline' }
    }
    const api = $fetch.create({
      baseURL: '/api',
      credentials: 'omit',
      retry: 0,
      timeout: 125000,
      onRequest({ options }) {
        if (state.value.phase === 'stopped') {
          throw createError({ data: { error: { code: 'service_unavailable', message: '' } } })
        }
        if (token) options.headers.set('Authorization', `Bearer ${token}`)
      },
      async onRequestError({ options }) {
        // Confirm service loss before an operation reports a transport error.
        // Navigation and user cancellation must not end a healthy session.
        if (state.value.phase === 'ready' && !options.signal?.aborted) await checkConnection()
      },
      onResponseError({ response }) {
        if (response.status === 401 && state.value.phase !== 'stopped') {
          clearToken()
          state.value.phase = 'expired'
        }
      }
    })
    function checkConnection(): Promise<void> {
      if (state.value.phase !== 'ready' || (state.value.flags.isMobile && document.hidden))
        return Promise.resolve()
      if (checking) return checking
      const controller = new AbortController()
      healthController = controller
      // Use a separate fetch so failed health checks never call their own hooks.
      checking ??= $fetch<{ version: string; tray: boolean; flags: HostFlags }>('/api/status', {
        credentials: 'omit',
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        retry: 0,
        timeout: 2500,
        signal: controller.signal
      })
        .then((result) => {
          if (state.value.phase !== 'ready' || controller.signal.aborted) return
          state.value = { ...state.value, ...result, connection: 'online', lastSeen: Date.now() }
        })
        .catch((error: FetchError) => {
          if (state.value.phase !== 'ready' || controller.signal.aborted) return
          if (error.response?.status === 401) {
            clearToken()
            state.value.phase = 'expired'
          } else endSession()
        })
        .finally(() => {
          checking = null
          healthController = null
        })
      return checking
    }
    try {
      token = sessionStorage.getItem('pkudisk-gui-session')
      if (ticket) {
        const result = await $fetch<{ token: string }>('/api/session', {
          method: 'POST',
          credentials: 'omit',
          retry: 0,
          body: { ticket }
        })
        token = result.token
        sessionStorage.setItem('pkudisk-gui-session', token)
      }
      if (!token) throw new Error('missing local session')
      const result = await api<{ version: string; tray: boolean; flags: HostFlags }>('/status')
      state.value = {
        ...result,
        hostKnown: true,
        phase: 'ready',
        connection: 'online',
        lastSeen: Date.now()
      }
      try {
        sessionStorage.setItem('pkudisk-gui-flags', JSON.stringify(result.flags))
      } catch {
        /* Storage can be disabled. */
      }
    } catch (error) {
      const failure = error as FetchError
      if (failure.request && !failure.response) endSession()
      else {
        clearToken()
        state.value.phase = 'expired'
      }
    }
    return { provide: { api, checkConnection, endSession } }
  }
})

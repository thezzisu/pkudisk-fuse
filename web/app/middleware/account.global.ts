import type { SessionState } from '~/types/api'
import type { LocationQueryRaw } from 'vue-router'

// Resolve the account before a page starts loading its account-scoped data.
// This also makes a deep link authoritative after a fresh mobile WebView starts.
export default defineNuxtRouteMiddleware(async (to) => {
  if (useState<SessionState>('local-session').value?.phase !== 'ready') return
  const workspace = useWorkspaceStore()
  try {
    await workspace.initialize()
  } catch {
    // Account loading errors are rendered by the workspace, not a router error page.
    return
  }
  const account = typeof to.query.account === 'string' ? to.query.account : ''
  if (workspace.accounts.some((saved) => saved.name === account)) {
    workspace.selected = account
    return
  }
  if (!workspace.selected) return
  const query: LocationQueryRaw = { ...to.query, account: workspace.selected }
  if (account) {
    // A removed account must not lend its remote IDs to the fallback account.
    query.path = query.q = query.scope = query.limit = query.library = undefined
  }
  // Preserve the original navigation's history mode. Forcing replace here
  // would erase the current folder when a sidebar link omits its account.
  return navigateTo({ path: to.path, query, hash: to.hash })
})

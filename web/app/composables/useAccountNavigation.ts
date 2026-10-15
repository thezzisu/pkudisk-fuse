import type { LocationQueryRaw } from 'vue-router'

export function useAccountNavigation() {
  const route = useRoute()
  const router = useRouter()
  const workspace = useWorkspaceStore()

  return async (account: string) => {
    if (account === workspace.selected && route.query.account === account) return
    const query: LocationQueryRaw = { ...route.query, account: account || undefined }
    query.path = query.q = query.scope = query.limit = query.library = undefined
    await router.push({ path: route.path, query })
  }
}

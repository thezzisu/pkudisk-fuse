export default defineNuxtRouteMiddleware(async (to) => {
  const workspace = useWorkspaceStore()
  await workspace.initialize()
  const path = to.path.replace(/\/+$/, '')
  if (path === '/mounts' && (workspace.flags.isMobile || !workspace.flags.canMount))
    return navigateTo('/')
  if (path === '/recovery' && (workspace.flags.isMobile || !workspace.flags.canBrowseLocalFS))
    return navigateTo('/')
})

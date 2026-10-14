import { defineStore } from 'pinia'
import type { Account, AccountsResponse, Job } from '~/types/api'

export const useWorkspaceStore = defineStore('workspace', () => {
  const { $api } = useNuxtApp()
  const accounts = ref<Account[]>([])
  const selected = ref('')
  const cache = ref('')
  const flags = ref({ isMobile: false, canMount: false, canBrowseLocalFS: false })
  const remoteTasks = ref<Job[]>([])
  const browser = useBrowserTransfersStore()
  const tasks = computed(() => {
    const merged = new Map<string, Job>()
    const key = (job: Job) => `${job.account}:${job.kind}:${job.clientId || job.id}`
    for (const job of browser.jobs) merged.set(key(job), job)
    // A retried client attempt can have several server jobs. Only the latest
    // server record is authoritative once the service has received the upload.
    for (const job of [...remoteTasks.value].sort((a, b) => a.created.localeCompare(b.created)))
      merged.set(key(job), job)
    return [...merged.values()].sort((a, b) => b.created.localeCompare(a.created))
  })
  const ready = ref(false)
  const loginOpen = ref(false)
  const loginAccount = ref<Account | null>(null)
  const restoreLogin = ref(false)
  let loading: Promise<void> | null = null
  let tasksLoading: Promise<void> | null = null

  const current = computed(() => accounts.value.find((account) => account.name === selected.value))
  const activeTasks = computed(() =>
    tasks.value.filter((task) => ['queued', 'running', 'mounted', 'stopping'].includes(task.state))
  )
  async function refreshAccounts() {
    const response = await $api<AccountsResponse>('/accounts')
    accounts.value = response.accounts
    cache.value = response.cache
    flags.value = response.flags
    if (!accounts.value.some((account) => account.name === selected.value))
      selected.value = response.default
    ready.value = true
  }
  async function initialize() {
    if (ready.value) return
    loading ??= refreshAccounts().finally(() => {
      loading = null
    })
    await loading
  }
  async function refreshTasks() {
    tasksLoading ??= $api<Job[]>('/tasks', { timeout: 5000 })
      .then((response) => {
        remoteTasks.value = response
      })
      .finally(() => {
        tasksLoading = null
      })
    await tasksLoading
  }
  function login(account: Account | null = null, restore = false) {
    loginAccount.value = account
    restoreLogin.value = restore
    loginOpen.value = true
  }
  return {
    accounts,
    selected,
    cache,
    flags,
    tasks,
    ready,
    current,
    activeTasks,
    loginOpen,
    loginAccount,
    restoreLogin,
    initialize,
    refreshAccounts,
    refreshTasks,
    login
  }
})

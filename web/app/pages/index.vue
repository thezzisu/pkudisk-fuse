<script setup lang="ts">
import type { Entry, Listing, SearchResponse } from '~/types/api'

const { t } = useI18n()
const { $api } = useNuxtApp()
const route = useRoute()
const router = useRouter()
const workspace = useWorkspaceStore()
const table = useTemplateRef('filesTable')
const pagination = ref({ pageIndex: 0, pageSize: 50 })
const filter = ref('')
const draftSearch = ref(String(route.query.q || ''))
const searchScope = computed(() => route.query.scope === 'current')
const searchLimit = computed(() => {
  const value = Number(route.query.limit)
  return Number.isInteger(value) && value >= 1 && value <= 1000 ? value : 100
})
const scopeSearch = ref(searchScope.value)
const limit = ref(searchLimit.value)
const path = computed(() => String(route.query.path || '/'))
const query = computed(() => String(route.query.q || ''))
const trail = ref<{ label: string; path: string }[]>([])

const viewKey = computed(() =>
  JSON.stringify([
    workspace.selected,
    path.value,
    query.value,
    searchScope.value,
    searchLimit.value
  ])
)
type FileView = Listing & { hits?: number; requestKey: string }
const empty: FileView = { entries: [], folder: null, writable: false, account: '', requestKey: '' }
const { data, status, error, refresh } = useLazyAsyncData<FileView>(
  'file-list',
  async (_app, { signal }) => {
    await workspace.initialize()
    if (!workspace.selected) return empty
    const account = workspace.selected
    const requestKey = viewKey.value
    if (query.value) {
      const result = await $api<SearchResponse>('/search', {
        query: {
          account,
          q: query.value,
          scope: searchScope.value && path.value !== '/' ? path.value : '',
          limit: searchLimit.value
        },
        signal
      })
      return { ...empty, entries: result.entries || [], hits: result.hits, account, requestKey }
    }
    const listing = await $api<Listing>('/files', { query: { account, path: path.value }, signal })
    return { ...listing, requestKey }
  },
  { default: () => empty, watch: [viewKey] }
)
const interactionReady = computed(
  () => data.value.requestKey === viewKey.value && status.value !== 'error'
)
const tableData = computed(() => (interactionReady.value ? data.value.entries : empty.entries))
const { transfer, change, menu } = useFileActions({
  account: () => workspace.selected,
  folder: () => data.value.folder,
  ready: interactionReady,
  refresh
})

const breadcrumbs = computed(() => [
  { label: t('files.libraries'), icon: 'i-lucide-cloud', onClick: () => navigate('/') },
  ...trail.value
    .filter((item) => path.value === item.path || path.value.startsWith(`${item.path}/`))
    .map((item) => ({ label: item.label, onClick: () => navigate(item.path) })),
  ...(data.value.folder && !trail.value.some((item) => item.path === data.value.folder?.id)
    ? [{ label: data.value.folder.name }]
    : [])
])

async function navigate(target: string, name?: string) {
  filter.value = ''
  draftSearch.value = ''
  pagination.value.pageIndex = 0
  if (target === '/') trail.value = []
  else {
    const existing = trail.value.findIndex((item) => item.path === target)
    if (existing >= 0) trail.value = trail.value.slice(0, existing + 1)
    else if (name) trail.value.push({ label: name, path: target })
  }
  await router.push({
    path: '/',
    query: { account: workspace.selected, path: target === '/' ? undefined : target }
  })
}
async function open(entry: Entry) {
  if (!interactionReady.value) return
  if (entry.directory) await navigate(entry.id, entry.name)
  else
    await transfer.open({
      kind: 'download',
      account: workspace.selected,
      entry,
      folder: data.value.folder
    }).result
}
async function search() {
  pagination.value.pageIndex = 0
  const nextQuery = draftSearch.value.trim()
  const nextScope = scopeSearch.value && path.value !== '/'
  const unchanged =
    query.value === nextQuery &&
    searchScope.value === nextScope &&
    searchLimit.value === limit.value
  await router.push({
    query: {
      ...route.query,
      account: workspace.selected,
      q: nextQuery || undefined,
      scope: nextScope ? 'current' : undefined,
      limit: limit.value === 100 ? undefined : limit.value
    }
  })
  if (unchanged) await refresh()
}
function up() {
  const id = data.value.folder?.id || path.value
  const cut = id.lastIndexOf('/')
  return navigate(cut > 'gns://'.length ? id.slice(0, cut) : '/')
}
watch(
  () => workspace.selected,
  () => {
    trail.value = []
  }
)
watch([path, query, searchScope, searchLimit], () => {
  draftSearch.value = query.value
  scopeSearch.value = searchScope.value
  limit.value = searchLimit.value
  filter.value = ''
  pagination.value.pageIndex = 0
})
watch(
  () => data.value.folder,
  (folder) => {
    if (folder && !trail.value.some((item) => item.path === folder.id)) {
      trail.value.push({ label: folder.name, path: folder.id })
      trail.value.sort((a, b) => a.path.length - b.path.length)
    }
  }
)
defineShortcuts({
  meta_k: () => document.querySelector<HTMLInputElement>('[data-search-input]')?.focus()
})
watch(
  () => workspace.tasks,
  (tasks, previous) => {
    if (
      tasks.some(
        (task) =>
          task.account === workspace.selected &&
          task.state === 'completed' &&
          ['upload', 'copy', 'extract', 'recycle_restore'].includes(task.kind) &&
          previous.find(
            (old) => old.id === task.id || (task.clientId && old.clientId === task.clientId)
          )?.state !== 'completed'
      )
    )
      void refresh()
  }
)

async function clearFilters() {
  filter.value = ''
  draftSearch.value = ''
  await search()
}
</script>

<template>
  <WorkspacePanel :title="query ? t('files.searchTitle') : t('files.title')" flush>
    <template v-if="workspace.accounts.length" #toolbar>
      <FileToolbar
        v-model:search="draftSearch"
        v-model:scope="scopeSearch"
        v-model:limit="limit"
        :can-write="interactionReady && !!data.folder && data.writable && !query"
        :loading="status === 'pending'"
        :at-root="path === '/'"
        @upload="
          transfer.open({ kind: 'upload', account: workspace.selected, folder: data.folder })
        "
        @mkdir="data.folder && change('mkdir', data.folder)"
        @refresh="refresh()"
        @search="search"
      />
    </template>
    <OperationError v-if="error" :error="error" class="m-4 sm:mx-6" />
    <AccountEmptyState v-if="workspace.ready && !workspace.accounts.length" class="flex-1" />
    <template v-else>
      <div
        class="flex w-full shrink-0 flex-wrap items-center justify-between gap-2 px-4 py-2 sm:px-6"
      >
        <div class="flex w-full min-w-0 flex-1 items-center gap-2 sm:w-auto">
          <UButton
            v-if="data.folder"
            icon="i-lucide-arrow-up"
            :aria-label="t('files.up')"
            color="neutral"
            variant="ghost"
            size="sm"
            @click="up"
          /><UBreadcrumb
            :items="breadcrumbs"
            class="min-w-0 flex-1"
            :ui="{
              list: 'min-w-0',
              item: 'min-w-0',
              link: 'min-w-0',
              linkLabel: 'max-w-32 truncate sm:max-w-none'
            }"
          />
        </div>
        <div class="flex w-full min-w-0 items-center gap-2 sm:w-auto">
          <UInput
            v-model="filter"
            icon="i-lucide-list-filter"
            :placeholder="t('files.filterPlaceholder')"
            :aria-label="t('files.filterPlaceholder')"
            size="sm"
            class="min-w-0 flex-1"
          />
          <FileSortControl />
        </div>
      </div>
      <UAlert
        v-if="query && data.hits && data.hits > data.entries.length"
        color="info"
        variant="subtle"
        :description="t('files.searchTruncated', { count: data.entries.length, total: data.hits })"
        class="mx-4 mb-3 shrink-0 sm:mx-6"
      />
      <UAlert
        v-if="data.folder && !data.writable"
        icon="i-lucide-lock-keyhole"
        color="neutral"
        variant="subtle"
        :description="t('files.readOnlyHint')"
        class="mx-4 mb-3 shrink-0 sm:mx-6"
      />
      <FileTable
        ref="filesTable"
        v-model:filter="filter"
        v-model:pagination="pagination"
        :account="data.account"
        :failed="Boolean(error)"
        :entries="tableData"
        :stats="data.stats"
        :folder="data.folder"
        :query="query"
        :loading="status === 'pending' || status === 'idle'"
        :menu="menu"
        @open="open"
        @clear="clearFilters"
      />
    </template>
    <template v-if="workspace.accounts.length" #footer>
      <p class="text-muted text-xs">
        {{ t('common.items', { count: table?.filteredCount || 0 }) }}
      </p>
      <UPagination
        v-if="(table?.filteredCount || 0) > pagination.pageSize"
        :page="pagination.pageIndex + 1"
        :items-per-page="pagination.pageSize"
        :total="table?.filteredCount || 0"
        @update:page="pagination.pageIndex = $event - 1"
      />
    </template>
  </WorkspacePanel>
</template>

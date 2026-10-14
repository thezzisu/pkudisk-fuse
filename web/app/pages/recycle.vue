<script setup lang="ts">
import { getPaginationRowModel, type Table } from '@tanstack/vue-table'
import type { TableColumn } from '@nuxt/ui'
import type { Entry } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'

const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const format = useFormatting()
const feedback = useFeedback()
const confirm = useOverlay().create(ConfirmModal)
const checkbox = resolveComponent('UCheckbox')
const table = useTemplateRef<{ tableApi: Table<Entry> }>('recycleTable')
const library = ref('')
const rename = ref(false)
const selection = ref<Record<string, boolean>>({})
const filter = ref('')
const pagination = ref({ pageIndex: 0, pageSize: 50 })
const busy = ref(false)
const {
  data: roots,
  status: rootStatus,
  error: rootError,
  refresh: refreshRoots
} = useLazyAsyncData(
  'recycle-libraries',
  async () => {
    await workspace.initialize()
    if (!workspace.selected) return { libraries: [], account: '' }
    return $api<{ libraries: Entry[]; account: string }>('/recycle', {
      query: { account: workspace.selected }
    })
  },
  { default: () => ({ libraries: [] as Entry[], account: '' }), watch: [() => workspace.selected] }
)
watch(roots, (value) => {
  if (!value.libraries.some((entry) => entry.id === library.value))
    library.value = value.libraries[0]?.id || ''
})
const { data, status, error, refresh } = useLazyAsyncData(
  'recycle-items',
  () =>
    library.value
      ? $api<{ entries: Entry[]; account: string; library: string }>('/recycle', {
          query: { account: workspace.selected, library: library.value }
        })
      : Promise.resolve({ entries: [], account: '', library: '' }),
  {
    default: () => ({ entries: [] as Entry[], account: '', library: '' }),
    watch: [library, () => workspace.selected]
  }
)
const interactionReady = computed(
  () =>
    rootStatus.value === 'success' &&
    status.value === 'success' &&
    roots.value.account === workspace.selected &&
    data.value.account === workspace.selected &&
    data.value.library === library.value
)
const loading = computed(
  () => ['pending', 'idle'].includes(rootStatus.value) || ['pending', 'idle'].includes(status.value)
)
async function refreshAll() {
  await refreshRoots()
  if (!rootError.value) await refresh()
}
const filteredCount = computed(() => table.value?.tableApi.getFilteredRowModel().rows.length || 0)
const emptyRows: Entry[] = []
const tableData = computed(() => (interactionReady.value ? data.value.entries : emptyRows))
watch(data, () => {
  selection.value = {}
  pagination.value.pageIndex = 0
})
const columns = computed<TableColumn<Entry>[]>(() => [
  {
    id: 'select',
    header: ({ table }) =>
      h(checkbox, {
        modelValue: table.getIsSomePageRowsSelected()
          ? 'indeterminate'
          : table.getIsAllPageRowsSelected(),
        'onUpdate:modelValue': (value: boolean | 'indeterminate') =>
          table.toggleAllPageRowsSelected(Boolean(value)),
        'aria-label': t('common.selectAll'),
        disabled: !interactionReady.value || !tableData.value.length
      }),
    cell: ({ row }) =>
      h(checkbox, {
        modelValue: row.getIsSelected(),
        'onUpdate:modelValue': (value: boolean | 'indeterminate') =>
          row.toggleSelected(Boolean(value)),
        'aria-label': t('common.selectRow')
      })
  },
  { accessorKey: 'name', header: t('common.name') },
  {
    accessorKey: 'path',
    header: t('recycle.originalPath'),
    meta: { class: { th: 'hidden lg:table-cell', td: 'hidden lg:table-cell' } }
  },
  {
    accessorKey: 'size',
    header: t('common.size'),
    meta: { class: { th: 'text-end', td: 'text-end' } }
  },
  {
    accessorKey: 'modified_at',
    header: t('recycle.deleted'),
    meta: { class: { th: 'hidden sm:table-cell', td: 'hidden sm:table-cell' } }
  }
])
const selected = computed<Entry[]>(
  () => table.value?.tableApi.getFilteredSelectedRowModel().rows.map((row) => row.original) || []
)

async function action(kind: 'restore' | 'remove', all = false) {
  if (!interactionReady.value) return
  const account = data.value.account
  const rows = all
    ? table.value?.tableApi.getFilteredRowModel().rows.map((row) => row.original) || []
    : selected.value
  const ids = rows.map((row) => row.id)
  if (!ids.length) return
  if (
    !(await confirm.open({
      title: t(kind === 'restore' ? 'recycle.confirmRestore' : 'recycle.confirmRemove', {
        count: ids.length
      }),
      description: t(kind === 'restore' ? 'recycle.restoreHint' : 'recycle.removeHint'),
      destructive: kind === 'remove'
    }).result)
  )
    return
  busy.value = true
  try {
    for (let offset = 0; offset < ids.length; offset += 1000) {
      await $api('/recycle/action', {
        method: 'POST',
        body: {
          account,
          action: kind,
          ids: ids.slice(offset, offset + 1000),
          rename: rename.value,
          confirm: true
        }
      })
    }
    await workspace.refreshTasks()
    feedback.success(t('files.transferQueued'))
    selection.value = {}
  } catch (err) {
    feedback.failure(err)
  } finally {
    busy.value = false
  }
}
watch(
  () =>
    workspace.tasks.filter((task) => task.kind.startsWith('recycle_') && task.state === 'completed')
      .length,
  () => {
    void refresh()
  }
)
</script>

<template>
  <WorkspacePanel :title="t('recycle.title')" flush>
    <template v-if="workspace.accounts.length" #toolbar>
      <div class="flex w-full flex-wrap gap-2">
        <USelectMenu
          v-model="library"
          :items="roots.libraries.map((entry) => ({ label: entry.name, value: entry.id }))"
          value-key="value"
          :placeholder="t('recycle.library')"
          :aria-label="t('recycle.library')"
          class="min-w-0 flex-1 sm:w-56 sm:flex-none"
        /><UButton
          icon="i-lucide-refresh-cw"
          :aria-label="t('common.refresh')"
          color="neutral"
          variant="ghost"
          :loading="loading"
          @click="refreshAll()"
        /><UButton
          :label="t('recycle.empty')"
          color="error"
          variant="outline"
          class="ml-auto"
          :disabled="!interactionReady || !filteredCount || busy"
          @click="action('remove', true)"
        />
      </div>
    </template>
    <AccountEmptyState v-if="workspace.ready && !workspace.accounts.length" class="flex-1" />
    <template v-else>
      <OperationError v-if="rootError || error" :error="rootError || error" class="m-4 sm:mx-6" />
      <div class="flex shrink-0 flex-wrap items-center gap-2 px-4 py-3 sm:px-6">
        <UButton
          icon="i-lucide-undo-2"
          :label="t('recycle.restore')"
          :disabled="!selected.length || busy"
          @click="action('restore')"
        /><UButton
          icon="i-lucide-trash-2"
          :label="t('recycle.remove')"
          color="error"
          variant="soft"
          :disabled="!selected.length || busy"
          @click="action('remove')"
        /><UCheckbox v-model="rename" :label="t('recycle.restoreRename')" class="sm:ml-2" /><UInput
          v-model="filter"
          icon="i-lucide-list-filter"
          :placeholder="t('files.filterPlaceholder')"
          :aria-label="t('files.filterPlaceholder')"
          class="ml-auto w-full sm:w-auto"
        />
      </div>
      <UTable
        ref="recycleTable"
        v-model:row-selection="selection"
        v-model:global-filter="filter"
        v-model:pagination="pagination"
        :data="tableData"
        :columns="columns"
        :loading="loading"
        :caption="t('recycle.title')"
        sticky
        class="min-h-0 flex-1"
        :get-row-id="(row) => row.id"
        :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
      >
        <template #name-cell="{ row }">
          <div class="flex min-w-28 max-w-44 items-center gap-2 sm:max-w-sm">
            <UIcon
              :name="row.original.directory ? 'i-lucide-folder' : 'i-lucide-file'"
              class="text-muted size-4 shrink-0"
            /><span class="truncate" :title="row.original.name">{{ row.original.name }}</span>
          </div>
        </template>
        <template #path-cell="{ row }">
          <span class="text-muted block max-w-sm truncate text-xs" :title="row.original.path">{{
            row.original.path
          }}</span>
        </template>
        <template #size-cell="{ row }">
          {{ format.bytes(row.original.size) }}
        </template>
        <template #modified_at-cell="{ row }">
          <span class="whitespace-nowrap text-xs">{{ format.date(row.original.modified_at) }}</span>
        </template>
        <template #empty>
          <div>
            <UEmpty
              v-if="!loading && !rootError && !error"
              :icon="filter ? 'i-lucide-search' : 'i-lucide-trash-2'"
              :title="t(filter ? 'files.noResults' : 'recycle.emptyTitle')"
              :description="
                filter
                  ? t('files.noResultsHint')
                  : library
                    ? t('recycle.emptyHint')
                    : t('recycle.selectLibrary')
              "
              :actions="
                filter
                  ? [
                      {
                        label: t('common.clear'),
                        color: 'neutral',
                        variant: 'outline',
                        onClick: () => (filter = '')
                      }
                    ]
                  : []
              "
              class="py-12 sm:py-16"
            />
          </div>
        </template>
      </UTable>
    </template>
    <template v-if="workspace.accounts.length" #footer>
      <span class="text-muted text-xs">{{ t('common.selected', { count: selected.length }) }}</span
      ><UPagination
        v-if="(table?.tableApi.getFilteredRowModel().rows.length || 0) > pagination.pageSize"
        :page="pagination.pageIndex + 1"
        :items-per-page="pagination.pageSize"
        :total="table?.tableApi.getFilteredRowModel().rows.length || 0"
        @update:page="pagination.pageIndex = $event - 1"
      />
    </template>
  </WorkspacePanel>
</template>

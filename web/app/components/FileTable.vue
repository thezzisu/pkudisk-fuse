<script setup lang="ts">
import { getPaginationRowModel } from '@tanstack/vue-table'
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Entry, Listing } from '~/types/api'
defineProps<{
  entries: Entry[]
  stats?: Listing['stats']
  folder: Entry | null
  query: string
  loading: boolean
  failed?: boolean
  menu: (entry: Entry) => DropdownMenuItem[][]
}>()
const emit = defineEmits<{ open: [entry: Entry]; clear: [] }>()
const filter = defineModel<string>('filter', { default: '' })
const pagination = defineModel<{ pageIndex: number; pageSize: number }>('pagination', {
  required: true
})
const sorting = ref([{ id: 'name', desc: false }])
const table = useTemplateRef('filesTable')
const filteredCount = computed(() => table.value?.tableApi?.getFilteredRowModel().rows.length || 0)
defineExpose({ filteredCount })
const { t } = useI18n()
const format = useFormatting()
const columns = computed<TableColumn<Entry>[]>(() => [
  { accessorKey: 'name', header: t('common.name') },
  {
    accessorKey: 'size',
    header: t('common.size'),
    meta: { class: { th: 'w-32 text-end', td: 'text-end' } }
  },
  {
    accessorKey: 'modified_at',
    header: t('common.modified'),
    meta: { class: { th: 'hidden w-56 sm:table-cell', td: 'hidden sm:table-cell' } }
  },
  {
    id: 'actions',
    header: () => h('span', { class: 'sr-only' }, t('common.actions')),
    meta: { class: { th: 'w-px', td: 'w-px' } }
  }
])
</script>
<template>
  <UTable
    ref="filesTable"
    v-model:global-filter="filter"
    v-model:sorting="sorting"
    v-model:pagination="pagination"
    :data="entries"
    :get-row-id="(row) => row.id"
    :columns="columns"
    :loading="loading"
    :caption="query ? t('files.searchTitle') : folder?.name || t('files.libraries')"
    sticky
    :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
    class="min-h-0 flex-1"
  >
    <template #name-header="{ column }">
      <UButton
        color="neutral"
        variant="ghost"
        :label="t('common.name')"
        trailing-icon="i-lucide-arrow-up-down"
        class="-ms-2.5"
        @click="column.toggleSorting(column.getIsSorted() === 'asc')"
      />
    </template>
    <template #name-cell="{ row }">
      <div class="min-w-32 max-w-44 sm:min-w-44 sm:max-w-lg">
        <UButton
          :icon="row.original.directory ? 'i-lucide-folder' : 'i-lucide-file'"
          :label="row.original.name"
          :title="row.original.name"
          color="neutral"
          variant="link"
          class="max-w-full px-0 text-left"
          :ui="{ label: 'truncate' }"
          @click="emit('open', row.original)"
        />
        <p v-if="query && row.original.path" class="text-muted mt-1 truncate text-xs">
          {{ row.original.path }}
        </p>
      </div>
    </template>
    <template #size-cell="{ row }">
      <div v-if="stats?.[row.original.id]?.quota" class="space-y-1">
        <p class="text-muted whitespace-nowrap text-xs tabular-nums">
          {{ format.bytes(stats[row.original.id]!.quota!.used) }} /
          {{ format.bytes(stats[row.original.id]!.quota!.allocated) }}
        </p>
        <UBadge
          :label="t(stats[row.original.id]?.writable ? 'common.writable' : 'common.readOnly')"
          color="neutral"
          variant="subtle"
          size="xs"
        />
      </div>
      <span v-else class="text-muted whitespace-nowrap tabular-nums">{{
        row.original.directory ? t('files.folder') : format.bytes(row.original.size)
      }}</span>
    </template>
    <template #modified_at-cell="{ row }">
      <span class="text-muted whitespace-nowrap text-xs">{{
        format.date(row.original.modified_at)
      }}</span>
    </template>
    <template #actions-cell="{ row }">
      <UDropdownMenu :items="menu(row.original)">
        <UButton
          icon="i-lucide-ellipsis"
          color="neutral"
          variant="ghost"
          :aria-label="`${t('common.actions')}: ${row.original.name}`"
        />
      </UDropdownMenu>
    </template>
    <template #empty>
      <div>
        <UEmpty
          v-if="!loading && !failed"
          :icon="filter || query ? 'i-lucide-search' : 'i-lucide-folder-open'"
          :title="t(filter || query ? 'files.noResults' : 'files.emptyTitle')"
          :description="
            filter || query
              ? t('files.noResultsHint')
              : folder
                ? t('files.emptyHint')
                : t('common.empty')
          "
          :actions="
            filter || query
              ? [
                  {
                    label: t('common.clear'),
                    color: 'neutral',
                    variant: 'outline',
                    onClick: () => emit('clear')
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

<script setup lang="ts">
import { getPaginationRowModel } from '@tanstack/vue-table'
import type { DropdownMenuItem, TableColumn } from '@nuxt/ui'
import type { Entry, Listing } from '~/types/api'
defineProps<{
  account: string
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
const view = useFileViewStore()
const table = useTemplateRef('filesTable')
const filteredCount = computed(() => table.value?.tableApi?.getFilteredRowModel().rows.length || 0)
defineExpose({ filteredCount })
const { t, locale } = useI18n()
const format = useFormatting()
const collator = computed(
  () => new Intl.Collator(locale.value, { numeric: true, sensitivity: 'base' })
)
const hiddenColumns = { directory: false }
function sortIcon(column: string) {
  const sort = view.sorting.find((entry) => entry.id === column)
  return !sort ? 'i-lucide-arrow-up-down' : sort.desc ? 'i-lucide-arrow-down' : 'i-lucide-arrow-up'
}
const columns = computed<TableColumn<Entry>[]>(() => [
  { accessorKey: 'directory', sortingFn: 'basic', enableGlobalFilter: false },
  {
    accessorKey: 'name',
    header: t('common.name'),
    sortingFn: (a, b) => collator.value.compare(a.original.name, b.original.name),
    meta: { class: { td: 'min-w-0' } }
  },
  {
    accessorKey: 'size',
    header: t('common.size'),
    meta: {
      class: { th: 'hidden w-32 text-end md:table-cell', td: 'hidden text-end md:table-cell' }
    }
  },
  {
    id: 'modified_at',
    accessorFn: (entry) => Date.parse(entry.modified_at) || 0,
    sortingFn: 'basic',
    header: t('common.modified'),
    meta: { class: { th: 'hidden w-56 md:table-cell', td: 'hidden md:table-cell' } }
  },
  {
    id: 'actions',
    header: () => h('span', { class: 'sr-only' }, t('common.actions')),
    meta: { class: { th: 'hidden w-px md:table-cell', td: 'hidden w-px md:table-cell' } }
  }
])
</script>
<template>
  <UTable
    ref="filesTable"
    v-model:global-filter="filter"
    v-model:pagination="pagination"
    :sorting="view.sorting"
    :column-visibility="hiddenColumns"
    :data="entries"
    :get-row-id="(row) => row.id"
    :columns="columns"
    :loading="loading"
    :caption="query ? t('files.searchTitle') : folder?.name || t('files.libraries')"
    sticky
    :pagination-options="{ getPaginationRowModel: getPaginationRowModel() }"
    :ui="{
      base: 'w-full table-fixed md:table-auto',
      thead: 'hidden md:table-header-group',
      td: 'py-2 whitespace-normal md:py-3'
    }"
    class="min-h-0 flex-1"
  >
    <template #name-header>
      <UButton
        color="neutral"
        variant="ghost"
        :label="t('common.name')"
        :trailing-icon="sortIcon('name')"
        class="-ms-2.5"
        @click="view.toggle('name')"
      />
    </template>
    <template #name-cell="{ row }">
      <div class="flex min-w-0 items-center gap-2 md:min-w-44 md:max-w-lg">
        <UButton
          :aria-label="row.original.name"
          :title="row.original.name"
          color="neutral"
          variant="link"
          class="min-h-11 min-w-0 flex-1 items-start gap-2 p-0 text-start text-highlighted md:min-h-0"
          @click="emit('open', row.original)"
        >
          <template #leading>
            <FileThumbnail :account="account" :entry="row.original" />
          </template>
          <span class="min-w-0 flex-1">
            <span class="block text-sm leading-6 font-medium [overflow-wrap:anywhere]">{{
              row.original.name
            }}</span>
            <span
              class="text-muted mt-0.5 block text-xs leading-4 font-normal [overflow-wrap:anywhere] md:hidden"
            >
              <template v-if="stats?.[row.original.id]?.quota">
                {{ format.bytes(stats[row.original.id]!.quota!.used) }} /
                {{ format.bytes(stats[row.original.id]!.quota!.allocated) }}
                <UBadge
                  :label="
                    t(stats[row.original.id]?.writable ? 'common.writable' : 'common.readOnly')
                  "
                  color="neutral"
                  variant="subtle"
                  size="xs"
                  class="ms-1 align-middle"
                />
              </template>
              <template v-else>
                {{ row.original.directory ? t('files.folder') : format.bytes(row.original.size) }}
              </template>
              <template v-if="format.date(row.original.modified_at) !== '—'">
                <span aria-hidden="true"> · </span>{{ format.date(row.original.modified_at) }}
              </template>
            </span>
            <span
              v-if="query && row.original.path"
              class="text-muted mt-0.5 block text-xs leading-4 font-normal [overflow-wrap:anywhere]"
            >
              {{ row.original.path }}
            </span>
          </span>
        </UButton>
        <UDropdownMenu :items="menu(row.original)">
          <UButton
            icon="i-lucide-ellipsis"
            color="neutral"
            variant="ghost"
            class="size-11 shrink-0 justify-center md:hidden"
            :aria-label="`${t('common.actions')}: ${row.original.name}`"
          />
        </UDropdownMenu>
      </div>
    </template>
    <template #size-header>
      <UButton
        color="neutral"
        variant="ghost"
        :label="t('common.size')"
        :trailing-icon="sortIcon('size')"
        class="-me-2.5"
        @click="view.toggle('size')"
      />
    </template>
    <template #modified_at-header>
      <UButton
        color="neutral"
        variant="ghost"
        :label="t('common.modified')"
        :trailing-icon="sortIcon('modified_at')"
        class="-ms-2.5"
        @click="view.toggle('modified')"
      />
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

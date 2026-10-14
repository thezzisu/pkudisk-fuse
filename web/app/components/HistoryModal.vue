<script setup lang="ts">
import type { Entry, Revision } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'

const props = defineProps<{ account: string; entry: Entry }>()
const emit = defineEmits<{ close: [changed: boolean] }>()
const { t } = useI18n()
const { $api } = useNuxtApp()
const format = useFormatting()
const confirm = useOverlay().create(ConfirmModal)
const changed = ref(false)
const { data, status, error, refresh } = useLazyAsyncData<Revision[]>(
  `history-${useId()}`,
  () => $api<Revision[]>('/revisions', { query: { account: props.account, path: props.entry.id } }),
  { default: () => [] }
)
const { busy, error: actionError, run } = useApiAction({ refresh, success: true })
const columns = computed(() => [
  { accessorKey: 'modified_at', header: t('common.modified') },
  {
    accessorKey: 'editor',
    header: t('history.editor'),
    meta: { class: { th: 'hidden sm:table-cell', td: 'hidden sm:table-cell' } }
  },
  { accessorKey: 'size', header: t('common.size') },
  { id: 'actions', header: () => h('span', { class: 'sr-only' }, t('common.actions')) }
])
async function restore(revision: Revision) {
  if (
    !(await confirm.open({
      title: t('history.restoreTitle'),
      description: t('history.restoreHint'),
      destructive: true
    }).result)
  )
    return
  await run(async () => {
    await $api('/revisions/restore', {
      method: 'POST',
      body: { account: props.account, path: props.entry.id, revision: revision.rev, confirm: true }
    })
    changed.value = true
  })
}
</script>

<template>
  <UModal
    :title="t('history.title')"
    :description="`${account} · ${entry.name}`"
    :close="{ onClick: () => emit('close', changed) }"
    :ui="{ content: 'sm:max-w-3xl', body: 'p-0 sm:p-0' }"
  >
    <template #body>
      <OperationError
        v-if="error || actionError"
        :error="error || actionError"
        class="m-4 sm:mx-6"
      />
      <UTable
        :data="data"
        :columns="columns"
        :caption="t('history.title')"
        :loading="status === 'pending' || status === 'idle'"
      >
        <template #modified_at-cell="{ row }">
          <p class="whitespace-nowrap text-xs sm:text-sm">
            {{ format.date(row.original.modified_at) }}
          </p>
          <p class="text-muted mt-1 text-xs sm:hidden">{{ row.original.editor }}</p>
          <p
            class="text-muted mt-1 max-w-40 truncate text-xs sm:max-w-56"
            :title="row.original.rev"
          >
            {{ row.original.rev }}
          </p>
        </template>
        <template #size-cell="{ row }">
          {{ format.bytes(row.original.size) }}
        </template>
        <template #actions-cell="{ row }">
          <UButton
            icon="i-lucide-history"
            :label="t('history.restore')"
            :aria-label="t('history.restore')"
            :ui="{ label: 'hidden sm:inline' }"
            color="neutral"
            variant="outline"
            size="xs"
            :disabled="busy"
            @click="restore(row.original)"
          />
        </template>
        <template #empty>
          <div>
            <UEmpty
              v-if="status === 'success'"
              icon="i-lucide-history"
              :title="t('history.empty')"
              class="py-12"
            />
          </div>
        </template>
      </UTable>
    </template>
  </UModal>
</template>

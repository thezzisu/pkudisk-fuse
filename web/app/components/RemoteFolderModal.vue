<script setup lang="ts">
import type { Entry, Listing } from '~/types/api'

const props = defineProps<{ account: string; initial?: string }>()
const emit = defineEmits<{ close: [value: { path: string; name: string } | null] }>()
const { $api } = useNuxtApp()
const { t } = useI18n()
const path = ref(props.initial || '/')
const { data, status, error } = useAsyncData(
  `folder-picker-${useId()}`,
  () => $api<Listing>('/files', { query: { account: props.account, path: path.value } }),
  { watch: [path] }
)
const folders = computed(() => data.value?.entries.filter((entry) => entry.directory) || [])
function parent() {
  const id = data.value?.folder?.id || ''
  const cut = id.lastIndexOf('/')
  path.value = cut > 'gns://'.length ? id.slice(0, cut) : '/'
}
function open(entry: Entry) {
  path.value = entry.id
}
</script>

<template>
  <UModal
    :title="t('files.selectDestination')"
    :description="data?.folder?.name || t('files.libraries')"
    :close="{ onClick: () => emit('close', null) }"
    :ui="{ body: 'p-0 sm:p-0' }"
  >
    <template #body>
      <OperationError v-if="error" :error="error" class="m-4 sm:mx-6" />
      <UButton
        v-if="data?.folder"
        icon="i-lucide-arrow-up"
        :label="t('files.up')"
        color="neutral"
        variant="ghost"
        class="mx-4 my-2 sm:mx-6"
        @click="parent"
      />
      <UTable
        :data="folders"
        :columns="[{ accessorKey: 'name', header: t('common.name') }]"
        :loading="status === 'pending'"
        :caption="t('files.selectDestination')"
        class="max-h-80"
      >
        <template #name-cell="{ row }">
          <UButton
            icon="i-lucide-folder"
            :label="row.original.name"
            color="neutral"
            variant="link"
            class="max-w-full px-0"
            :ui="{ label: 'truncate' }"
            @click="open(row.original)"
          />
        </template>
        <template #empty>
          <div>
            <UEmpty
              v-if="status === 'success'"
              :title="t('common.empty')"
              icon="i-lucide-folder"
              size="sm"
              class="py-10"
            />
          </div>
        </template>
      </UTable>
    </template>
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton
          :label="t('common.cancel')"
          color="neutral"
          variant="outline"
          @click="emit('close', null)"
        />
        <UButton
          :label="t('files.useFolder')"
          :disabled="status !== 'success' || !data?.folder || !data.writable"
          @click="emit('close', { path: data!.folder!.id, name: data!.folder!.name })"
        />
      </div>
    </template>
  </UModal>
</template>

<script setup lang="ts">
import type { RecoveryRecord } from '~/types/api'
import TransferModal from '~/components/TransferModal.vue'
import LocalPickerModal from '~/components/LocalPickerModal.vue'

definePageMeta({ middleware: 'host-flags' })

const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const format = useFormatting()
const feedback = useFeedback()
const clipboard = useClipboard()
const transfer = useOverlay().create(TransferModal)
const path = ref('')
const localPicker = useOverlay().create(LocalPickerModal)
async function browse() {
  const selected = await localPicker.open({
    initial: path.value || workspace.cache,
    kind: 'directory',
    title: t('mounts.cache')
  }).result
  if (selected) {
    path.value = selected.path
    await refresh()
  }
}
const hasMounts = computed(() => workspace.activeTasks.some((task) => task.kind === 'mount'))
function originalAccount(record: RecoveryRecord) {
  return workspace.accounts.find(
    (account) => account.name === record.account && account.server === record.server
  )
}
function upload(record: RecoveryRecord) {
  const account = originalAccount(record)?.name || workspace.selected
  if (account && !hasMounts.value)
    transfer.open({
      kind: 'upload',
      account,
      entry: record.entry,
      recoverySource: record.data,
      initialDestination: record.parent
    })
}
const { data, status, error, refresh } = useLazyAsyncData(
  'recovery',
  async () => {
    return $api<{ path: string; entries: RecoveryRecord[] }>('/recovery', {
      query: { path: path.value || workspace.cache }
    })
  },
  { default: () => ({ path: '', entries: [] }) }
)
async function copy(path: string) {
  await clipboard.copy(path)
  feedback.success(t('common.copied'))
}
</script>

<template>
  <WorkspacePanel :title="t('recovery.title')" :description="t('recovery.description')">
    <template #toolbar>
      <UFieldGroup class="w-full max-w-2xl">
        <UInput
          :model-value="path"
          readonly
          :placeholder="workspace.cache || data.path"
          :aria-label="t('mounts.cache')"
          class="min-w-0 flex-1"
          @click="browse"
        /><UButton
          :label="t('common.browse')"
          icon="i-lucide-folder-search"
          color="neutral"
          variant="outline"
          @click="browse"
        /><UButton
          :label="t('recovery.scan')"
          icon="i-lucide-search"
          :loading="status === 'pending'"
          @click="refresh()"
        />
      </UFieldGroup>
    </template>
    <UAlert
      v-if="data.entries.length"
      icon="i-lucide-info"
      color="warning"
      variant="subtle"
      :description="t('recovery.warning')"
    />
    <OperationError v-if="error" :error="error" />
    <UProgress v-if="status === 'pending' || status === 'idle'" :aria-label="t('common.loading')" />
    <UEmpty
      v-else-if="!error && !data.entries.length"
      icon="i-lucide-shield-check"
      :title="t('recovery.emptyTitle')"
      :description="t('recovery.emptyHint')"
      class="py-12 sm:py-16"
    />
    <div v-else-if="!error && data.entries.length" class="divide-default divide-y">
      <article v-for="record in data.entries" :key="record.data" class="space-y-3 py-5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-highlighted font-medium">
            {{ record.entry.name }}
          </h2>
          <UBadge v-if="record.account" color="neutral" variant="subtle" :label="record.account" />
        </div>
        <p class="text-muted text-sm">
          {{ format.bytes(record.entry.size) }} · {{ format.date(record.updated) }}
        </p>
        <dl class="grid gap-2 text-xs">
          <div v-if="record.server">
            <dt class="text-muted">
              {{ t('accounts.server') }}
            </dt>
            <dd class="mt-1 break-all">
              {{ record.server }}
            </dd>
          </div>
          <div>
            <dt class="text-muted">
              {{ t('recovery.localFile') }}
            </dt>
            <dd class="mt-1 break-all">
              {{ record.data }}
            </dd>
          </div>
          <div>
            <dt class="text-muted">
              {{ t('recovery.remoteFile') }}
            </dt>
            <dd class="mt-1 break-all">
              {{ record.entry.id }}
            </dd>
          </div>
          <div>
            <dt class="text-muted">
              {{ t('recovery.revision') }}
            </dt>
            <dd class="mt-1 break-all">
              {{ record.entry.rev }}
            </dd>
          </div>
        </dl>
        <UAlert
          v-if="record.account && !originalAccount(record)"
          color="warning"
          variant="subtle"
          :description="t('recovery.accountChanged')"
        />
        <div class="flex gap-2">
          <UButton
            icon="i-lucide-copy"
            :label="t('recovery.copyPath')"
            color="neutral"
            variant="outline"
            @click="copy(record.data)"
          /><UButton
            icon="i-lucide-upload"
            :label="t('recovery.upload')"
            :title="t('recovery.uploadHint')"
            variant="soft"
            :disabled="hasMounts || (!workspace.selected && !originalAccount(record))"
            @click="upload(record)"
          />
        </div>
      </article>
    </div>
  </WorkspacePanel>
</template>

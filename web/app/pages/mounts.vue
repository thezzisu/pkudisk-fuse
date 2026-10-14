<script setup lang="ts">
import type { Job, MountSupport } from '~/types/api'
import MountModal from '~/components/MountModal.vue'
import ConfirmModal from '~/components/ConfirmModal.vue'

definePageMeta({ middleware: 'host-flags' })

const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const feedback = useFeedback()
const overlay = useOverlay()
const modal = overlay.create(MountModal)
const confirm = overlay.create(ConfirmModal)
const {
  data: support,
  error,
  refresh
} = useLazyAsyncData('mount-support', () => $api<MountSupport>('/mounts/support'))
const mounts = computed(() => workspace.tasks.filter((task) => task.kind === 'mount'))
const active = (task: Job) => ['queued', 'running', 'mounted', 'stopping'].includes(task.state)
async function stop(task: Job) {
  if (
    !(await confirm.open({ title: t('mounts.unmountTitle'), description: t('mounts.unmountHint') })
      .result)
  )
    return
  try {
    await $api(`/tasks/${task.id}/cancel`, { method: 'POST' })
    await workspace.refreshTasks()
  } catch (err) {
    feedback.failure(err)
  }
}
async function clear(task: Job) {
  try {
    await $api(`/tasks/${task.id}`, { method: 'DELETE' })
    await workspace.refreshTasks()
  } catch (err) {
    feedback.failure(err)
  }
}
</script>

<template>
  <WorkspacePanel :title="t('mounts.title')" :description="t('mounts.description')">
    <template #toolbar>
      <div class="flex w-full justify-between">
        <UButton
          icon="i-lucide-refresh-cw"
          color="neutral"
          variant="ghost"
          :label="t('common.refresh')"
          @click="refresh()"
        /><UButton
          icon="i-lucide-hard-drive"
          :label="t('mounts.add')"
          :disabled="!support?.available || !workspace.selected"
          @click="modal.open({ account: workspace.selected, platform: support!.platform })"
        />
      </div>
    </template>
    <OperationError v-if="error" :error="error" />
    <MountDriverInfo v-if="support && !support.available" :support="support" />
    <UEmpty
      v-if="!mounts.length"
      icon="i-lucide-hard-drive"
      :title="t('mounts.emptyTitle')"
      :description="t('mounts.emptyHint')"
      class="py-12 sm:py-16"
    />
    <div v-else class="grid gap-4 xl:grid-cols-2">
      <UCard v-for="mount in mounts" :key="mount.id">
        <div class="flex items-start gap-3">
          <UIcon name="i-lucide-hard-drive" class="text-primary mt-1 size-6 shrink-0" />
          <div class="min-w-0 flex-1">
            <p class="text-highlighted break-all font-medium">
              {{ mount.destination }}
            </p>
            <p class="text-muted mt-1 text-sm">
              {{ mount.allAccounts ? t('mounts.allAccounts') : mount.account }}
            </p>
          </div>
          <UBadge
            :label="t(`tasks.states.${mount.state}`)"
            :color="
              mount.state === 'mounted' ? 'success' : mount.state === 'failed' ? 'error' : 'neutral'
            "
            variant="subtle"
          />
        </div>
        <p v-if="mount.source" class="text-muted mt-4 break-all text-xs">
          {{ mount.source }}
        </p>
        <UProgress v-if="['queued', 'running', 'stopping'].includes(mount.state)" class="mt-5" />
        <OperationError v-if="mount.error" :error="mount.error" class="mt-4" />
        <p v-if="mount.error && mount.state === 'mounted'" class="text-muted mt-2 text-xs">
          {{ t('mounts.busyHint') }}
        </p>
        <div class="mt-5 flex justify-end">
          <UButton
            v-if="active(mount)"
            icon="i-lucide-unplug"
            :label="t('mounts.unmount')"
            color="neutral"
            variant="outline"
            :disabled="mount.state === 'stopping'"
            @click="stop(mount)"
          /><UButton
            v-else
            :label="t('tasks.remove')"
            color="neutral"
            variant="ghost"
            @click="clear(mount)"
          />
        </div>
      </UCard>
    </div>
  </WorkspacePanel>
</template>

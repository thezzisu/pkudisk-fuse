<script setup lang="ts">
import type { Job } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'

const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const format = useFormatting()
const feedback = useFeedback()
const confirm = useOverlay().create(ConfirmModal)
const browser = useBrowserTransfersStore()
const selected = ref('all')
const active = (task: Job) => ['queued', 'running', 'stopping'].includes(task.state)
const tasks = computed(() =>
  workspace.tasks.filter(
    (task) =>
      task.kind !== 'mount' &&
      (selected.value === 'all' || (selected.value === 'active' ? active(task) : !active(task)))
  )
)
const colors = {
  completed: 'success',
  failed: 'error',
  cancelled: 'neutral',
  queued: 'neutral',
  running: 'info',
  stopping: 'warning'
} as const

async function cancel(task: Job) {
  if (
    !(await confirm.open({ title: t('tasks.cancelTitle'), description: t('tasks.cancelHint') })
      .result)
  )
    return
  try {
    if (task.browser) {
      browser.cancel(task.id)
      return
    }
    await $api(`/tasks/${task.id}/cancel`, { method: 'POST' })
    await workspace.refreshTasks()
  } catch (error) {
    feedback.failure(error)
  }
}
async function clear(task: Job) {
  try {
    if (task.browser) {
      browser.remove(task.id)
      return
    }
    if (task.clientId) browser.remove(`browser:${task.clientId}`)
    await $api(`/tasks/${task.id}`, { method: 'DELETE' })
    await workspace.refreshTasks()
  } catch (error) {
    feedback.failure(error)
  }
}
</script>

<template>
  <WorkspacePanel :title="t('tasks.title')" :description="t('tasks.description')">
    <template #toolbar>
      <UTabs
        v-model="selected"
        :items="[
          { label: t('common.all'), value: 'all' },
          { label: t('tasks.active'), value: 'active' },
          { label: t('tasks.history'), value: 'history' }
        ]"
        :content="false"
        variant="pill"
        color="neutral"
        size="sm"
        :ui="{ trigger: 'text-default data-[state=inactive]:text-default' }"
      />
    </template>
    <UEmpty
      v-if="!tasks.length"
      icon="i-lucide-arrow-down-up"
      :title="
        t(
          selected === 'active'
            ? 'tasks.noActive'
            : selected === 'history'
              ? 'tasks.noHistory'
              : 'tasks.emptyTitle'
        )
      "
      :description="t('tasks.emptyHint')"
      class="py-12 sm:py-16"
    />
    <div v-else class="divide-default -mx-4 divide-y sm:-mx-6">
      <article v-for="task in tasks" :key="task.id" class="space-y-3 px-4 py-4 first:pt-0 sm:px-6">
        <div class="flex items-start gap-3">
          <div class="bg-muted flex size-10 shrink-0 items-center justify-center rounded-lg">
            <UIcon
              :name="
                task.kind === 'upload'
                  ? 'i-lucide-upload'
                  : task.kind === 'download'
                    ? 'i-lucide-download'
                    : 'i-lucide-folder-sync'
              "
              class="text-primary size-5"
            />
          </div>
          <div class="min-w-0 flex-1">
            <p class="text-highlighted truncate font-medium">
              {{ task.name }}
            </p>
            <p class="text-muted mt-1 text-xs">
              {{ t(`tasks.kinds.${task.kind}`) }} · {{ task.account }} ·
              {{ format.date(task.created) }}
            </p>
          </div>
          <UBadge
            :color="colors[task.state as keyof typeof colors] || 'neutral'"
            variant="subtle"
            :label="t(`tasks.states.${task.state}`)"
          />
          <UButton
            v-if="active(task)"
            icon="i-lucide-x"
            :aria-label="t('tasks.cancel')"
            color="neutral"
            variant="ghost"
            :disabled="task.state === 'stopping'"
            @click="cancel(task)"
          />
          <UButton
            v-else
            icon="i-lucide-trash-2"
            :aria-label="t('tasks.remove')"
            color="neutral"
            variant="ghost"
            @click="clear(task)"
          />
        </div>
        <div v-if="active(task)" class="space-y-2 sm:ml-13">
          <UProgress :model-value="task.total > 0 || task.percent > 0 ? task.percent : undefined" />
          <div class="text-muted flex justify-between text-xs">
            <span>{{
              task.phase ? t(`tasks.phases.${task.phase}`) : t(`tasks.states.${task.state}`)
            }}</span
            ><span class="tabular-nums">{{
              task.total > 0
                ? t('tasks.progress', {
                    done: task.kind.startsWith('recycle_')
                      ? String(task.done)
                      : format.bytes(task.done),
                    total: task.kind.startsWith('recycle_')
                      ? String(task.total)
                      : format.bytes(task.total)
                  })
                : `${task.percent}%`
            }}</span>
          </div>
        </div>
        <div class="text-muted grid gap-1 text-xs sm:ml-13">
          <p class="break-all">{{ t('files.source') }}: {{ task.source || '—' }}</p>
          <p class="break-all">{{ t('files.destination') }}: {{ task.destination || '—' }}</p>
        </div>
        <OperationError v-if="task.error" :error="task.error" class="sm:ml-13" />
      </article>
    </div>
  </WorkspacePanel>
</template>

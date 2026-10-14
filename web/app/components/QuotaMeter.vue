<script setup lang="ts">
const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const format = useFormatting()
const { data } = useLazyAsyncData(
  'account-quota',
  async () => {
    if (!workspace.selected) return null
    return $api<{ allocated: number; used: number }>('/quota', {
      query: { account: workspace.selected }
    })
  },
  { watch: [() => workspace.selected] }
)
</script>

<template>
  <div v-if="data" class="space-y-2">
    <div class="text-muted flex items-center justify-between text-xs">
      <span>{{ t('files.quota') }}</span
      ><span class="tabular-nums"
        >{{
          data.allocated ? Math.min(100, Math.round((100 * data.used) / data.allocated)) : 0
        }}%</span
      >
    </div>
    <UProgress :model-value="data.used" :max="Math.max(data.allocated, data.used, 1)" size="xs" />
    <p class="text-muted text-xs">
      {{
        t('files.quotaUsed', { used: format.bytes(data.used), total: format.bytes(data.allocated) })
      }}
    </p>
  </div>
</template>

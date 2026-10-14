<script setup lang="ts">
import type { MountSupport } from '~/types/api'

const props = defineProps<{ support: MountSupport }>()
const { t, te } = useI18n()
const description = computed(() => {
  const key = `mounts.drivers.${props.support.platform}`
  return t(te(key) ? key : 'mounts.drivers.unsupported')
})
const links = computed(() =>
  props.support.platform === 'windows'
    ? [{ label: 'WinFsp', to: 'https://winfsp.dev/rel/', target: '_blank' }]
    : props.support.platform === 'darwin'
      ? [{ label: 'macFUSE', to: 'https://macfuse.github.io/', target: '_blank' }]
      : []
)
</script>

<template>
  <UAlert
    color="warning"
    variant="subtle"
    icon="i-lucide-plug"
    :title="t('mounts.unsupported')"
    :description="description"
    :actions="links"
  >
    <template #description>
      <p>{{ description }}</p>
      <UCollapsible v-if="support.message" class="mt-2">
        <UButton :label="t('common.showError')" color="warning" variant="link" size="xs" />
        <template #content>
          <p class="mt-2 text-xs">
            {{ support.message }}
          </p>
        </template>
      </UCollapsible>
    </template>
  </UAlert>
</template>

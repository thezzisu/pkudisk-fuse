<script setup lang="ts">
defineProps<{ title: string; description?: string; label?: string; destructive?: boolean }>()
const emit = defineEmits<{ close: [confirmed: boolean] }>()
const { t } = useI18n()
</script>

<template>
  <UModal
    :title="title"
    :description="description"
    :close="{ onClick: () => emit('close', false) }"
  >
    <template #footer>
      <div class="flex w-full justify-end gap-2">
        <UButton
          color="neutral"
          variant="outline"
          :label="t('common.cancel')"
          @click="emit('close', false)"
        />
        <UButton
          :color="destructive ? 'error' : 'primary'"
          :label="label || t('common.confirm')"
          @click="emit('close', true)"
        />
      </div>
    </template>
  </UModal>
</template>

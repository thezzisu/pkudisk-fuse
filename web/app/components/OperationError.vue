<script setup lang="ts">
const props = defineProps<{ error: unknown }>()
const { t } = useI18n()
const feedback = useFeedback()
const workspace = useWorkspaceStore()
const detail = computed(() => feedback.detail(props.error))
</script>

<template>
  <UAlert
    v-if="error"
    color="error"
    variant="soft"
    icon="i-lucide-circle-alert"
    :title="feedback.message(error)"
  >
    <template #description>
      <UCollapsible v-if="detail.message" class="mt-1">
        <UButton color="error" variant="link" size="xs" :label="t('common.showError')" />
        <template #content>
          <p class="mt-2 break-words text-xs">
            {{ detail.message }}
          </p>
        </template>
      </UCollapsible>
      <UButton
        v-if="detail.code === 'login_required' && workspace.current"
        color="error"
        variant="outline"
        class="mt-2"
        :label="t('accounts.reconnect')"
        @click="workspace.login(workspace.current, true)"
      />
    </template>
  </UAlert>
</template>

<script setup lang="ts">
const { t } = useI18n()
const workspace = useWorkspaceStore()
</script>

<template>
  <UEmpty
    icon="i-lucide-users-round"
    :title="t('accounts.emptyTitle')"
    :description="t('accounts.emptyHint')"
    :actions="[
      {
        label: t('accounts.add'),
        icon: 'i-lucide-user-round-plus',
        onClick: () => workspace.login()
      }
    ]"
  />
</template>

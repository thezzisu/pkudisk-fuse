<script setup lang="ts">
const workspace = useWorkspaceStore()
const { t } = useI18n()
const items = computed(() =>
  workspace.accounts.map((account) => ({
    label: account.name,
    value: account.name,
    icon: 'i-lucide-user-round'
  }))
)
</script>

<template>
  <USelectMenu
    v-if="items.length"
    v-model="workspace.selected"
    :items="items"
    value-key="value"
    :aria-label="t('common.account')"
    :placeholder="t('common.selectAccount')"
    class="w-40 sm:w-48"
  />
</template>

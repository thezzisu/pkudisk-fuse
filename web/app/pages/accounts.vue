<script setup lang="ts">
import type { Account } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'

const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const error = ref<unknown>(null)
async function refreshAccounts() {
  await workspace.refreshAccounts()
  error.value = null
}
const { run } = useApiAction({
  refresh: refreshAccounts,
  notifyError: true,
  success: true
})
const confirm = useOverlay().create(ConfirmModal)
onMounted(() =>
  refreshAccounts().catch((err) => {
    error.value = err
  })
)

async function setDefault(account: Account) {
  await run(async () => {
    await $api('/accounts/default', { method: 'POST', body: { account: account.name } })
  })
}
async function remove(account: Account) {
  if (
    !(await confirm.open({
      title: t('accounts.removeTitle'),
      description: t('accounts.removeHint'),
      destructive: true
    }).result)
  )
    return
  await run(async () => {
    await $api('/accounts/remove', {
      method: 'POST',
      body: { account: account.name, confirm: true }
    })
  })
}
async function files(account: Account) {
  workspace.selected = account.name
  await navigateTo({ path: '/', query: { account: account.name } })
}
</script>

<template>
  <WorkspacePanel
    :title="t('accounts.title')"
    :description="t('accounts.description')"
    :account-switcher="false"
  >
    <template #toolbar>
      <div class="flex w-full justify-end">
        <UButton
          data-testid="add-account"
          icon="i-lucide-user-round-plus"
          :label="t('accounts.add')"
          @click="workspace.login()"
        />
      </div>
    </template>
    <OperationError v-if="error" :error="error" />
    <UEmpty
      v-if="!error && !workspace.ready"
      loading
      :title="t('common.loading')"
      class="py-12 sm:py-16"
    />
    <UEmpty
      v-else-if="!error && !workspace.accounts.length"
      icon="i-lucide-users-round"
      :title="t('accounts.emptyTitle')"
      :description="t('accounts.emptyHint')"
      class="py-12 sm:py-16"
    />
    <div v-else class="grid gap-4 xl:grid-cols-2">
      <UCard v-for="account in workspace.accounts" :key="account.name">
        <div class="flex items-start justify-between gap-3">
          <UUser
            :name="account.name"
            :description="account.username"
            :avatar="{ icon: 'i-lucide-user-round' }"
            size="lg"
          />
          <UBadge
            v-if="account.default"
            color="primary"
            variant="subtle"
            :label="t('common.default')"
          />
        </div>
        <p class="text-muted mt-4 truncate text-sm">
          {{ account.server }}
        </p>
        <div class="mt-5 flex flex-wrap items-center gap-2">
          <UButton
            icon="i-lucide-folder-open"
            :label="t('accounts.openFiles')"
            variant="soft"
            @click="files(account)"
          />
          <UDropdownMenu
            :items="[
              [
                {
                  label: t('accounts.reconnect'),
                  icon: 'i-lucide-refresh-cw',
                  onSelect: () => workspace.login(account, true)
                },
                {
                  label: t('accounts.changeLogin'),
                  icon: 'i-lucide-key-round',
                  onSelect: () => workspace.login(account)
                },
                {
                  label: t('accounts.setDefault'),
                  icon: 'i-lucide-star',
                  disabled: account.default,
                  onSelect: () => setDefault(account)
                }
              ],
              [
                {
                  label: t('common.remove'),
                  icon: 'i-lucide-trash-2',
                  color: 'error',
                  onSelect: () => remove(account)
                }
              ]
            ]"
          >
            <UButton
              icon="i-lucide-ellipsis"
              color="neutral"
              variant="ghost"
              :aria-label="t('common.actions')"
            />
          </UDropdownMenu>
          <UBadge
            class="ml-auto"
            :color="account.connected ? 'success' : 'neutral'"
            variant="subtle"
            :label="t(account.connected ? 'accounts.connected' : 'accounts.saved')"
          />
        </div>
      </UCard>
    </div>
  </WorkspacePanel>
</template>

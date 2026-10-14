<script setup lang="ts">
import type { Entry, Permission, ShareLink, SharingResponse } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'
const props = defineProps<{ account: string; entry: Entry }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const { $api } = useNuxtApp()
const confirm = useOverlay().create(ConfirmModal)
const clipboard = useClipboard()
const tab = ref('internal')
const expanded = ref(false)
const formBusy = ref(false)
const identity = useId()
const formId = useId()
const { data, status, error, refresh } = useLazyAsyncData(
  `sharing-${identity}`,
  () =>
    $api<SharingResponse>('/sharing', {
      query: { account: props.account, path: props.entry.id, external: tab.value === 'external' }
    }),
  { watch: [tab] }
)
const { busy: actionBusy, error: actionError, run } = useApiAction({ refresh })
const busy = computed(() => actionBusy.value || formBusy.value)
watch(tab, () => {
  actionError.value = null
  expanded.value = false
})
async function copy(value: string) {
  await run(() => clipboard.copy(value), t('common.copied'))
}
async function internalLink() {
  await run(async () => {
    const link = await $api<ShareLink>('/sharing/action', {
      method: 'POST',
      body: { action: 'internal_link', account: props.account, path: props.entry.id }
    })
    await clipboard.copy(link.url)
  }, t('common.copied'))
}
async function revoke(permission: Permission) {
  if (
    !(await confirm.open({
      title: t('sharing.revokeTitle'),
      description: t('sharing.revokeHint'),
      destructive: true
    }).result)
  )
    return
  await run(
    () =>
      $api('/sharing/action', {
        method: 'POST',
        body: {
          action: 'revoke',
          account: props.account,
          path: props.entry.id,
          recipients: [{ id: permission.id, type: permission.type }],
          confirm: true
        }
      }),
    true
  )
}
async function removeLink(link: ShareLink) {
  if (
    !(await confirm.open({
      title: t('sharing.revokeLinkTitle'),
      description: t('sharing.revokeLinkHint'),
      destructive: true
    }).result)
  )
    return
  await run(
    () =>
      $api('/sharing/action', {
        method: 'POST',
        body: { action: 'remove_link', account: props.account, linkId: link.id, confirm: true }
      }),
    true
  )
}
async function saved() {
  await refresh()
  expanded.value = false
}
</script>

<template>
  <UModal
    :title="t('sharing.title')"
    :description="`${account} · ${entry.name}`"
    :close="{ onClick: () => emit('close') }"
    :ui="{ content: 'sm:max-w-3xl', body: 'p-0 sm:p-0' }"
  >
    <template #body>
      <UTabs
        v-model="tab"
        :content="false"
        variant="link"
        :ui="{ root: 'gap-0', list: 'p-0 px-4 sm:px-6', trigger: 'py-3', indicator: 'bottom-0' }"
        :items="[
          { label: t('sharing.internal'), value: 'internal', icon: 'i-lucide-users-round' },
          { label: t('sharing.external'), value: 'external', icon: 'i-lucide-link' }
        ]"
      />
      <div class="space-y-3 px-4 py-4 sm:px-6">
        <p class="text-muted text-sm leading-relaxed">
          {{ t(tab === 'internal' ? 'sharing.internalHint' : 'sharing.externalHint') }}
        </p>
        <OperationError v-if="error || actionError" :error="error || actionError" />
        <OperationError v-if="data?.policyError" :error="data.policyError" />
      </div>
      <UProgress v-if="status === 'pending'" />
      <template v-if="data && !expanded">
        <template v-if="tab === 'internal'">
          <div class="space-y-3 px-4 pb-4 sm:px-6">
            <UButton
              icon="i-lucide-link"
              :label="t('sharing.getLink')"
              color="neutral"
              variant="outline"
              :loading="busy"
              @click="internalLink"
            />
            <UFieldGroup v-for="link in data.links" :key="link.id" class="w-full">
              <UInput
                :model-value="link.url"
                readonly
                class="min-w-0 flex-1"
                :aria-label="t('sharing.internal')"
              />
              <UButton
                icon="i-lucide-copy"
                :aria-label="t('common.copy')"
                color="neutral"
                variant="outline"
                @click="copy(link.url)"
              />
            </UFieldGroup>
          </div>
          <SharingPermissions
            :permissions="data.acl?.permissions || []"
            :busy="busy"
            @revoke="revoke"
          />
        </template>
        <SharingLinks
          v-else
          :links="data.links"
          :name="entry.name"
          :busy="busy"
          @remove="removeLink"
        />
      </template>
      <SharingForm
        v-if="data && expanded"
        v-model:busy="formBusy"
        :account="account"
        :entry="entry"
        :external="tab === 'external'"
        :policy="data.policy"
        :form-id="formId"
        @saved="saved"
      />
    </template>
    <template v-if="data" #footer>
      <UButton
        v-if="expanded"
        :label="t('common.back')"
        color="neutral"
        variant="outline"
        :disabled="busy"
        @click="expanded = false"
      />
      <UButton
        v-if="expanded"
        type="submit"
        :form="formId"
        :label="t(tab === 'internal' ? 'sharing.addRecipient' : 'sharing.createLink')"
        :loading="busy"
      />
      <UButton
        v-else
        icon="i-lucide-plus"
        :label="t(tab === 'internal' ? 'sharing.addRecipient' : 'sharing.createLink')"
        :disabled="Boolean(data.policyError) || status !== 'success'"
        @click="expanded = true"
      />
    </template>
  </UModal>
</template>

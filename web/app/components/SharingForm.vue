<script setup lang="ts">
import * as z from 'zod'
import type { Entry, Recipient, SharingResponse } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'
const props = defineProps<{
  account: string
  entry: Entry
  external: boolean
  policy: SharingResponse['policy']
  formId: string
}>()
const emit = defineEmits<{ saved: [] }>()
const parentBusy = defineModel<boolean>('busy', { default: false })
const { t } = useI18n()
const { $api } = useNuxtApp()
const feedback = useFeedback()
const confirm = useOverlay().create(ConfirmModal)
const { busy, error: actionError, run } = useApiAction()
watch(busy, (value) => {
  parentBusy.value = value
})
const identity = useId()
const search = ref('')
const delayedSearch = refDebounced(search, 300)
const selectedRecipients = ref<{ value: string; label: string; recipient: Recipient }[]>([])
const state = reactive({
  permissions: [] as string[],
  expiry: 'default',
  expiresHours: 168,
  title: props.entry.name,
  code: '',
  visits: 'default',
  limit: 10,
  verifyMobile: false
})
const {
  data: people,
  status: peopleStatus,
  error: peopleError
} = useLazyAsyncData(
  `recipients-${identity}`,
  async () => {
    if (props.external || !delayedSearch.value.trim()) return []
    return $api<Recipient[]>('/sharing/recipients', {
      query: { account: props.account, q: delayedSearch.value, limit: 50 }
    })
  },
  { default: () => [], watch: [delayedSearch] }
)
const candidates = computed(() =>
  people.value.map((person) => ({
    value: `${person.type}:${person.id}`,
    recipient: person,
    label: [person.name || person.id, person.account, t(`sharing.${person.type}`)]
      .filter(Boolean)
      .join(' · ')
  }))
)
const permissionItems = computed(() =>
  (props.policy.perm.allow || ['display', 'preview', 'download']).map((value) => ({
    label: t(`sharing.allow.${value}`),
    value,
    disabled: value === 'display'
  }))
)
const schema = computed(() =>
  z.object({
    title: z
      .string()
      .refine((value) => new TextEncoder().encode(value).length <= 255, t('common.invalid')),
    code: z
      .string()
      .refine(
        (value) =>
          !props.external ||
          (!props.policy.must_set_password && !value) ||
          /^[A-Za-z0-9~!%#$@_.-]{4,100}$/.test(value),
        t('common.invalid')
      ),
    expiresHours: z
      .number()
      .min(1 / 3600)
      .max(24 * 365 * 100),
    limit: z.number().int().min(1)
  })
)
watch(
  () => props.policy,
  (value) => {
    state.permissions = [...new Set(['display', ...(value.perm.default || [])])]
    state.verifyMobile = value.verify_mobile.force
  },
  { immediate: true }
)
async function submit() {
  const external = props.external
  if (!external && !selectedRecipients.value.length) {
    actionError.value = { code: 'invalid_input', message: t('sharing.selectRecipients') }
    return
  }
  if (
    !(await confirm.open({
      title: t(external ? 'sharing.createConfirm' : 'sharing.grantConfirm'),
      description: props.entry.name
    }).result)
  )
    return
  await run(async () => {
    const result = await $api<{ granted?: boolean; linkError?: unknown }>('/sharing/action', {
      method: 'POST',
      body: {
        account: props.account,
        path: props.entry.id,
        action: external ? 'external_link' : 'grant',
        confirm: true,
        permissions: state.permissions.length ? state.permissions : undefined,
        expiresHours:
          state.expiry === 'default'
            ? undefined
            : state.expiry === 'never'
              ? 0
              : state.expiresHours,
        recipients: external
          ? []
          : selectedRecipients.value.map((person) => ({
              id: person.recipient.id,
              type: person.recipient.type
            })),
        title: external ? state.title : '',
        code: external ? state.code : '',
        verifyMobile: external && state.verifyMobile,
        limit:
          external && state.visits !== 'default'
            ? state.visits === 'unlimited'
              ? -1
              : state.limit
            : undefined
      }
    })
    if (result?.linkError) feedback.success(t('sharing.grantPartial'))
    else feedback.success()
    emit('saved')
  })
}
</script>
<template>
  <UForm
    :id="formId"
    :state="state"
    :schema="schema"
    class="space-y-5 px-4 pb-5 sm:px-6"
    @submit="submit"
  >
    <OperationError v-if="actionError || peopleError" :error="actionError || peopleError" />
    <UFormField v-if="!external" :label="t('sharing.recipients')" required>
      <USelectMenu
        v-model="selectedRecipients"
        v-model:search-term="search"
        :items="candidates"
        multiple
        ignore-filter
        :loading="peopleStatus === 'pending'"
        :placeholder="t('sharing.searchRecipients')"
        :search-input="{ placeholder: t('sharing.searchRecipients') }"
        class="w-full"
      />
    </UFormField>
    <UFormField v-if="external" name="title" :label="t('sharing.linkTitle')">
      <UInput v-model="state.title" class="w-full" />
    </UFormField>
    <UFormField :label="t('sharing.permissions')" :description="t('sharing.policyDefault')">
      <UCheckboxGroup
        v-model="state.permissions"
        :items="permissionItems"
        orientation="horizontal"
      />
    </UFormField>
    <div class="grid gap-4 sm:grid-cols-2">
      <UFormField :label="t('sharing.expiry')">
        <USelect
          v-model="state.expiry"
          :items="[
            { label: t('common.serverDefault'), value: 'default' },
            { label: t('common.never'), value: 'never' },
            { label: t('sharing.hours'), value: 'hours' }
          ]"
          class="w-full"
        />
      </UFormField>
      <UFormField v-if="state.expiry === 'hours'" name="expiresHours" :label="t('sharing.hours')">
        <UInputNumber v-model="state.expiresHours" :min="0.001" class="w-full" />
      </UFormField>
      <UFormField
        v-if="external"
        name="code"
        :label="t('sharing.code')"
        :required="policy.must_set_password"
      >
        <UInput v-model="state.code" autocomplete="off" class="w-full" />
      </UFormField>
      <UFormField v-if="external" :label="t('sharing.visits')">
        <USelect
          v-model="state.visits"
          :items="[
            { label: t('common.serverDefault'), value: 'default' },
            { label: t('sharing.unlimited'), value: 'unlimited' },
            { label: t('sharing.visits'), value: 'limited' }
          ]"
          class="w-full"
        />
      </UFormField>
      <UFormField
        v-if="external && state.visits === 'limited'"
        name="limit"
        :label="t('sharing.visits')"
      >
        <UInputNumber v-model="state.limit" :min="1" class="w-full" />
      </UFormField>
    </div>
    <UCheckbox
      v-if="external && policy.verify_mobile.enable"
      v-model="state.verifyMobile"
      :disabled="policy.verify_mobile.force"
      :label="t('sharing.verifyMobile')"
    />
  </UForm>
</template>

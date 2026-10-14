<script setup lang="ts">
import * as z from 'zod'
import type { AuthFormField, FormSubmitEvent } from '@nuxt/ui'
import type { LoginAttempt } from '~/types/api'

const workspace = useWorkspaceStore()
const { $api } = useNuxtApp()
const { t } = useI18n()
const feedback = useFeedback()
const attempt = ref<LoginAttempt | null>(null)
const error = ref<unknown>(null)
const sending = ref(false)
const answer = ref('')
const makeDefault = ref(false)
const replace = ref(false)
const form = useTemplateRef('authForm')
const schema = computed(() =>
  z.object({
    username: z.string().trim().min(1, t('common.required')),
    password: z.string().min(1, t('common.required')).max(8192, t('common.invalid')),
    name: z.string().optional(),
    server: z.url(t('common.invalid'))
  })
)
type LoginFields = { username: string; password: string; name?: string; server: string }
const fields = computed<AuthFormField[]>(() => [
  {
    name: 'username',
    type: 'text',
    label: t('accounts.username'),
    required: true,
    autocomplete: 'username',
    defaultValue: workspace.loginAccount?.username || ''
  },
  {
    name: 'password',
    type: 'password',
    label: t('accounts.password'),
    required: true,
    autocomplete: 'current-password'
  },
  {
    name: 'name',
    type: 'text',
    label: t('accounts.label'),
    placeholder: t('accounts.labelHint'),
    defaultValue: workspace.loginAccount?.name || ''
  },
  {
    name: 'server',
    type: 'text',
    label: t('accounts.server'),
    defaultValue: workspace.loginAccount?.server || 'https://disk.pku.edu.cn'
  }
])
const working = computed(
  () => attempt.value && ['connecting', 'challenge'].includes(attempt.value.state)
)
const challengeLabel = computed(() => {
  const kind = attempt.value?.challenge?.kind.toLowerCase() || ''
  return t(['captcha', 'otp', 'sms'].includes(kind) ? `accounts.${kind}` : 'accounts.answer')
})

const poll = useIntervalFn(
  async () => {
    if (!attempt.value) return
    const id = attempt.value.id
    try {
      const result = await $api<LoginAttempt>(`/logins/${id}`)
      if (!workspace.loginOpen || attempt.value?.id !== id) return
      attempt.value = result
      if (attempt.value.state === 'completed') {
        poll.pause()
        await workspace.refreshAccounts()
        workspace.selected = attempt.value.account
        workspace.loginOpen = false
        feedback.success()
      } else if (['failed', 'cancelled'].includes(attempt.value.state)) {
        poll.pause()
        error.value = attempt.value.error
      }
    } catch (err) {
      poll.pause()
      error.value = err
    }
  },
  700,
  { immediate: false }
)

async function start(body: Record<string, unknown>) {
  error.value = null
  sending.value = true
  try {
    attempt.value = await $api<LoginAttempt>('/logins', { method: 'POST', body })
    poll.resume()
  } catch (err) {
    error.value = err
  } finally {
    sending.value = false
  }
}

async function submit(event: FormSubmitEvent<LoginFields>) {
  const request = start({ ...event.data, replace: replace.value, default: makeDefault.value })
  if (form.value?.state) form.value.state.password = ''
  await request
}

async function verify() {
  if (!attempt.value?.challenge || !answer.value) return
  sending.value = true
  try {
    await $api(`/logins/${attempt.value.id}/answer`, {
      method: 'POST',
      body: { challenge: attempt.value.challenge.id, answer: answer.value }
    })
    answer.value = ''
    attempt.value = { ...attempt.value, state: 'connecting', challenge: undefined }
  } catch (err) {
    error.value = err
  } finally {
    sending.value = false
  }
}

async function cancelAttempt() {
  poll.pause()
  if (attempt.value && working.value)
    await $api(`/logins/${attempt.value.id}`, { method: 'DELETE' }).catch(() => {})
}

watch(
  () => workspace.loginOpen,
  (open) => {
    if (!open) {
      void cancelAttempt()
      return
    }
    attempt.value = null
    error.value = null
    answer.value = ''
    makeDefault.value = workspace.accounts.length === 0
    replace.value = Boolean(workspace.loginAccount)
    if (workspace.restoreLogin && workspace.loginAccount)
      void start({ account: workspace.loginAccount.name })
  }
)
onBeforeUnmount(() => {
  void cancelAttempt()
})
</script>

<template>
  <UModal
    v-model:open="workspace.loginOpen"
    :title="t('accounts.loginTitle')"
    :description="t('accounts.loginHint')"
    scrollable
  >
    <template #body>
      <OperationError v-if="error" :error="error" class="mb-5" />
      <div v-if="working" class="space-y-5">
        <template v-if="attempt?.challenge">
          <div>
            <h3 class="text-highlighted font-medium">
              {{ t('accounts.verify') }}
            </h3>
            <p class="text-muted mt-1 text-sm">
              {{ t('accounts.verifyHint') }}
            </p>
          </div>
          <img
            v-if="attempt.challenge.image"
            :src="attempt.challenge.image"
            :alt="t('accounts.captcha')"
            class="max-h-36 rounded-lg"
          />
          <UForm :state="{ answer }" @submit="verify">
            <UFormField :label="challengeLabel" name="answer" required>
              <UInput v-model="answer" autocomplete="one-time-code" autofocus class="w-full" />
            </UFormField>
            <UButton
              type="submit"
              :label="t('accounts.verifySubmit')"
              :loading="sending"
              :disabled="!answer"
              class="mt-4"
              block
            />
          </UForm>
        </template>
        <template v-else>
          <p role="status">
            {{ t('accounts.connecting') }}
          </p>
          <UProgress />
        </template>
        <UButton
          :label="t('common.cancel')"
          variant="outline"
          color="neutral"
          block
          @click="workspace.loginOpen = false"
        />
      </div>
      <UAuthForm
        v-else
        ref="authForm"
        :fields="fields"
        :schema="schema"
        :loading="sending"
        :submit="{ label: t('accounts.login'), block: true }"
        @submit="submit"
      >
        <template #footer>
          <div class="space-y-3">
            <UCheckbox v-model="makeDefault" :label="t('accounts.makeDefault')" />
            <UCheckbox v-model="replace" :label="t('accounts.replace')" />
          </div>
        </template>
      </UAuthForm>
    </template>
  </UModal>
</template>

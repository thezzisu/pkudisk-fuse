<script setup lang="ts">
import * as z from 'zod'
import type { Entry } from '~/types/api'
import RemoteFolderModal from '~/components/RemoteFolderModal.vue'

const props = defineProps<{ action: 'mkdir' | 'rename' | 'move'; account: string; entry: Entry }>()
const emit = defineEmits<{ close: [changed: boolean] }>()
const { t } = useI18n()
const { $api } = useNuxtApp()
const picker = useOverlay().create(RemoteFolderModal)
const state = reactive({ name: props.action === 'rename' ? props.entry.name : '', destination: '' })
const { error, busy, run } = useApiAction({ success: true })
const formId = useId()
const title = computed(() =>
  props.action === 'mkdir'
    ? t('files.newFolder')
    : t(`files.${props.action}Title`, { name: props.entry.name })
)
const schema = computed(() =>
  props.action === 'move'
    ? z.object({ destination: z.string().min(1, t('common.required')) })
    : z.object({
        name: z
          .string()
          .min(1, t('common.required'))
          .refine(
            (name) =>
              !name.includes('/') && !name.includes('\\') && !name.includes(String.fromCharCode(0)),
            t('common.invalid')
          )
      })
)

async function destination() {
  const selected = await picker.open({ account: props.account }).result
  if (selected) state.destination = selected.path
}
async function submit() {
  await run(async () => {
    await $api('/files/action', {
      method: 'POST',
      body: { account: props.account, path: props.entry.id, action: props.action, ...state }
    })
    emit('close', true)
  })
}
</script>

<template>
  <UModal :title="title" :description="entry.name" :close="{ onClick: () => emit('close', false) }">
    <template #body>
      <UForm :id="formId" :state="state" :schema="schema" class="space-y-5" @submit="submit">
        <OperationError v-if="error" :error="error" />
        <UFormField v-if="action !== 'move'" name="name" :label="t('files.newName')" required>
          <UInput v-model="state.name" autofocus class="w-full" />
        </UFormField>
        <UFormField v-else name="destination" :label="t('files.destination')" required>
          <UFieldGroup class="w-full">
            <UInput
              :model-value="state.destination"
              readonly
              class="min-w-0 flex-1"
              @click="destination"
            /><UButton
              :label="t('common.browse')"
              color="neutral"
              variant="outline"
              @click="destination"
            />
          </UFieldGroup>
        </UFormField>
      </UForm>
    </template>
    <template #footer>
      <div class="flex justify-end gap-2">
        <UButton
          :label="t('common.cancel')"
          color="neutral"
          variant="outline"
          @click="emit('close', false)"
        /><UButton :label="t('common.save')" type="submit" :form="formId" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>

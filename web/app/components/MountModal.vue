<script setup lang="ts">
import * as z from 'zod'
import RemoteFolderModal from '~/components/RemoteFolderModal.vue'
import LocalPickerModal from '~/components/LocalPickerModal.vue'

const props = defineProps<{ account: string; platform: string }>()
const emit = defineEmits<{ close: [started: boolean] }>()
const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const picker = useOverlay().create(RemoteFolderModal)
const localPicker = useOverlay().create(LocalPickerModal)
const state = reactive({
  point: '',
  root: '',
  allAccounts: false,
  readOnly: false,
  cache: workspace.cache,
  cacheLimit: '10GiB',
  partSize: '16MiB',
  wholeFile: '16MiB',
  blockSize: '4MiB',
  readAhead: '16MiB',
  concurrency: 4,
  ttlSeconds: 2,
  refreshSeconds: 3600,
  timeoutSeconds: 600,
  verbose: false,
  debug: false
})
const schema = computed(() =>
  z.object({
    point: z.string().min(1, t('common.required')),
    concurrency: z.number().int().min(1).max(64),
    ttlSeconds: z.number().int().min(0).max(86400),
    refreshSeconds: z.number().int().min(0).max(86400),
    timeoutSeconds: z.number().int().min(1).max(86400)
  })
)
const sizeFields = ['cacheLimit', 'partSize', 'wholeFile', 'blockSize', 'readAhead'] as const
const { error, busy, run } = useApiAction({ success: true })
const formId = useId()
const rootLabel = ref('')
const rootDisplay = computed(() => rootLabel.value || state.root)
async function local(field: 'point' | 'cache') {
  const selected = await localPicker.open({
    initial: state[field],
    kind: field === 'point' ? 'mount' : 'directory',
    title: t(field === 'point' ? 'mounts.point' : 'mounts.cache')
  }).result
  if (selected) state[field] = selected.path
}

async function remote() {
  const selected = await picker.open({ account: props.account }).result
  if (selected) {
    state.root = selected.path
    rootLabel.value = selected.name
  }
}
async function submit() {
  await run(async () => {
    await $api('/mounts', {
      method: 'POST',
      body: {
        ...state,
        account: state.allAccounts ? '' : props.account,
        root: state.allAccounts ? '' : state.root
      }
    })
    await workspace.refreshTasks()
    emit('close', true)
  })
}
</script>

<template>
  <UModal
    :title="t('mounts.add')"
    :description="t('mounts.description')"
    :close="{ onClick: () => emit('close', false) }"
  >
    <template #body>
      <UForm :id="formId" :state="state" :schema="schema" class="space-y-5" @submit="submit">
        <OperationError v-if="error" :error="error" />
        <UFormField name="point" :label="t('mounts.point')" required>
          <UFieldGroup class="w-full">
            <UInput
              :model-value="state.point"
              readonly
              class="min-w-0 flex-1"
              :placeholder="t('localPicker.title')"
              @click="local('point')"
            />
            <UButton
              icon="i-lucide-folder-search"
              :label="t('common.browse')"
              color="neutral"
              variant="outline"
              @click="local('point')"
            />
          </UFieldGroup>
        </UFormField>
        <UCheckbox
          v-model="state.allAccounts"
          :label="t('mounts.allAccounts')"
          :description="t('mounts.allHint')"
        />
        <UFormField
          v-if="!state.allAccounts"
          :label="t('mounts.root')"
          :description="t('mounts.rootHint')"
        >
          <UFieldGroup class="w-full">
            <UInput
              :model-value="rootDisplay"
              readonly
              class="min-w-0 flex-1"
              @click="remote"
            /><UButton
              :label="t('common.browse')"
              color="neutral"
              variant="outline"
              @click="remote"
            />
          </UFieldGroup>
        </UFormField>
        <UCheckbox v-model="state.readOnly" :label="t('mounts.readOnly')" />
        <UCollapsible>
          <UButton
            :label="t('common.advanced')"
            trailing-icon="i-lucide-chevron-down"
            color="neutral"
            variant="ghost"
          /><template #content>
            <div class="mt-4 space-y-4">
              <UFormField :label="t('mounts.cache')">
                <UFieldGroup class="w-full">
                  <UInput
                    :model-value="state.cache"
                    readonly
                    class="min-w-0 flex-1"
                    @click="local('cache')"
                  />
                  <UButton
                    icon="i-lucide-folder-search"
                    :label="t('common.browse')"
                    color="neutral"
                    variant="outline"
                    @click="local('cache')"
                  />
                </UFieldGroup>
              </UFormField>
              <p class="text-muted text-xs">
                {{ t('mounts.unitsHint') }}
              </p>
              <div class="grid gap-4 sm:grid-cols-2">
                <UFormField
                  v-for="field in sizeFields"
                  :key="field"
                  :name="field"
                  :label="t(`mounts.${field}`)"
                >
                  <UInput v-model="state[field]" class="w-full" />
                </UFormField>
              </div>
              <div class="grid gap-4 sm:grid-cols-2">
                <UFormField name="concurrency" :label="t('mounts.concurrency')">
                  <UInputNumber v-model="state.concurrency" :min="1" :max="64" class="w-full" />
                </UFormField>
                <UFormField name="ttlSeconds" :label="t('mounts.ttl')">
                  <UInputNumber v-model="state.ttlSeconds" :min="0" :max="86400" class="w-full" />
                </UFormField>
                <UFormField name="refreshSeconds" :label="t('mounts.refresh')">
                  <UInputNumber
                    v-model="state.refreshSeconds"
                    :min="0"
                    :max="86400"
                    class="w-full"
                  />
                </UFormField>
                <UFormField name="timeoutSeconds" :label="t('common.timeout')">
                  <UInputNumber
                    v-model="state.timeoutSeconds"
                    :min="1"
                    :max="86400"
                    class="w-full"
                  />
                </UFormField>
              </div>
              <UCheckbox v-model="state.verbose" :label="t('mounts.verbose')" /><UCheckbox
                v-model="state.debug"
                :label="t('mounts.debug')"
              />
            </div>
          </template>
        </UCollapsible>
      </UForm>
    </template>
    <template #footer>
      <div class="flex justify-end gap-2">
        <UButton
          :label="t('common.cancel')"
          color="neutral"
          variant="outline"
          @click="emit('close', false)"
        /><UButton :label="t('mounts.start')" type="submit" :form="formId" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>

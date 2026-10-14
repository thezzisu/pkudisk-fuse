<script setup lang="ts">
import * as z from 'zod'
import { fileOpen, directoryOpen } from 'browser-fs-access'
import type { Entry } from '~/types/api'
import RemoteFolderModal from '~/components/RemoteFolderModal.vue'
import LocalPickerModal from '~/components/LocalPickerModal.vue'

const props = defineProps<{
  kind: 'upload' | 'download' | 'copy' | 'extract'
  account: string
  entry?: Entry
  folder?: Entry | null
  initialDestination?: string
  recoverySource?: string
}>()
const emit = defineEmits<{ close: [started: boolean] }>()
const { t } = useI18n()
const { $api } = useNuxtApp()
const workspace = useWorkspaceStore()
const browser = useBrowserTransfersStore()
const feedback = useFeedback()
const folderPicker = useOverlay().create(RemoteFolderModal)
const localPicker = useOverlay().create(LocalPickerModal)
const localSource = ref('')
const localSourceDirectory = ref('')
const localDestination = ref('')
const files = shallowRef<File[]>([])
const state = reactive({
  destination: props.initialDestination || props.folder?.id || '',
  overwrite: false,
  fast: false,
  mode: 'browser',
  timeoutSeconds: 600
})
const destinationLabel = ref(props.folder?.name || props.initialDestination || '')
const destinationDisplay = computed(() => destinationLabel.value || state.destination)
const error = ref<unknown>(null)
const busy = ref(false)
const formId = useId()
const schema = computed(() =>
  z.object({
    destination: props.kind === 'download' ? z.string() : z.string().min(1, t('common.required')),
    timeoutSeconds: z.number().int().min(1).max(86400)
  })
)
async function selectFiles(directory = false) {
  try {
    const selected = directory
      ? await directoryOpen({ recursive: true })
      : await fileOpen({ multiple: true })
    localSource.value = ''
    files.value = [...files.value, ...selected]
  } catch (err) {
    if (!(err instanceof DOMException && err.name === 'AbortError')) error.value = err
  }
}
async function remote() {
  const folder = await folderPicker.open({ account: props.account, initial: props.folder?.id })
    .result
  if (folder) {
    state.destination = folder.path
    destinationLabel.value = folder.name
  }
}
async function local() {
  const selected = await localPicker.open({
    initial: props.kind === 'upload' ? localSourceDirectory.value : localDestination.value,
    kind: props.kind === 'upload' ? 'any' : 'directory',
    title: t(props.kind === 'upload' ? 'files.source' : 'files.destination')
  }).result
  if (!selected) return
  if (props.kind === 'upload') {
    localSource.value = selected.path
    localSourceDirectory.value = selected.dir
      ? selected.path
      : selected.path.slice(0, -selected.name.length)
    files.value = []
  } else {
    localDestination.value = selected.path
    if (state.mode === 'browser') state.mode = 'direct'
  }
}
function useBrowserDownload() {
  localDestination.value = ''
  state.mode = 'browser'
}
async function queueLocal() {
  await $api('/tasks', {
    method: 'POST',
    body: {
      kind: props.recoverySource ? 'recover' : props.kind,
      account: props.account,
      source:
        props.recoverySource || (props.kind === 'upload' ? localSource.value : props.entry?.id),
      destination: props.kind === 'upload' ? state.destination : localDestination.value,
      mode: props.kind === 'download' ? state.mode : undefined,
      overwrite: state.overwrite,
      fast: state.fast,
      timeoutSeconds: state.timeoutSeconds
    }
  })
  await workspace.refreshTasks()
}
async function submit() {
  busy.value = true
  error.value = null
  try {
    if (
      props.recoverySource ||
      (props.kind === 'upload' && localSource.value) ||
      (props.kind === 'download' && localDestination.value)
    ) {
      await queueLocal()
    } else if (props.kind === 'upload') {
      if (!files.value.length) {
        error.value = { code: 'invalid_input', message: t('files.selectFiles') }
        return
      }
      browser.upload(files.value, {
        account: props.account,
        destination: state.destination,
        overwrite: state.overwrite,
        fast: state.fast,
        timeoutSeconds: state.timeoutSeconds
      })
    } else if (props.kind === 'download' && props.entry) {
      await browser.nativeDownload(props.account, props.entry, state.timeoutSeconds)
    } else {
      await $api('/tasks', {
        method: 'POST',
        body: {
          kind: props.kind,
          account: props.account,
          source: props.entry?.id,
          destination: state.destination,
          overwrite: state.overwrite,
          fast: state.fast,
          timeoutSeconds: state.timeoutSeconds
        }
      })
      await workspace.refreshTasks()
    }
    feedback.success(
      t(
        props.kind === 'download' && state.mode === 'browser' && !localDestination.value
          ? 'files.browserDownloadStarted'
          : 'files.transferQueued'
      )
    )
    emit('close', true)
  } catch (err) {
    if (!(err instanceof DOMException && err.name === 'AbortError')) error.value = err
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UModal
    :title="t(`tasks.kinds.${kind}`)"
    :description="`${account} · ${entry?.name || t('files.selectFiles')}`"
    :close="{ onClick: () => emit('close', false) }"
  >
    <template #body>
      <UForm :id="formId" :state="state" :schema="schema" class="space-y-5" @submit="submit">
        <OperationError v-if="error" :error="error" />
        <template v-if="kind === 'upload' && !recoverySource">
          <UFormField name="files" :label="t('files.source')" required>
            <UFileUpload
              v-if="!localSource"
              v-model="files"
              multiple
              layout="list"
              :file-image="false"
              :interactive="true"
              :label="t('files.dropFiles')"
              :description="t('files.browserUploadHint')"
              class="w-full"
              :ui="{ files: 'max-h-52 overflow-y-auto' }"
            >
              <template #actions>
                <UButton
                  icon="i-lucide-file-plus"
                  :label="t('files.localFiles')"
                  color="neutral"
                  variant="outline"
                  @click.stop="selectFiles()"
                />
                <UButton
                  icon="i-lucide-folder-plus"
                  :label="t('files.localFolder')"
                  color="neutral"
                  variant="outline"
                  @click.stop="selectFiles(true)"
                />
              </template>
            </UFileUpload>
            <div v-else class="bg-muted flex items-center gap-2 rounded-md px-3 py-2">
              <UIcon name="i-lucide-file" class="size-4 shrink-0" />
              <span class="min-w-0 flex-1 break-all text-sm">{{ localSource }}</span>
              <UButton
                icon="i-lucide-x"
                :aria-label="t('common.cancel')"
                color="neutral"
                variant="ghost"
                @click="localSource = ''"
              />
            </div>
            <UButton
              v-if="workspace.flags.canBrowseLocalFS"
              icon="i-lucide-hard-drive"
              :label="t('localPicker.backgroundUpload')"
              color="neutral"
              variant="outline"
              class="mt-3"
              @click="local"
            />
          </UFormField>
        </template>
        <div
          v-else
          class="bg-muted text-highlighted flex items-center gap-2 rounded-md px-3 py-2 text-sm"
        >
          <UIcon
            :name="entry?.directory ? 'i-lucide-folder' : 'i-lucide-file'"
            class="size-4 shrink-0"
          />
          <span class="break-all">{{ entry?.name || t('files.selectFiles') }}</span>
        </div>
        <UFormField
          v-if="kind !== 'download'"
          name="destination"
          :label="t('files.destination')"
          required
        >
          <UFieldGroup class="w-full">
            <UInput
              :model-value="destinationDisplay"
              readonly
              class="min-w-0 flex-1"
              @click="remote"
            />
            <UButton
              icon="i-lucide-folder-search"
              :label="t('common.browse')"
              color="neutral"
              variant="outline"
              @click="remote"
            />
          </UFieldGroup>
        </UFormField>
        <template v-else>
          <UFormField
            v-if="entry?.directory && localDestination"
            :label="t('files.downloadMode')"
            name="mode"
          >
            <USelect
              v-model="state.mode"
              :items="[
                { label: t('files.archive'), value: 'archive' },
                {
                  label: t('files.direct'),
                  value: 'direct'
                },
                {
                  label: t('files.viaArchive'),
                  value: 'viaArchive'
                }
              ]"
              class="w-full"
            />
          </UFormField>
          <div v-if="workspace.flags.canBrowseLocalFS" class="space-y-2">
            <UFieldGroup v-if="localDestination" class="w-full">
              <UInput
                :model-value="localDestination"
                readonly
                class="min-w-0 flex-1"
                @click="local"
              />
              <UButton
                :label="t('common.browse')"
                icon="i-lucide-folder-search"
                color="neutral"
                variant="outline"
                @click="local"
              />
              <UButton
                icon="i-lucide-x"
                :aria-label="t('localPicker.browser')"
                color="neutral"
                variant="outline"
                @click="useBrowserDownload"
              />
            </UFieldGroup>
            <UButton
              v-else
              :label="t('localPicker.backgroundDownload')"
              icon="i-lucide-hard-drive"
              color="neutral"
              variant="outline"
              @click="local"
            />
          </div>
          <p class="text-muted text-sm">
            {{ t(localDestination ? 'localPicker.backgroundHint' : 'files.browserDownloadHint') }}
          </p>
        </template>
        <UCheckbox
          v-if="kind === 'upload' || (kind === 'download' && state.mode !== 'browser')"
          v-model="state.overwrite"
          :label="t('files.overwrite')"
          :description="t('files.overwriteHint')"
        />
        <UCheckbox
          v-if="kind === 'upload'"
          v-model="state.fast"
          :label="t('files.fast')"
          :description="t('files.fastHint')"
        />
        <UAlert
          v-if="kind === 'extract' || kind === 'copy'"
          color="neutral"
          variant="subtle"
          :description="t(kind === 'extract' ? 'files.extractHint' : 'files.copyHint')"
        />
        <UFormField name="timeoutSeconds" :label="t('common.timeout')">
          <UInputNumber v-model="state.timeoutSeconds" :min="1" :max="86400" class="w-full" />
        </UFormField>
      </UForm>
    </template>
    <template #footer>
      <UButton
        :label="t('common.cancel')"
        color="neutral"
        variant="outline"
        @click="emit('close', false)"
      />
      <UButton type="submit" :form="formId" :label="t('files.startTransfer')" :loading="busy" />
    </template>
  </UModal>
</template>

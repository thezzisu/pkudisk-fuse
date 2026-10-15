<script setup lang="ts">
import * as z from 'zod'
import { directoryOpen } from 'browser-fs-access'
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
const format = useFormatting()
const isMobile = computed(() => workspace.flags.isMobile)
const canBrowseLocal = computed(() => !isMobile.value && workspace.flags.canBrowseLocalFS)
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
const ready = computed(
  () =>
    props.kind !== 'upload' || !!props.recoverySource || !!localSource.value || !!files.value.length
)
const hasAdvanced = computed(() => props.kind !== 'download' || canBrowseLocal.value)
const schema = computed(() =>
  z.object({
    destination: props.kind === 'download' ? z.string() : z.string().min(1, t('common.required')),
    timeoutSeconds: z.number().int().min(1).max(86400)
  })
)
async function selectDirectory() {
  try {
    const selected = await directoryOpen({ recursive: true })
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
  if (!ready.value) return
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
    :description="`${t('common.account')}: ${account}`"
    :close="{ onClick: () => emit('close', false) }"
    :ui="{ footer: 'justify-end' }"
  >
    <template #body>
      <UForm :id="formId" :state="state" :schema="schema" class="space-y-5" @submit="submit">
        <OperationError v-if="error" :error="error" />
        <UFormField
          v-if="kind === 'upload' && !recoverySource"
          name="files"
          :label="t('files.uploadFiles')"
          required
        >
          <UFileUpload
            v-if="!localSource"
            v-model="files"
            multiple
            layout="list"
            :file-image="false"
            :dropzone="!isMobile"
            :interactive="false"
            :icon="isMobile ? false : 'i-lucide-upload'"
            :label="isMobile ? undefined : t('files.dropFiles')"
            class="w-full"
            :ui="{
              files: 'max-h-48 overflow-y-auto',
              ...(isMobile ? { base: 'border-0 p-0', wrapper: 'p-0', actions: 'mt-0 w-full' } : {})
            }"
          >
            <template #file-leading="{ file }">
              <FileIcon :name="file.name" />
            </template>
            <template #actions="{ open }">
              <UButton
                icon="i-lucide-file-plus"
                :label="t('files.localFiles')"
                :block="isMobile"
                color="neutral"
                variant="outline"
                @click="open()"
              />
              <UButton
                v-if="!isMobile"
                icon="i-lucide-folder-plus"
                :label="t('files.localFolder')"
                color="neutral"
                variant="outline"
                @click="selectDirectory"
              />
            </template>
          </UFileUpload>
          <div v-else class="flex items-center gap-2">
            <UButton
              icon="i-lucide-hard-drive"
              :label="localSource"
              color="neutral"
              variant="outline"
              class="min-w-0 flex-1"
              :ui="{ label: 'truncate' }"
              @click="local"
            />
            <UButton
              icon="i-lucide-x"
              :aria-label="t('common.remove')"
              color="neutral"
              variant="ghost"
              @click="localSource = ''"
            />
          </div>
          <p class="text-muted mt-2 text-xs">
            {{
              t(
                localSource
                  ? 'localPicker.backgroundHint'
                  : isMobile
                    ? 'files.mobileUploadHint'
                    : 'files.browserUploadHint'
              )
            }}
          </p>
        </UFormField>
        <div v-else class="flex items-center gap-3">
          <FileIcon :name="entry?.name || ''" :directory="entry?.directory" class="size-6" />
          <div class="min-w-0">
            <p class="text-highlighted break-all text-sm font-medium">
              {{ entry?.name || t('files.selectFiles') }}
            </p>
            <p v-if="entry && !entry.directory" class="text-muted text-xs">
              {{ format.bytes(entry.size) }}
            </p>
          </div>
        </div>
        <UFormField
          v-if="kind !== 'download'"
          name="destination"
          :label="t('files.destination')"
          required
        >
          <UButton
            icon="i-lucide-folder"
            trailing-icon="i-lucide-chevron-right"
            :label="destinationDisplay || t('files.selectDestination')"
            :aria-label="`${t('files.destination')}: ${destinationDisplay || t('common.choose')}`"
            color="neutral"
            variant="outline"
            class="w-full justify-start"
            :ui="{ label: 'truncate', trailingIcon: 'ms-auto' }"
            @click="remote"
          />
        </UFormField>
        <template v-else>
          <UFormField v-if="localDestination" :label="t('files.destination')">
            <UButton
              icon="i-lucide-folder"
              trailing-icon="i-lucide-chevron-right"
              :label="localDestination"
              color="neutral"
              variant="outline"
              class="w-full justify-start"
              :ui="{ label: 'truncate', trailingIcon: 'ms-auto' }"
              @click="local"
            />
          </UFormField>
          <p class="text-muted text-sm">
            {{
              t(
                localDestination
                  ? 'localPicker.backgroundHint'
                  : entry?.directory
                    ? isMobile
                      ? 'files.mobileFolderDownloadHint'
                      : 'files.browserFolderDownloadHint'
                    : isMobile
                      ? 'files.mobileDownloadHint'
                      : 'files.browserDownloadHint'
              )
            }}
          </p>
        </template>
        <UAlert
          v-if="kind === 'extract' || kind === 'copy'"
          color="neutral"
          variant="subtle"
          :description="t(kind === 'extract' ? 'files.extractHint' : 'files.copyHint')"
        />
        <UCollapsible v-if="hasAdvanced">
          <UButton
            :label="t('common.advanced')"
            trailing-icon="i-lucide-chevron-down"
            color="neutral"
            variant="ghost"
            class="px-0"
          />
          <template #content>
            <div class="mt-3 space-y-4">
              <UButton
                v-if="canBrowseLocal && !recoverySource && kind === 'upload'"
                icon="i-lucide-hard-drive"
                :label="t('localPicker.backgroundUpload')"
                color="neutral"
                variant="outline"
                @click="local"
              />
              <UButton
                v-if="canBrowseLocal && kind === 'download'"
                :icon="localDestination ? 'i-lucide-globe' : 'i-lucide-hard-drive'"
                :label="
                  t(localDestination ? 'localPicker.browser' : 'localPicker.backgroundDownload')
                "
                color="neutral"
                variant="outline"
                @click="localDestination ? useBrowserDownload() : local()"
              />
              <UFormField
                v-if="kind === 'download' && entry?.directory && localDestination"
                :label="t('files.downloadMode')"
                name="mode"
              >
                <USelect
                  v-model="state.mode"
                  :items="[
                    { label: t('files.direct'), value: 'direct' },
                    { label: t('files.archive'), value: 'archive' },
                    { label: t('files.viaArchive'), value: 'viaArchive' }
                  ]"
                  class="w-full"
                />
              </UFormField>
              <UCheckbox
                v-if="kind === 'upload' || (kind === 'download' && localDestination)"
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
              <UFormField
                v-if="kind !== 'download' || localDestination"
                name="timeoutSeconds"
                :label="t('common.timeout')"
              >
                <UInputNumber v-model="state.timeoutSeconds" :min="1" :max="86400" class="w-full" />
              </UFormField>
            </div>
          </template>
        </UCollapsible>
      </UForm>
    </template>
    <template #footer>
      <UButton
        :label="t('common.cancel')"
        color="neutral"
        variant="outline"
        @click="emit('close', false)"
      />
      <UButton
        type="submit"
        :form="formId"
        :label="t(`tasks.kinds.${kind}`)"
        :loading="busy"
        :disabled="!ready"
      />
    </template>
  </UModal>
</template>

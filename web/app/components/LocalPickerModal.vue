<script setup lang="ts">
import type { LocalEntry, LocalListing, LocalRoot } from '~/types/api'

const props = withDefaults(
  defineProps<{
    initial?: string
    kind?: 'directory' | 'file' | 'any' | 'mount'
    title?: string
  }>(),
  { initial: '', kind: 'directory', title: '' }
)
const emit = defineEmits<{ close: [value: { path: string; name: string; dir: boolean } | null] }>()
const { $api } = useNuxtApp()
const { t } = useI18n()
const format = useFormatting()
const columns = computed(() => [
  { accessorKey: 'name', header: t('common.name') },
  {
    accessorKey: 'size',
    header: t('common.size'),
    meta: { class: { th: 'hidden w-40 md:table-cell', td: 'hidden md:table-cell' } }
  }
])
// An unused drive letter is a mount target, not a browsable directory.
const path = ref(props.kind === 'mount' && /^[A-Za-z]:$/.test(props.initial) ? '' : props.initial)
const filter = ref('')
const showHidden = ref(false)
const selected = ref<{ path: string; name: string; dir: boolean } | null>(null)
const newFolder = ref('')
const createOpen = ref(false)
const creating = ref(false)
const createError = ref<unknown>(null)
const { data, status, error, refresh } = useAsyncData(
  `local-picker-${useId()}`,
  () => $api<LocalListing>('/local-fs', { query: { path: path.value } }),
  { watch: [path] }
)
const entries = computed(() =>
  (data.value?.entries || []).filter(
    (entry) =>
      (props.kind === 'any' || props.kind === 'file' || entry.dir) &&
      (showHidden.value || !entry.name.startsWith('.')) &&
      entry.name.toLocaleLowerCase().includes(filter.value.toLocaleLowerCase())
  )
)
const roots = computed(() => data.value?.roots.filter((root) => !root.free) || [])
const freeDrives = computed(
  () =>
    data.value?.roots
      .filter((root) => root.free)
      .map((root) => ({ label: root.name, value: root.path })) || []
)
const value = computed(
  () =>
    selected.value ||
    (data.value
      ? {
          path: data.value.path,
          dir: true,
          name: data.value.path.split(/[\\/]/).filter(Boolean).at(-1) || data.value.path
        }
      : null)
)
const canChoose = computed(
  () =>
    status.value === 'success' &&
    !!value.value &&
    (selected.value || (props.kind !== 'file' && (props.kind !== 'mount' || data.value?.empty)))
)
function navigate(next: string) {
  selected.value = null
  filter.value = ''
  path.value = next
}
function open(entry: LocalEntry) {
  if (entry.dir) navigate(entry.path)
  else selected.value = entry
}
function rootLabel(root: LocalRoot) {
  return root.kind === 'drive' ? root.name : t(`localPicker.roots.${root.kind}`)
}
function selectDrive(path: string) {
  selected.value = { path: path.replace(/\\$/, ''), name: path.replace(/\\$/, ''), dir: true }
}
async function mkdir() {
  if (!data.value || !newFolder.value.trim()) return
  creating.value = true
  createError.value = null
  try {
    const created = await $api<{ path: string }>('/local-fs/mkdir', {
      method: 'POST',
      body: { path: data.value.path, name: newFolder.value.trim() }
    })
    createOpen.value = false
    newFolder.value = ''
    navigate(created.path)
  } catch (err) {
    createError.value = err
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <UModal
    :title="title || t('localPicker.title')"
    :description="t('localPicker.description')"
    :close="{ onClick: () => emit('close', null) }"
    :ui="{ content: 'sm:max-w-3xl', body: 'p-0 sm:p-0', footer: 'justify-end' }"
  >
    <template #body>
      <div class="space-y-3 px-4 py-3 sm:px-6">
        <div class="flex flex-wrap gap-1">
          <UButton
            v-for="root in roots"
            :key="`${root.kind}:${root.path}`"
            :label="rootLabel(root)"
            :title="root.path"
            :disabled="!root.available"
            :icon="
              root.kind === 'drive' || root.kind === 'filesystem'
                ? 'i-lucide-hard-drive'
                : 'i-lucide-folder'
            "
            color="neutral"
            variant="ghost"
            size="sm"
            @click="navigate(root.path)"
          />
        </div>
        <USelect
          v-if="kind === 'mount' && freeDrives.length"
          :items="freeDrives"
          :placeholder="t('localPicker.freeDrive')"
          :aria-label="t('localPicker.freeDrive')"
          class="w-full"
          @update:model-value="selectDrive"
        />
        <div class="flex items-center gap-2">
          <UButton
            v-if="!roots.some((root) => root.kind === 'home')"
            icon="i-lucide-house"
            :aria-label="t('localPicker.roots.home')"
            color="neutral"
            variant="ghost"
            @click="navigate('')"
          />
          <UButton
            icon="i-lucide-arrow-up"
            :aria-label="t('files.up')"
            :disabled="!data || data.path === data.parent || status === 'pending'"
            color="neutral"
            variant="outline"
            @click="navigate(data!.parent)"
          />
          <p class="text-muted min-w-0 flex-1 break-all text-sm">{{ data?.path || path }}</p>
          <UButton
            icon="i-lucide-refresh-cw"
            :aria-label="t('common.refresh')"
            :loading="status === 'pending'"
            color="neutral"
            variant="ghost"
            @click="refresh()"
          />
          <UPopover v-model:open="createOpen">
            <UButton
              icon="i-lucide-folder-plus"
              :label="t('localPicker.newFolder')"
              :disabled="!data || status !== 'success'"
              color="neutral"
              variant="outline"
            />
            <template #content>
              <form class="w-72 space-y-3 p-4" @submit.prevent="mkdir">
                <OperationError v-if="createError" :error="createError" />
                <UFormField :label="t('common.name')" required>
                  <UInput
                    v-model="newFolder"
                    :aria-label="t('common.name')"
                    autofocus
                    class="w-full"
                  />
                </UFormField>
                <UButton
                  type="submit"
                  :label="t('localPicker.create')"
                  :loading="creating"
                  :disabled="!newFolder.trim()"
                />
              </form>
            </template>
          </UPopover>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <UInput
            v-model="filter"
            icon="i-lucide-search"
            :placeholder="t('localPicker.filter')"
            :aria-label="t('localPicker.filter')"
            class="min-w-40 flex-1"
          />
          <UCheckbox v-model="showHidden" :label="t('localPicker.showHidden')" />
        </div>
        <OperationError v-if="error" :error="error" />
      </div>
      <UTable
        :data="entries"
        :columns="columns"
        :loading="status === 'pending'"
        :caption="title || t('localPicker.title')"
        :ui="{
          base: 'w-full table-fixed md:table-auto',
          td: 'whitespace-normal py-2 md:py-3'
        }"
        class="max-h-80"
      >
        <template #name-cell="{ row }">
          <UButton
            :aria-label="row.original.name"
            :color="selected?.path === row.original.path ? 'primary' : 'neutral'"
            variant="link"
            class="min-h-11 w-full items-start justify-start gap-2 px-0 py-0 text-start md:min-h-0 md:py-1.5"
            @click="open(row.original)"
          >
            <template #leading>
              <FileIcon
                :name="row.original.name"
                :directory="row.original.dir"
                :path="row.original.path"
                class="mt-0.5 size-5"
              />
            </template>
            <template #default>
              <span class="min-w-0 flex-1">
                <span class="block min-w-0 text-sm leading-6 [overflow-wrap:anywhere]">
                  {{ row.original.name }}
                </span>
                <span class="text-muted mt-0.5 block text-xs leading-4 md:hidden">
                  <span>{{
                    row.original.dir
                      ? t(row.original.empty ? 'localPicker.emptyFolder' : 'files.folder')
                      : format.bytes(row.original.size)
                  }}</span>
                  <template v-if="format.date(row.original.modified) !== '—'">
                    <span aria-hidden="true"> · </span>
                    <span>{{ format.date(row.original.modified) }}</span>
                  </template>
                </span>
              </span>
            </template>
          </UButton>
        </template>
        <template #size-cell="{ row }">
          <span class="text-muted">{{
            row.original.dir
              ? row.original.empty
                ? t('localPicker.emptyFolder')
                : '—'
              : format.bytes(row.original.size)
          }}</span>
        </template>
        <template #empty>
          <div>
            <UEmpty
              v-if="status === 'success'"
              :title="t('common.empty')"
              icon="i-lucide-folder"
              size="sm"
              class="py-10"
            />
          </div>
        </template>
      </UTable>
      <div class="space-y-1 px-4 py-3 sm:px-6">
        <p class="text-highlighted break-all text-sm">{{ value?.path }}</p>
        <p
          v-if="kind === 'mount' && status === 'success' && !selected && !data?.empty"
          class="text-warning text-xs"
        >
          {{ t('localPicker.emptyRequired') }}
        </p>
      </div>
    </template>
    <template #footer>
      <UButton
        :label="t('common.cancel')"
        color="neutral"
        variant="outline"
        @click="emit('close', null)"
      />
      <UButton
        :label="t('localPicker.select')"
        :disabled="!canChoose"
        @click="emit('close', value)"
      />
    </template>
  </UModal>
</template>

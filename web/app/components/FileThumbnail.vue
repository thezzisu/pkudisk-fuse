<script setup lang="ts">
import type { Entry } from '~/types/api'

const props = defineProps<{ account: string; entry: Entry }>()
const { t } = useI18n()
const thumbnails = useThumbnailsStore()
const target = useTemplateRef('target')
const visible = useElementVisibility(target)
const blob = shallowRef<Blob | null>(null)
const source = useObjectUrl(blob)
watch(
  [
    visible,
    () => props.account,
    () => props.entry.id,
    () => props.entry.rev,
    () => props.entry.modified_at
  ],
  async ([isVisible], _old, onCleanup) => {
    blob.value = null
    if (!isVisible || !props.entry.thumbnail) return
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    const result = await thumbnails
      .get(props.account, props.entry, controller.signal)
      .catch(() => null)
    if (!controller.signal.aborted) blob.value = result
  },
  { immediate: true }
)
</script>

<template>
  <span ref="target" class="flex size-6 shrink-0 items-center justify-center">
    <img
      v-if="source"
      :src="source"
      :alt="t('files.thumbnail', { name: entry.name })"
      class="size-6 rounded object-cover"
      :draggable="false"
      decoding="async"
      @error="blob = null"
    />
    <FileIcon
      v-else
      :name="entry.name"
      :directory="entry.directory"
      :path="entry.path"
      class="size-6"
    />
  </span>
</template>

<script setup lang="ts">
import { materialFileIcon } from '~/utils/material-icons'

const props = withDefaults(
  defineProps<{
    name: string
    directory?: boolean
    expanded?: boolean
    root?: boolean
    path?: string
    language?: string
  }>(),
  { directory: false, expanded: false, root: false, path: undefined, language: undefined }
)
const colorMode = useColorMode()
const source = computed(() =>
  materialFileIcon(props.name, { ...props, light: colorMode.value !== 'dark' })
)
</script>

<template>
  <img
    :src="source"
    alt=""
    aria-hidden="true"
    :draggable="false"
    decoding="async"
    width="20"
    height="20"
    class="shrink-0"
  />
</template>

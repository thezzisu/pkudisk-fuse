<script setup lang="ts">
defineProps<{ canWrite: boolean; loading: boolean; atRoot: boolean }>()
const emit = defineEmits<{ upload: []; mkdir: []; refresh: []; search: [] }>()
const draftSearch = defineModel<string>('search', { required: true })
const scopeSearch = defineModel<boolean>('scope', { required: true })
const limit = defineModel<number>('limit', { required: true })
const { t } = useI18n()
</script>
<template>
  <div class="flex w-full flex-wrap items-center gap-2">
    <UButton
      icon="i-lucide-upload"
      :label="t('files.upload')"
      :disabled="!canWrite"
      @click="emit('upload')"
    />
    <UButton
      icon="i-lucide-folder-plus"
      :label="t('files.newFolder')"
      color="neutral"
      variant="outline"
      :disabled="!canWrite"
      @click="emit('mkdir')"
    />
    <UButton
      icon="i-lucide-refresh-cw"
      :aria-label="t('common.refresh')"
      color="neutral"
      variant="ghost"
      :loading="loading"
      @click="emit('refresh')"
    />
    <form
      class="order-last flex w-full min-w-0 gap-1 sm:order-none sm:ml-auto sm:w-auto sm:max-w-xs"
      @submit.prevent="emit('search')"
    >
      <UInput
        v-model="draftSearch"
        data-search-input
        icon="i-lucide-search"
        :placeholder="t('files.searchPlaceholder')"
        :aria-label="t('common.search')"
        class="min-w-0 flex-1"
      /><UButton
        type="submit"
        icon="i-lucide-arrow-right"
        color="neutral"
        variant="outline"
        :aria-label="t('common.search')"
      />
    </form>
    <UPopover>
      <UButton
        icon="i-lucide-sliders-horizontal"
        color="neutral"
        variant="ghost"
        :aria-label="t('common.advanced')"
      /><template #content>
        <div class="space-y-4 p-4">
          <UCheckbox
            v-model="scopeSearch"
            :label="t('files.searchScope')"
            :disabled="atRoot"
          /><UFormField :label="t('files.searchLimit')">
            <UInputNumber v-model="limit" :min="1" :max="1000" />
          </UFormField>
        </div>
      </template>
    </UPopover>
  </div>
</template>

<script setup lang="ts">
const { t } = useI18n()
const view = useFileViewStore()
const options = computed(() => [
  { value: 'name-asc', label: t('files.nameAscending') },
  { value: 'name-desc', label: t('files.nameDescending') },
  { value: 'modified-desc', label: t('files.modifiedNewest') },
  { value: 'modified-asc', label: t('files.modifiedOldest') },
  { value: 'size-desc', label: t('files.sizeLargest') },
  { value: 'size-asc', label: t('files.sizeSmallest') }
])
</script>

<template>
  <UPopover>
    <UButton
      icon="i-lucide-arrow-down-wide-narrow"
      :label="t('files.sort')"
      color="neutral"
      variant="outline"
      size="sm"
    />
    <template #content>
      <div class="w-64 space-y-4 p-4">
        <UFormField :label="t('files.sort')">
          <USelect
            v-model="view.order"
            :items="options"
            :aria-label="t('files.sort')"
            class="w-full"
          />
        </UFormField>
        <USwitch v-model="view.foldersFirst" :label="t('files.foldersFirst')" />
      </div>
    </template>
  </UPopover>
</template>

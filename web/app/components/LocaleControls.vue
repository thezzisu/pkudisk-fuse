<script setup lang="ts">
import { en, zh_cn } from '@nuxt/ui/locale'

const { locale, setLocale, t } = useI18n()
</script>

<template>
  <div class="flex items-center gap-1">
    <USelectMenu
      :model-value="locale"
      :items="[zh_cn, en]"
      value-key="code"
      label-key="name"
      icon="i-lucide-languages"
      :search-input="false"
      size="sm"
      variant="ghost"
      :aria-label="t('common.language')"
      @update:model-value="setLocale($event as 'zh-CN' | 'en')"
    />
    <UColorModeButton size="sm" :aria-label="t('common.theme')" />
  </div>
</template>

<script setup lang="ts">
import type { ShareLink } from '~/types/api'
defineProps<{ links: ShareLink[]; name: string; busy: boolean }>()
const emit = defineEmits<{ remove: [link: ShareLink] }>()
const { t } = useI18n()
const format = useFormatting()
const clipboard = useClipboard()
const { run } = useApiAction({ notifyError: true })
async function copy(value: string) {
  await run(() => clipboard.copy(value), t('common.copied'))
}
</script>
<template>
  <div class="divide-default divide-y">
    <UEmpty
      v-if="!links.length"
      :title="t('sharing.noLinks')"
      icon="i-lucide-link"
      size="sm"
      class="py-10"
    />
    <article v-for="link in links" :key="link.id" class="space-y-3 px-4 py-4 sm:px-6">
      <div class="flex items-start justify-between gap-3">
        <h3 class="min-w-0 break-words font-medium">{{ link.title || name }}</h3>
        <UButton
          icon="i-lucide-unlink"
          :disabled="busy"
          :aria-label="t('sharing.revokeLink')"
          color="error"
          variant="ghost"
          size="sm"
          @click="emit('remove', link)"
        />
      </div>
      <UFieldGroup class="w-full">
        <UInput
          :model-value="link.url"
          readonly
          class="min-w-0 flex-1"
          :aria-label="t('sharing.external')"
        />
        <UButton
          icon="i-lucide-copy"
          :aria-label="t('common.copy')"
          color="neutral"
          variant="outline"
          @click="copy(link.url)"
        />
      </UFieldGroup>
      <dl class="text-muted flex flex-wrap gap-x-6 gap-y-2 text-xs">
        <div>
          <dt class="inline">{{ t('sharing.expiry') }}:</dt>
          <dd class="inline">
            {{ link.expires_at ? format.date(link.expires_at) : t('common.never') }}
          </dd>
        </div>
        <div>
          <dt class="inline">{{ t('sharing.visits') }}:</dt>
          <dd class="inline">
            {{ link.limited_times < 0 ? t('sharing.unlimited') : link.limited_times }}
          </dd>
        </div>
        <div v-if="link.code">
          <dt class="inline">{{ t('sharing.code') }}:</dt>
          <dd class="inline">{{ link.code }}</dd>
        </div>
        <div v-if="link.verify_mobile">
          <dt>{{ t('sharing.verifyMobile') }}</dt>
        </div>
      </dl>
      <div v-if="link.permissions?.length" class="flex flex-wrap gap-1">
        <UBadge
          v-for="permission in link.permissions"
          :key="permission"
          size="xs"
          color="neutral"
          variant="subtle"
          :label="t(`sharing.allow.${permission}`)"
        />
      </div>
    </article>
  </div>
</template>

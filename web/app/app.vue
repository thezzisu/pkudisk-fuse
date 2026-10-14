<script setup lang="ts">
import { en, zh_cn } from '@nuxt/ui/locale'
import { config as configureValidation } from 'zod'
import validationEN from 'zod/v4/locales/en.js'
import validationZH from 'zod/v4/locales/zh-CN.js'
import type { SessionState } from '~/types/api'

const { locale, t } = useI18n()
const nonce = import.meta.client
  ? document.querySelector<HTMLMetaElement>('meta[name="csp-nonce"]')?.content
  : undefined
watch(
  locale,
  (value) =>
    configureValidation({
      ...(value === 'zh-CN' ? validationZH() : validationEN()),
      jitless: true
    }),
  {
    immediate: true
  }
)
const session = useState<SessionState>('local-session')
const { isMobile, hostKnown, name: appName, description: appDescription } = useAppInfo()
const stoppedMessage = computed(() =>
  t(!hostKnown.value ? 'app.stoppedUnknown' : isMobile.value ? 'app.stoppedMobile' : 'app.stopped')
)
const expiredHint = computed(() =>
  t(
    !hostKnown.value
      ? 'app.expiredUnknownHint'
      : isMobile.value
        ? 'app.expiredMobileHint'
        : 'app.expiredHint'
  )
)
const overlay = useOverlay()
const toast = useToast()
watch(
  () => session.value.phase,
  (phase) => {
    if (phase === 'stopped') {
      overlay.closeAll()
      toast.clear()
    }
  }
)
const head = useLocaleHead()
useHead(() => ({ htmlAttrs: head.value.htmlAttrs, title: appName.value }))
</script>

<template>
  <UApp
    :locale="locale === 'zh-CN' ? zh_cn : en"
    :nonce="nonce"
    :toaster="session.phase === 'stopped' ? null : undefined"
  >
    <NuxtLayout v-if="session.phase === 'ready'">
      <NuxtPage />
    </NuxtLayout>
    <UModal
      v-else-if="session.phase === 'stopped'"
      :open="true"
      :title="stoppedMessage"
      :dismissible="false"
      :close="false"
      :transition="false"
      fullscreen
    >
      <template #content>
        <UEmpty
          icon="i-lucide-unplug"
          :title="stoppedMessage"
          variant="naked"
          size="xl"
          class="flex-1"
        />
      </template>
    </UModal>
    <div v-else class="bg-muted flex min-h-dvh items-center justify-center p-6">
      <UPageCard class="w-full max-w-md" variant="outline">
        <div class="flex items-center gap-3">
          <img src="/icon.png" :alt="appName" class="size-12 rounded-xl" />
          <div>
            <p class="text-highlighted font-semibold">
              {{ appName }}
            </p>
            <p v-if="appDescription" class="text-muted text-sm">
              {{ appDescription }}
            </p>
          </div>
        </div>
        <h1 role="status" class="text-highlighted mt-4 text-xl font-semibold">
          {{ t(`app.${session.phase}`) }}
        </h1>
        <p v-if="session.phase !== 'connecting'" class="text-muted text-sm leading-relaxed">
          {{ expiredHint }}
        </p>
        <UProgress v-else />
        <LocaleControls class="mt-4" />
      </UPageCard>
    </div>
  </UApp>
</template>

<script setup lang="ts">
import type { NavigationMenuItem } from '@nuxt/ui'
import type { SessionState } from '~/types/api'
import ConfirmModal from '~/components/ConfirmModal.vue'

const { t } = useI18n()
const { $api, $checkConnection, $endSession } = useNuxtApp()
const workspace = useWorkspaceStore()
const browser = useBrowserTransfersStore()
useEventListener(window, 'beforeunload', (event) => {
  if (browser.active.length) {
    event.preventDefault()
    event.returnValue = ''
  }
})
const session = useState<SessionState>('local-session')
const { isMobile: mobile, name: appName, description: appDescription } = useAppInfo()
const quitLabel = computed(() => t(mobile.value ? 'app.quitMobile' : 'app.quit'))
const feedback = useFeedback()
const confirm = useOverlay().create(ConfirmModal)
const links = computed<NavigationMenuItem[][]>(() => [
  [
    { label: t('nav.files'), icon: 'i-lucide-folder-open', to: '/', exact: true },
    {
      label: t('nav.transfers'),
      icon: 'i-lucide-arrow-down-up',
      to: '/transfers',
      badge: workspace.activeTasks.filter((task) => task.kind !== 'mount').length || undefined
    },
    ...(!mobile.value && workspace.flags.canMount
      ? [
          {
            label: t('nav.mounts'),
            icon: 'i-lucide-hard-drive',
            to: '/mounts',
            badge: workspace.activeTasks.filter((task) => task.kind === 'mount').length || undefined
          }
        ]
      : []),
    { label: t('nav.recycle'), icon: 'i-lucide-trash-2', to: '/recycle' }
  ],
  [
    { label: t('nav.accounts'), icon: 'i-lucide-users-round', to: '/accounts' },
    ...(!mobile.value && workspace.flags.canBrowseLocalFS
      ? [{ label: t('nav.recovery'), icon: 'i-lucide-history', to: '/recovery' }]
      : []),
    { label: t('nav.settings'), icon: 'i-lucide-settings-2', to: '/settings' }
  ]
])

const { pause, resume } = useIntervalFn(
  async () => {
    await $checkConnection()
    if (session.value.connection === 'online') await workspace.refreshTasks().catch(() => {})
  },
  2000,
  { immediateCallback: true }
)
useEventListener(window, 'focus', () => {
  void $checkConnection()
})
const visibility = useDocumentVisibility()
watch(visibility, (value) => {
  if (value === 'visible') void $checkConnection()
})
onMounted(() => workspace.initialize().catch(feedback.failure))

async function quit() {
  if (browser.active.length) {
    feedback.failure({ code: 'busy' })
    return
  }
  if (
    !(await confirm.open({
      title: quitLabel.value,
      description: t(mobile.value ? 'app.quitMobileHint' : 'app.quitHint')
    }).result)
  )
    return
  try {
    pause()
    await $api('/quit', { method: 'POST' })
    $endSession()
  } catch (error) {
    resume()
    feedback.failure(error)
  }
}
</script>

<template>
  <UDashboardGroup storage="cookie" storage-key="pkudisk-dashboard">
    <UDashboardSidebar
      role="complementary"
      :aria-label="appName"
      collapsible
      resizable
      :default-size="18"
      :min-size="14"
      :max-size="26"
    >
      <template #header="{ collapsed }">
        <NuxtLink to="/" class="flex min-w-0 items-center gap-3 py-2">
          <img src="/icon.png" :alt="appName" class="size-9 shrink-0 rounded-lg" />
          <div v-if="!collapsed" class="min-w-0">
            <p class="text-highlighted truncate font-semibold">{{ appName }}</p>
            <p class="text-muted truncate text-xs">
              {{ appDescription }}
            </p>
          </div>
        </NuxtLink>
      </template>
      <template #default="{ collapsed }">
        <UNavigationMenu
          :items="links"
          :collapsed="collapsed"
          orientation="vertical"
          :tooltip="collapsed"
          :popover="collapsed"
        />
        <div v-if="!collapsed" class="mt-auto space-y-3 px-2 py-5">
          <QuotaMeter class="mb-5" />
          <p v-if="!mobile && !session.tray" class="text-muted text-xs leading-relaxed">
            {{ t('app.noTray') }}
          </p>
          <LocaleControls class="sm:hidden" />
        </div>
      </template>
      <template #footer="{ collapsed }">
        <div class="flex w-full min-w-0 items-center gap-1">
          <UButton
            icon="i-lucide-power"
            :label="collapsed ? undefined : t('common.quit')"
            :aria-label="quitLabel"
            :title="quitLabel"
            color="neutral"
            variant="ghost"
            class="min-w-0 flex-1"
            :class="collapsed ? 'justify-center' : 'justify-start'"
            :ui="{ label: 'truncate' }"
            @click="quit"
          />
          <span
            v-if="!collapsed"
            data-testid="sidebar-version"
            :title="session.version"
            class="text-muted max-w-24 shrink-0 truncate pr-2 text-xs tabular-nums"
          >
            {{ session.version }}
          </span>
        </div>
      </template>
    </UDashboardSidebar>
    <slot />
    <LoginModal />
  </UDashboardGroup>
</template>

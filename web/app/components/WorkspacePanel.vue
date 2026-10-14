<script setup lang="ts">
withDefaults(
  defineProps<{ title: string; description?: string; flush?: boolean; accountSwitcher?: boolean }>(),
  { accountSwitcher: true, description: undefined }
)
</script>

<template>
  <UDashboardPanel
    role="main"
    :aria-label="title"
    :ui="flush ? { body: 'gap-0 overflow-hidden p-0 sm:gap-0 sm:p-0' } : undefined"
  >
    <template #header>
      <UDashboardNavbar :title="title">
        <template #leading>
          <UDashboardSidebarCollapse />
        </template>
        <template #right>
          <AccountSwitcher v-if="accountSwitcher" /><LocaleControls class="hidden sm:flex" />
        </template>
      </UDashboardNavbar>
      <UDashboardToolbar v-if="$slots.toolbar" class="py-2">
        <slot name="toolbar" />
      </UDashboardToolbar>
    </template>
    <template #body>
      <p
        v-if="description"
        class="text-muted text-sm"
        :class="flush ? 'px-4 pt-4 sm:px-6' : undefined"
      >
        {{ description }}
      </p>
      <slot />
    </template>
    <template v-if="$slots.footer" #footer>
      <div
        class="border-default flex shrink-0 flex-wrap items-center justify-between gap-3 border-t px-4 py-3 sm:px-6"
      >
        <slot name="footer" />
      </div>
    </template>
  </UDashboardPanel>
</template>

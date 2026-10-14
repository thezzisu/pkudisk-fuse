<script setup lang="ts">
import type { Permission } from '~/types/api'
defineProps<{ permissions: Permission[]; busy: boolean }>()
const emit = defineEmits<{ revoke: [permission: Permission] }>()
const { t } = useI18n()
const format = useFormatting()
const columns = computed(() => [
  { accessorKey: 'name', header: t('common.name') },
  { id: 'allow', header: t('sharing.permissions') },
  { id: 'source', header: t('common.details') },
  { id: 'actions', header: () => h('span', { class: 'sr-only' }, t('common.actions')) }
])
</script>
<template>
  <UTable :data="permissions" :columns="columns" :caption="t('sharing.permissions')">
    <template #name-cell="{ row }">
      <p class="max-w-44 truncate" :title="row.original.name || row.original.id">
        {{ row.original.name || row.original.id }}
      </p>
      <p class="text-muted text-xs">
        {{
          ['user', 'department'].includes(row.original.type)
            ? t(`sharing.${row.original.type}`)
            : row.original.type
        }}
      </p>
    </template>
    <template #allow-cell="{ row }">
      <div class="flex max-w-64 flex-wrap gap-1">
        <UBadge
          v-for="permission in row.original.allow"
          :key="permission"
          color="neutral"
          variant="subtle"
          size="xs"
          :label="t(`sharing.allow.${permission}`)"
        />
        <UBadge
          v-for="permission in row.original.deny"
          :key="`deny-${permission}`"
          color="error"
          variant="subtle"
          size="xs"
          :label="`${t('sharing.deny')}: ${t(`sharing.allow.${permission}`)}`"
        />
      </div>
    </template>
    <template #source-cell="{ row }">
      <p class="max-w-40 truncate text-xs" :title="row.original.inherited_path">
        {{
          row.original.inherited_path ||
          (row.original.inherited_from ? t('sharing.inherited') : t('sharing.direct'))
        }}
      </p>
      <p class="text-muted mt-1 text-xs">
        {{ row.original.expires_at ? format.date(row.original.expires_at) : t('common.never') }}
      </p>
    </template>
    <template #actions-cell="{ row }">
      <UButton
        v-if="!row.original.inherited_from && ['user', 'department'].includes(row.original.type)"
        icon="i-lucide-user-round-minus"
        :disabled="busy"
        :aria-label="t('sharing.revoke')"
        color="error"
        variant="ghost"
        @click="emit('revoke', row.original)"
      />
    </template>
    <template #empty>
      <UEmpty :title="t('sharing.noAccess')" icon="i-lucide-users-round" size="sm" class="py-10" />
    </template>
  </UTable>
</template>

import type { MaybeRefOrGetter } from 'vue'
import type { DropdownMenuItem } from '@nuxt/ui'
import type { Entry } from '~/types/api'
import TransferModal from '~/components/TransferModal.vue'
import FileActionModal from '~/components/FileActionModal.vue'
import SharingModal from '~/components/SharingModal.vue'
import HistoryModal from '~/components/HistoryModal.vue'
import ConfirmModal from '~/components/ConfirmModal.vue'

export function useFileActions(options: {
  account: MaybeRefOrGetter<string>
  folder: MaybeRefOrGetter<Entry | null>
  ready: MaybeRefOrGetter<boolean>
  refresh: () => Promise<unknown>
}) {
  const { t } = useI18n()
  const { $api } = useNuxtApp()
  const overlay = useOverlay()
  const transfer = overlay.create(TransferModal)
  const action = overlay.create(FileActionModal)
  const sharing = overlay.create(SharingModal)
  const history = overlay.create(HistoryModal)
  const confirm = overlay.create(ConfirmModal)
  const { run } = useApiAction({ refresh: options.refresh, notifyError: true })
  async function change(kind: 'mkdir' | 'rename' | 'move', entry: Entry) {
    if (!toValue(options.ready)) return
    if (await action.open({ action: kind, account: toValue(options.account), entry }).result)
      await options.refresh()
  }
  async function remove(entry: Entry) {
    if (!toValue(options.ready)) return
    const account = toValue(options.account)
    if (
      !(await confirm.open({
        title: t('files.deleteTitle'),
        description: t('files.deleteHint', { name: entry.name }),
        destructive: true
      }).result)
    )
      return
    await run(async () => {
      await $api('/files/action', {
        method: 'POST',
        body: { action: 'delete', account, path: entry.id, confirm: true }
      })
    }, true)
  }
  function menu(entry: Entry): DropdownMenuItem[][] {
    const root = entry.id.slice('gns://'.length).split('/').filter(Boolean).length < 2
    return [
      [
        {
          label: t('files.download'),
          icon: 'i-lucide-download',
          onSelect: () =>
            transfer.open({
              kind: 'download',
              account: toValue(options.account),
              entry,
              folder: toValue(options.folder)
            })
        },
        {
          label: t('files.copy'),
          icon: 'i-lucide-copy',
          onSelect: () =>
            transfer.open({
              kind: 'copy',
              account: toValue(options.account),
              entry,
              folder: toValue(options.folder)
            })
        },
        {
          label: t('files.extract'),
          icon: 'i-lucide-archive-restore',
          disabled: entry.directory,
          onSelect: () =>
            transfer.open({
              kind: 'extract',
              account: toValue(options.account),
              entry,
              folder: toValue(options.folder)
            })
        }
      ],
      [
        {
          label: t('files.share'),
          icon: 'i-lucide-share-2',
          onSelect: () => sharing.open({ account: toValue(options.account), entry })
        },
        {
          label: t('files.history'),
          icon: 'i-lucide-history',
          disabled: entry.directory,
          onSelect: () => history.open({ account: toValue(options.account), entry })
        }
      ],
      [
        {
          label: t('files.rename'),
          icon: 'i-lucide-pencil',
          disabled: root,
          onSelect: () => change('rename', entry)
        },
        {
          label: t('files.move'),
          icon: 'i-lucide-folder-input',
          disabled: root,
          onSelect: () => change('move', entry)
        },
        {
          label: t('common.delete'),
          icon: 'i-lucide-trash-2',
          color: 'error',
          disabled: root,
          onSelect: () => remove(entry)
        }
      ]
    ]
  }

  return { transfer, change, menu }
}

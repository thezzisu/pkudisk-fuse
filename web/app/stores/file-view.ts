import { defineStore } from 'pinia'
import type { SortingState } from '@tanstack/vue-table'

export type FileSort =
  'name-asc' | 'name-desc' | 'modified-desc' | 'modified-asc' | 'size-desc' | 'size-asc'

export const useFileViewStore = defineStore('file-view', () => {
  const order = useCookie<FileSort>('pkudisk-file-order', {
    default: () => 'name-asc',
    sameSite: 'strict'
  })
  const foldersFirst = useCookie<boolean>('pkudisk-folders-first', {
    default: () => true,
    sameSite: 'strict'
  })
  const sorting = computed<SortingState>(() => {
    const [column, direction] = order.value.split('-')
    return [
      ...(foldersFirst.value ? [{ id: 'directory', desc: true }] : []),
      { id: column === 'modified' ? 'modified_at' : column!, desc: direction === 'desc' },
      ...(column !== 'name' ? [{ id: 'name', desc: false }] : [])
    ]
  })
  function toggle(column: 'name' | 'modified' | 'size') {
    const first = column === 'name' ? 'asc' : 'desc'
    order.value =
      order.value === `${column}-${first}`
        ? `${column}-${first === 'asc' ? 'desc' : 'asc'}`
        : `${column}-${first}`
  }
  return { order, foldersFirst, sorting, toggle }
})

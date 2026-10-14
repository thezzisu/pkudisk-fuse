import Uppy from '@uppy/core'
import XHRUpload from '@uppy/xhr-upload'
import { defineStore } from 'pinia'
import type { Entry, Job, SessionState } from '~/types/api'

export const useBrowserTransfersStore = defineStore('browser-transfers', () => {
  const { $api, $checkConnection } = useNuxtApp()
  const session = useState<SessionState>('local-session')
  const uploads = shallowRef<Job[]>([])
  const uppy = new Uppy({ autoProceed: true, restrictions: { maxNumberOfFiles: 10000 } })
  uppy.use(XHRUpload, {
    endpoint: '/api/uploads',
    fieldName: 'file',
    limit: 2,
    timeout: 0,
    shouldRetry: () => false,
    allowedMetaFields: [
      'account',
      'destination',
      'relativePath',
      'lastModified',
      'overwrite',
      'fast',
      'timeoutSeconds',
      'clientId'
    ],
    onBeforeRequest(xhr) {
      if (session.value.connection !== 'online') throw new Error('Local service disconnected')
      const token = sessionStorage.getItem('pkudisk-gui-session')
      if (!token) throw new Error('Open the interface again')
      xhr.setRequestHeader('Authorization', `Bearer ${token}`)
    }
  })
  uppy.on('state-update', () => {
    uploads.value = uppy.getFiles().map((file) => ({
      id: `browser:${file.id}`,
      clientId: file.id,
      browser: true,
      allAccounts: false,
      kind: 'upload',
      account: String(file.meta.account),
      name: String(file.meta.relativePath || file.name),
      source: String(file.meta.relativePath || file.name),
      destination: String(file.meta.destination),
      state: file.error
        ? 'failed'
        : file.progress.uploadComplete
          ? 'completed'
          : file.progress.uploadStarted
            ? 'running'
            : 'queued',
      phase: 'receiving',
      done: file.progress.bytesUploaded || 0,
      total: file.size || 0,
      percent: file.progress.percentage || 0,
      created: String(file.meta.created),
      error: file.error ? { code: 'operation_failed', message: file.error } : undefined
    }))
  })
  uppy.on('upload-error', () => {
    void $checkConnection()
  })
  function upload(
    files: File[],
    options: {
      account: string
      destination: string
      overwrite: boolean
      fast: boolean
      timeoutSeconds: number
    }
  ) {
    for (const file of files) {
      const id = crypto.randomUUID()
      uppy.addFile({
        id,
        name: file.name,
        type: file.type,
        data: file,
        meta: {
          ...options,
          clientId: id,
          relativePath: file.webkitRelativePath || file.name,
          lastModified: file.lastModified,
          created: new Date().toISOString()
        }
      })
    }
  }
  const jobs = computed(() => uploads.value)
  const active = computed(() =>
    jobs.value.filter((job) => ['queued', 'running'].includes(job.state))
  )
  function remove(id: string) {
    if (id.startsWith('browser:')) uppy.removeFile(id.slice(8))
  }
  const cancel = remove
  async function nativeDownload(account: string, entry: Entry, timeoutSeconds: number) {
    const { url } = await $api<{ id: string; url: string }>('/downloads', {
      method: 'POST',
      credentials: 'same-origin',
      body: { account, path: entry.id, archive: entry.directory, timeoutSeconds }
    })
    // Standard attachment navigation works in browsers and WebView. The scoped
    // HttpOnly cookie is never exposed to JavaScript or put in the URL.
    const link = document.createElement('a')
    link.href = url
    link.download = entry.name + (entry.directory ? '.zip' : '')
    document.body.append(link)
    link.click()
    link.remove()
  }
  return { jobs, active, upload, nativeDownload, cancel, remove }
})

import type { ApiError, SessionState } from '~/types/api'

export function useFeedback() {
  const { t, te } = useI18n()
  const toast = useToast()
  const session = useState<SessionState>('local-session')
  function detail(error: unknown): ApiError {
    const value = error as { data?: { error?: ApiError }; code?: string; message?: string }
    const detail =
      value?.data?.error ??
      (value?.code
        ? { code: value.code, message: value.message ?? '' }
        : { code: 'operation_failed', message: value?.message ?? '' })
    const code = detail.code.replace(/^ERR_/, '').toLowerCase()
    const aliases: Record<string, string> = {
      pending: 'approval_pending',
      pickup_code_required: 'pickup_code',
      session_expired: 'login_required'
    }
    return { ...detail, code: aliases[code] || code }
  }
  function message(error: unknown) {
    const value = detail(error)
    if (session.value.flags.isMobile && te(`errors.mobile.${value.code}`))
      return t(`errors.mobile.${value.code}`)
    return te(`errors.${value.code}`) ? t(`errors.${value.code}`) : t('errors.operation_failed')
  }
  function failure(error: unknown) {
    if (session.value.phase === 'stopped') return
    toast.add({
      title: t('errors.title'),
      description: message(error),
      color: 'error',
      icon: 'i-lucide-circle-alert'
    })
  }
  function success(title = t('common.success')) {
    if (session.value.phase === 'stopped') return
    toast.add({ title, color: 'success', icon: 'i-lucide-circle-check' })
  }
  return { detail, message, failure, success }
}

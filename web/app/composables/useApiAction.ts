/** One mutation boundary for inline errors, duplicate-submit protection and refresh. */
export function useApiAction(
  options: {
    refresh?: () => unknown | Promise<unknown>
    notifyError?: boolean
    success?: string | boolean
  } = {}
) {
  const busy = ref(false)
  const error = shallowRef<unknown>(null)
  const feedback = useFeedback()
  async function run<T>(
    operation: () => Promise<T>,
    success = options.success ?? false
  ): Promise<T | undefined> {
    if (busy.value) return
    busy.value = true
    error.value = null
    try {
      const result = await operation()
      await options.refresh?.()
      if (success) feedback.success(typeof success === 'string' ? success : undefined)
      return result
    } catch (failure) {
      error.value = failure
      if (options.notifyError) feedback.failure(failure)
    } finally {
      busy.value = false
    }
  }
  return { busy, error, run }
}

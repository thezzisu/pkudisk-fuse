import prettyBytes from 'pretty-bytes'

export function useFormatting() {
  const { locale } = useI18n()
  const exact = useCookie<boolean>('pkudisk-exact-bytes', {
    default: () => false,
    sameSite: 'strict'
  })
  function bytes(value: number) {
    if (value < 0) return '—'
    return exact.value
      ? `${new Intl.NumberFormat(locale.value).format(value)} B`
      : prettyBytes(value, { binary: true, locale: locale.value })
  }
  function date(value?: string | null) {
    if (!value || value.startsWith('0001')) return '—'
    const parsed = new Date(value)
    return Number.isNaN(parsed.getTime())
      ? '—'
      : new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(
          parsed
        )
  }
  return { bytes, date, exact }
}

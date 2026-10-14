export default defineAppConfig({
  ui: {
    colors: { primary: 'pku', neutral: 'zinc' },
    dashboardPanel: { slots: { body: 'min-h-0' } },
    table: {
      slots: {
        th: 'py-2 first:ps-4 last:pe-4 sm:first:ps-6 sm:last:pe-6',
        td: 'py-3 first:ps-4 last:pe-4 sm:first:ps-6 sm:last:pe-6',
        empty: 'p-0'
      }
    },
    empty: { defaultVariants: { variant: 'naked' } },
    modal: {
      slots: {
        header: 'pe-14 sm:pe-14',
        footer: 'justify-end'
      }
    }
  }
})

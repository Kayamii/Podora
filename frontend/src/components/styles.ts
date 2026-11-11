/** Shared class strings for tables, so every list reads the same way. */

export const TH =
  'px-3 py-2.5 text-left text-[0.8125rem] font-medium text-muted whitespace-nowrap first:pl-5 last:pr-5'
export const TD = 'px-3 py-3 align-middle first:pl-5 last:pr-5'
export const NUM = 'text-right tabular-nums'
export const ROW = 'border-t border-line'
/** A row whose first-cell link covers the whole row. */
export const ROW_LINK = `${ROW} relative transition-colors duration-100 hover:bg-subtle`
export const STRETCHED_LINK =
  'font-medium text-fg after:absolute after:inset-0 hover:text-accent focus-visible:text-accent'

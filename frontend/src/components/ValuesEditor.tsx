import { useRef } from 'react'
import type { KeyboardEvent } from 'react'

interface Props {
  value: string
  onChange: (value: string) => void
  /** Accessible name. */
  label: string
  readOnly?: boolean
  /** Height of the editing area. */
  rows?: number
}

/** A YAML editor: monospace, line numbers, and Tab indents by two spaces. */
export function ValuesEditor({ value, onChange, label, readOnly = false, rows = 24 }: Props) {
  const gutter = useRef<HTMLDivElement>(null)
  const lineCount = Math.max(1, value.split('\n').length)

  function onKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key !== 'Tab' || readOnly) return
    // Indent in place instead of moving focus; Shift+Tab still leaves.
    if (e.shiftKey) return
    e.preventDefault()
    const el = e.currentTarget
    const { selectionStart: start, selectionEnd: end } = el
    const next = value.slice(0, start) + '  ' + value.slice(end)
    onChange(next)
    requestAnimationFrame(() => el.setSelectionRange(start + 2, start + 2))
  }

  return (
    <div
      className="flex overflow-hidden rounded-md border border-line-strong bg-surface focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/25"
      style={{ height: `${rows * 1.6 + 1.5}em` }}
    >
      <div
        ref={gutter}
        aria-hidden="true"
        className="shrink-0 overflow-hidden border-r border-line bg-subtle py-3 pr-3 pl-4 text-right font-mono text-[0.8125rem] leading-[1.6] text-muted select-none"
      >
        {Array.from({ length: lineCount }, (_, i) => (
          <div key={i}>{i + 1}</div>
        ))}
      </div>
      <textarea
        aria-label={label}
        value={value}
        readOnly={readOnly}
        spellCheck={false}
        autoCapitalize="off"
        autoComplete="off"
        wrap="off"
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={onKeyDown}
        onScroll={(e) => {
          if (gutter.current) gutter.current.scrollTop = e.currentTarget.scrollTop
        }}
        className={`flex-1 resize-none overflow-auto bg-transparent px-4 py-3 font-mono text-[0.8125rem] leading-[1.6] text-fg outline-none ${
          readOnly ? 'text-fg-2' : ''
        }`}
      />
    </div>
  )
}

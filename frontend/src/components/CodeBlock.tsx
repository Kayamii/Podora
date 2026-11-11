import { useState } from 'react'
import { Check, Copy } from 'lucide-react'
import { Button } from './ui'

/** Read-only text such as a rendered manifest, with a copy button. */
export function CodeBlock({ code, label, maxHeight = '36rem' }: { code: string; label: string; maxHeight?: string }) {
  const [copied, setCopied] = useState(false)

  async function copy() {
    try {
      await navigator.clipboard.writeText(code)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard can be unavailable (insecure context); the text stays selectable.
    }
  }

  return (
    <div className="relative">
      <div className="absolute top-3 right-3">
        <Button onClick={copy} icon={copied ? <Check size={15} /> : <Copy size={15} />}>
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      <pre
        tabIndex={0}
        aria-label={label}
        className="overflow-auto rounded-lg bg-code-bg p-5 pr-28 font-mono text-[0.8125rem] leading-[1.6] text-code-fg"
        style={{ maxHeight }}
      >
        {code || '(empty)'}
      </pre>
    </div>
  )
}

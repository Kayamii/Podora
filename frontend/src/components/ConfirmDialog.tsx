import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import { Button } from './ui'

interface Props {
  open: boolean
  title: string
  /** What will happen, in plain words. */
  children: ReactNode
  confirmLabel: string
  /** Style the confirm button as destructive. */
  danger?: boolean
  /** Performs the change. Rejecting keeps the dialog open and shows the error. */
  onConfirm: () => Promise<void>
  onClose: () => void
  /** Set false when the caller shows its own result after success. */
  closeOnSuccess?: boolean
}

/** Asks before changing the cluster, and reports failure where the user is looking. */
export function ConfirmDialog({ open, title, children, confirmLabel, danger = false, onConfirm, onClose, closeOnSuccess = true }: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const dialog = ref.current
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    if (!open && dialog.open) dialog.close()
  }, [open])

  async function confirm() {
    setBusy(true)
    setError(null)
    try {
      await onConfirm()
      if (closeOnSuccess) onClose()
    } catch (err) {
      setError(explain(err instanceof Error ? err.message : String(err)))
    } finally {
      setBusy(false)
    }
  }

  function close() {
    setError(null)
    onClose()
  }

  return (
    <dialog
      ref={ref}
      onClose={close}
      // Escape must not abandon a request that is already in flight.
      onCancel={(e) => busy && e.preventDefault()}
      className="m-auto w-[min(28rem,calc(100vw-2rem))] rounded-xl border border-line bg-surface p-0 text-fg shadow-xl"
    >
      <form
        method="dialog"
        onSubmit={(e) => {
          e.preventDefault()
          void confirm()
        }}
        className="p-6"
      >
        <h2 className="text-lg font-semibold">{title}</h2>
        <div className="mt-2 text-fg-2">{children}</div>
        {error && (
          <p role="alert" className="mt-4 rounded-md bg-danger-soft px-3 py-2 text-sm text-danger-fg">
            {error}
          </p>
        )}
        <div className="mt-6 flex justify-end gap-2">
          <Button onClick={close} disabled={busy}>
            Cancel
          </Button>
          <Button type="submit" variant={danger ? 'danger' : 'primary'} disabled={busy}>
            {busy ? 'Working…' : confirmLabel}
          </Button>
        </div>
      </form>
    </dialog>
  )
}

/** Explains the failures a user can do something about. */
function explain(message: string): string {
  if (message.includes('forbidden') || message.includes('Forbidden')) {
    return `Podora's service account is not allowed to make this change. Scale, restart and delete need the chart value rbac.allowWrites=true; Helm installs need rbac.helm=true. (${message})`
  }
  if (message.includes('not found')) {
    return 'This resource no longer exists. It may have been deleted or replaced.'
  }
  return message
}

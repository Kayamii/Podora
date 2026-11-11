import { useState } from 'react'
import { accounts } from '../services/api'
import { ConfirmDialog } from './ConfirmDialog'
import { Field } from './ui'

/** Lets the signed-in user replace their password. Other sessions are signed out. */
export function ChangePasswordDialog({ onClose }: { onClose: () => void }) {
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [repeat, setRepeat] = useState('')
  const [done, setDone] = useState(false)

  if (done) {
    return (
      <ConfirmDialog open title="Password changed" confirmLabel="Done" onClose={onClose} onConfirm={async () => {}}>
        Your new password is active. Any other browser where you were signed in has been signed out.
      </ConfirmDialog>
    )
  }

  return (
    <ConfirmDialog
      open
      title="Change password"
      confirmLabel="Change password"
      onClose={onClose}
      closeOnSuccess={false}
      onConfirm={async () => {
        if (next !== repeat) throw new Error('The two new passwords are different.')
        await accounts.changePassword(current, next)
        setDone(true)
      }}
    >
      <div className="space-y-4">
        <Field label="Current password">
          <input className="input w-full" type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} required autoFocus />
        </Field>
        <Field label="New password (at least 10 characters)">
          <input className="input w-full" type="password" autoComplete="new-password" value={next} onChange={(e) => setNext(e.target.value)} required />
        </Field>
        <Field label="Repeat new password">
          <input className="input w-full" type="password" autoComplete="new-password" value={repeat} onChange={(e) => setRepeat(e.target.value)} required />
        </Field>
      </div>
    </ConfirmDialog>
  )
}

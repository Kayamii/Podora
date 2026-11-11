import { useCallback, useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import { RotateCw } from 'lucide-react'
import { makeCan, SessionContext } from '../hooks/session'
import { accounts, SIGNED_OUT_EVENT } from '../services/api'
import type { Me } from '../types/api'
import { Button } from './ui'

type State =
  | { phase: 'loading' }
  | { phase: 'error'; message: string }
  | { phase: 'setup' }
  | { phase: 'signIn'; notice?: string }
  | { phase: 'in'; me: Me }

/** Shows sign-in or first-run setup until there is a session, then the app. */
export function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State>({ phase: 'loading' })
  // Bumped by "Try again" to check once more.
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    let cancelled = false
    void resolveSession().then((next) => {
      if (!cancelled) setState(next)
    })
    return () => {
      cancelled = true
    }
  }, [attempt])

  const retry = () => {
    setState({ phase: 'loading' })
    setAttempt((n) => n + 1)
  }

  // Any request that comes back 401 means the session expired or was ended.
  useEffect(() => {
    const onSignedOut = () =>
      setState((s) => (s.phase === 'in' ? { phase: 'signIn', notice: 'Your session ended. Sign in again to continue.' } : s))
    window.addEventListener(SIGNED_OUT_EVENT, onSignedOut)
    return () => window.removeEventListener(SIGNED_OUT_EVENT, onSignedOut)
  }, [])

  const signOut = useCallback(async () => {
    try {
      await accounts.logout()
    } finally {
      setState({ phase: 'signIn' })
    }
  }, [])

  const session = useMemo(
    () => (state.phase === 'in' ? { me: state.me, can: makeCan(state.me), signOut } : null),
    [state, signOut],
  )

  if (session) return <SessionContext.Provider value={session}>{children}</SessionContext.Provider>

  return (
    <div className="flex min-h-screen items-center justify-center bg-canvas px-4 py-10 text-fg">
      <div className="w-full max-w-100">
        <div className="mb-8 flex items-center justify-center gap-2.5">
          <Logo />
          <span className="text-xl font-semibold tracking-tight">Podora</span>
        </div>
        {state.phase === 'loading' && <p className="text-center text-fg-2">Loading…</p>}
        {state.phase === 'error' && (
          <Panel title="Podora is not responding">
            <p className="text-fg-2">The Podora backend could not be reached.</p>
            <p className="mt-2 text-sm break-words text-muted">{state.message}</p>
            <Button className="mt-5" icon={<RotateCw size={15} />} onClick={retry}>
              Try again
            </Button>
          </Panel>
        )}
        {state.phase === 'setup' && <Setup onDone={(me) => setState({ phase: 'in', me })} />}
        {state.phase === 'signIn' && <SignIn notice={state.notice} onDone={(me) => setState({ phase: 'in', me })} />}
      </div>
    </div>
  )
}

/** Asks the backend whether setup is needed and who is signed in. */
async function resolveSession(): Promise<State> {
  try {
    const { setupRequired } = await accounts.status()
    if (setupRequired) return { phase: 'setup' }
    const me = await accounts.me()
    return me ? { phase: 'in', me } : { phase: 'signIn' }
  } catch (err) {
    return { phase: 'error', message: err instanceof Error ? err.message : String(err) }
  }
}

function Panel({ title, subtitle, children }: { title: string; subtitle?: string; children: ReactNode }) {
  return (
    <div className="rounded-xl border border-line bg-surface p-7 shadow-card">
      <h1 className="text-lg font-semibold">{title}</h1>
      {subtitle && <p className="mt-1 text-sm text-fg-2">{subtitle}</p>}
      <div className="mt-6">{children}</div>
    </div>
  )
}

function TextField(props: { label: string; type?: string; value: string; onChange: (v: string) => void; autoComplete: string; autoFocus?: boolean; hint?: string }) {
  return (
    <label className="block">
      <span className="text-sm font-medium">{props.label}</span>
      <input
        className="input mt-1 w-full"
        type={props.type ?? 'text'}
        value={props.value}
        onChange={(e) => props.onChange(e.target.value)}
        autoComplete={props.autoComplete}
        autoFocus={props.autoFocus}
        required
      />
      {props.hint && <span className="mt-1 block text-xs text-muted">{props.hint}</span>}
    </label>
  )
}

function useSubmit(action: () => Promise<Me>, onDone: (me: Me) => void) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      onDone(await action())
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
      setBusy(false)
    }
  }
  return { busy, error, submit, setError }
}

function SignIn({ notice, onDone }: { notice?: string; onDone: (me: Me) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const { busy, error, submit } = useSubmit(() => accounts.login(username, password), onDone)

  return (
    <Panel title="Sign in" subtitle="Use the account an administrator gave you.">
      {notice && !error && <p className="mb-4 rounded-md bg-accent-soft px-3 py-2 text-sm text-accent">{notice}</p>}
      <form onSubmit={submit} className="space-y-4">
        <TextField label="Username" value={username} onChange={setUsername} autoComplete="username" autoFocus />
        <TextField label="Password" type="password" value={password} onChange={setPassword} autoComplete="current-password" />
        {error && (
          <p role="alert" className="rounded-md bg-danger-soft px-3 py-2 text-sm text-danger-fg">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" className="w-full justify-center" disabled={busy}>
          {busy ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>
    </Panel>
  )
}

function Setup({ onDone }: { onDone: (me: Me) => void }) {
  const [token, setToken] = useState('')
  const [username, setUsername] = useState('admin')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const { busy, error, submit, setError } = useSubmit(() => accounts.setup(token, username, password), onDone)

  return (
    <Panel title="Set up Podora" subtitle="Create the first administrator. They can then add everyone else.">
      <form
        onSubmit={(e) => {
          if (password !== confirm) {
            e.preventDefault()
            setError('The two passwords are different.')
            return
          }
          void submit(e)
        }}
        className="space-y-4"
      >
        <TextField
          label="Setup token"
          value={token}
          onChange={setToken}
          autoComplete="one-time-code"
          autoFocus
          hint="Printed in the Podora backend's log when it starts with no accounts. It proves you run this installation."
        />
        <TextField label="Administrator username" value={username} onChange={setUsername} autoComplete="username" />
        <TextField label="Password" type="password" value={password} onChange={setPassword} autoComplete="new-password" hint="At least 10 characters." />
        <TextField label="Repeat password" type="password" value={confirm} onChange={setConfirm} autoComplete="new-password" />
        {error && (
          <p role="alert" className="rounded-md bg-danger-soft px-3 py-2 text-sm text-danger-fg">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" className="w-full justify-center" disabled={busy}>
          {busy ? 'Creating…' : 'Create administrator'}
        </Button>
      </form>
    </Panel>
  )
}

export function Logo({ size = 28 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 22 22" aria-hidden="true">
      <rect x="1" y="1" width="20" height="20" rx="5" fill="var(--accent)" />
      <rect x="5" y="5" width="5" height="5" rx="1.2" fill="white" />
      <rect x="12" y="5" width="5" height="5" rx="1.2" fill="white" opacity="0.7" />
      <rect x="5" y="12" width="12" height="5" rx="1.2" fill="white" opacity="0.85" />
    </svg>
  )
}

import { useState } from 'react'
import { Pencil, Trash2, UserPlus } from 'lucide-react'
import { ConfirmDialog } from '../components/ConfirmDialog'
import { ROW, TD, TH } from '../components/styles'
import { Badge, Button, Card, Field, Notice, PageHeader, Skeleton, Status, Switch, TableScroll } from '../components/ui'
import { usePolling } from '../hooks/usePolling'
import { ROLE_DESCRIPTION, ROLE_LABEL, useSession } from '../hooks/session'
import { accounts, api } from '../services/api'
import { formatAge } from '../services/format'
import { plural } from '../services/health'
import type { Role, User } from '../types/api'

const ROLES: Role[] = ['viewer', 'developer', 'operator', 'admin']

type Editing = { mode: 'create' } | { mode: 'edit'; user: User } | null

export function Users() {
  const { me } = useSession()
  const { data, error, loading, reload } = usePolling(accounts.users, 15_000)
  const [editing, setEditing] = useState<Editing>(null)
  const [deleting, setDeleting] = useState<User | null>(null)
  const [done, setDone] = useState<string | null>(null)

  const users = data ?? []
  const finish = (message: string) => {
    setDone(message)
    reload()
  }

  return (
    <div>
      <PageHeader
        title="Users"
        subtitle={loading ? 'Loading…' : `${plural(users.length, 'account')} · ${plural(users.filter((u) => u.role === 'admin').length, 'administrator')}`}
        actions={
          <Button variant="primary" icon={<UserPlus size={16} />} onClick={() => setEditing({ mode: 'create' })}>
            Add user
          </Button>
        }
      />
      {done && <Notice level="normal">{done}</Notice>}
      {error && <Notice level="warning">Could not refresh: {error}</Notice>}

      <Card padded={false}>
        {loading ? (
          <Skeleton rows={3} />
        ) : (
          <TableScroll>
            <thead>
              <tr>
                <th className={TH}>User</th>
                <th className={TH}>Role</th>
                <th className={TH}>Namespaces</th>
                <th className={TH}>Status</th>
                <th className={TH}>Last sign-in</th>
                <th className={TH}>
                  <span className="sr-only">Actions</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {users.map((u) => (
                <tr key={u.id} className={ROW}>
                  <td className={TD}>
                    <span className="font-medium">{u.username}</span>
                    {u.id === me.user.id && <span className="ml-2 text-sm text-muted">(you)</span>}
                    <div className="text-sm text-muted">Added {formatAge(u.createdAt)} ago</div>
                  </td>
                  <td className={TD}>
                    <Badge level={u.role === 'admin' ? 'warning' : 'neutral'}>{ROLE_LABEL[u.role]}</Badge>
                  </td>
                  <td className={TD}>
                    {u.namespaces.length === 0 ? (
                      <span className="text-fg-2">All</span>
                    ) : (
                      <span className="flex flex-wrap gap-1.5">
                        {u.namespaces.map((ns) => (
                          <span key={ns} className="rounded bg-subtle px-1.5 py-0.5 text-sm">
                            {ns}
                          </span>
                        ))}
                      </span>
                    )}
                  </td>
                  <td className={TD}>
                    <Status level={u.disabled ? 'neutral' : 'normal'}>{u.disabled ? 'Disabled' : 'Active'}</Status>
                  </td>
                  <td className={`${TD} text-fg-2`}>{u.lastLogin ? `${formatAge(u.lastLogin)} ago` : 'Never'}</td>
                  <td className={`${TD} text-right whitespace-nowrap`}>
                    <Button icon={<Pencil size={14} />} onClick={() => setEditing({ mode: 'edit', user: u })}>
                      Edit
                    </Button>
                    {u.id !== me.user.id && (
                      <Button
                        className="ml-2"
                        variant="dangerOutline"
                        icon={<Trash2 size={14} />}
                        aria-label={`Delete ${u.username}`}
                        onClick={() => setDeleting(u)}
                      />
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </TableScroll>
        )}
      </Card>

      {editing && (
        <UserDialog
          key={editing.mode === 'edit' ? editing.user.id : 'new'}
          editing={editing}
          isSelf={editing.mode === 'edit' && editing.user.id === me.user.id}
          onClose={() => setEditing(null)}
          onDone={finish}
        />
      )}
      <ConfirmDialog
        open={deleting != null}
        title={`Delete ${deleting?.username}?`}
        confirmLabel="Delete account"
        danger
        onClose={() => setDeleting(null)}
        onConfirm={async () => {
          if (!deleting) return
          await accounts.deleteUser(deleting.id)
          finish(`Deleted ${deleting.username}. They were signed out everywhere.`)
        }}
      >
        They are signed out at once and can no longer sign in. Their entries in the audit log stay. To keep the account for later,
        disable it instead.
      </ConfirmDialog>
    </div>
  )
}

function UserDialog({ editing, isSelf, onClose, onDone }: { editing: NonNullable<Editing>; isSelf: boolean; onClose: () => void; onDone: (m: string) => void }) {
  const existing = editing.mode === 'edit' ? editing.user : null
  const namespaces = usePolling(api.namespaces, 60_000)
  const [username, setUsername] = useState(existing?.username ?? '')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState<Role>(existing?.role ?? 'viewer')
  const [allNamespaces, setAllNamespaces] = useState((existing?.namespaces.length ?? 0) === 0)
  const [selected, setSelected] = useState<string[]>(existing?.namespaces ?? [])
  const [disabled, setDisabled] = useState(existing?.disabled ?? false)

  const known = [...new Set([...(namespaces.data ?? []).map((n) => n.name), ...selected])].sort()
  const scoped = role !== 'admin' && !allNamespaces

  async function save() {
    const nsList = scoped ? selected : []
    if (scoped && nsList.length === 0) throw new Error('Choose at least one namespace, or give access to all namespaces.')
    if (existing) {
      await accounts.updateUser(existing.id, {
        ...(isSelf ? {} : { role, disabled }),
        namespaces: nsList,
        ...(password ? { password } : {}),
      })
      onDone(`Saved ${existing.username}.${password || disabled ? ' They were signed out everywhere.' : ''}`)
    } else {
      await accounts.createUser({ username, password, role, namespaces: nsList })
      onDone(`Added ${username}. Give them their username and password; they can change the password after signing in.`)
    }
  }

  return (
    <ConfirmDialog open title={existing ? `Edit ${existing.username}` : 'Add user'} confirmLabel={existing ? 'Save' : 'Add user'} onClose={onClose} onConfirm={save}>
      <div className="space-y-5">
        {!existing && (
          <Field label="Username">
            <input className="input w-full" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" required autoFocus />
          </Field>
        )}
        <Field label={existing ? 'New password (leave empty to keep the current one)' : 'Password (at least 10 characters)'}>
          <input
            className="input w-full"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            required={!existing}
          />
        </Field>

        <fieldset disabled={isSelf}>
          <legend className="text-[0.8125rem] font-medium text-muted">Role{isSelf && ' (another administrator must change your own role)'}</legend>
          <div className="mt-1.5 space-y-1.5">
            {ROLES.map((r) => (
              <label
                key={r}
                className={`flex cursor-pointer gap-3 rounded-md border px-3 py-2 ${role === r ? 'border-accent bg-accent-soft' : 'border-line hover:bg-subtle'}`}
              >
                <input type="radio" name="role" className="mt-1 accent-[var(--accent)]" checked={role === r} onChange={() => setRole(r)} />
                <span>
                  <span className="block text-sm font-medium">{ROLE_LABEL[r]}</span>
                  <span className="block text-xs text-fg-2">{ROLE_DESCRIPTION[r]}</span>
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        {role !== 'admin' && (
          <div>
            <Switch checked={allNamespaces} onChange={setAllNamespaces}>
              Access to all namespaces
            </Switch>
            {scoped && (
              <div className="mt-2 max-h-44 overflow-y-auto rounded-md border border-line p-2">
                {known.length === 0 && <p className="p-1 text-sm text-muted">Loading namespaces…</p>}
                {known.map((ns) => (
                  <label key={ns} className="flex cursor-pointer items-center gap-2 rounded px-1.5 py-1 text-sm hover:bg-subtle">
                    <input
                      type="checkbox"
                      className="accent-[var(--accent)]"
                      checked={selected.includes(ns)}
                      onChange={(e) => setSelected((s) => (e.target.checked ? [...s, ns] : s.filter((x) => x !== ns)))}
                    />
                    {ns}
                  </label>
                ))}
              </div>
            )}
            {scoped && role === 'operator' && (
              <p className="mt-2 text-xs text-fg-2">Limited to namespaces, an operator can scale and restart but not use Helm.</p>
            )}
          </div>
        )}

        {existing && !isSelf && (
          <Switch checked={disabled} onChange={setDisabled}>
            Disable this account (signs them out and blocks sign-in)
          </Switch>
        )}
      </div>
    </ConfirmDialog>
  )
}

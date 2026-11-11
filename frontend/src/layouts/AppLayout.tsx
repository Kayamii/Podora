import { useEffect, useRef, useState } from 'react'
import type { ReactNode } from 'react'
import {
  Activity,
  Boxes,
  ChevronsUpDown,
  Container,
  FolderTree,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  Network,
  Package,
  ScrollText,
  Server,
  Users,
  X,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Logo } from '../components/AuthGate'
import { ChangePasswordDialog } from '../components/ChangePasswordDialog'
import { ROLE_LABEL, useSession } from '../hooks/session'
import type { SessionValue } from '../hooks/session'
import { href } from '../hooks/useRoute'
import type { Section } from '../hooks/useRoute'

interface NavItem {
  id: Section
  label: string
  to: string
  icon: LucideIcon
  /** Hidden from users this returns false for. */
  visible?: (s: SessionValue) => boolean
}

const clusterWide = (s: SessionValue) => s.me.allNamespaces

const NAV: { group: string; items: NavItem[] }[] = [
  {
    group: 'Cluster',
    items: [
      { id: 'overview', label: 'Overview', to: href.overview, icon: LayoutDashboard, visible: clusterWide },
      { id: 'nodes', label: 'Nodes', to: href.nodes, icon: Server, visible: clusterWide },
      { id: 'namespaces', label: 'Namespaces', to: href.namespaces, icon: FolderTree },
      { id: 'events', label: 'Events', to: href.events(), icon: Activity },
    ],
  },
  {
    group: 'Workloads',
    items: [
      { id: 'workloads', label: 'Workloads', to: href.workloads(), icon: Boxes },
      { id: 'pods', label: 'Pods', to: href.pods(), icon: Container },
    ],
  },
  {
    group: 'Applications',
    items: [{ id: 'helm', label: 'Helm releases', to: href.helm, icon: Package }],
  },
  {
    group: 'Networking',
    items: [{ id: 'network', label: 'Services & Ingresses', to: href.network(), icon: Network }],
  },
  {
    group: 'Administration',
    items: [
      { id: 'users', label: 'Users', to: href.users, icon: Users, visible: (s) => s.can('users.manage') },
      { id: 'audit', label: 'Audit log', to: href.audit, icon: ScrollText, visible: (s) => s.can('audit.view') },
    ],
  },
]

interface Props {
  section: Section | null
  /** Shown at the right of the top bar: cluster version and connection. */
  status?: ReactNode
  children: ReactNode
}

export function AppLayout({ section, status, children }: Props) {
  const session = useSession()
  // Mobile only: the sidebar becomes a drawer.
  const [open, setOpen] = useState(false)
  const home = session.me.allNamespaces ? href.overview : href.namespaces

  const groups = NAV.map((g) => ({ ...g, items: g.items.filter((i) => !i.visible || i.visible(session)) })).filter(
    (g) => g.items.length > 0,
  )

  return (
    <div className="min-h-screen bg-canvas text-fg lg:grid lg:grid-cols-[15rem_1fr]">
      <aside
        className={`fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-line bg-surface transition-transform duration-200 lg:sticky lg:top-0 lg:h-screen lg:translate-x-0 ${
          open ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="flex h-14 shrink-0 items-center justify-between border-b border-line px-5">
          <a href={home} className="flex items-center gap-2.5" onClick={() => setOpen(false)}>
            <Logo size={22} />
            <span className="text-[1.0625rem] font-semibold tracking-tight">Podora</span>
          </a>
          <button
            type="button"
            className="cursor-pointer rounded-md p-1.5 text-muted hover:bg-subtle hover:text-fg lg:hidden"
            onClick={() => setOpen(false)}
            aria-label="Close navigation"
          >
            <X size={18} />
          </button>
        </div>

        <nav aria-label="Sections" className="flex-1 space-y-6 overflow-y-auto px-3 py-5">
          {groups.map((group) => (
            <div key={group.group}>
              <h2 className="mb-1.5 px-2 text-xs font-medium text-muted">{group.group}</h2>
              <ul className="space-y-0.5">
                {group.items.map((item) => {
                  const current = item.id === section
                  const Icon = item.icon
                  return (
                    <li key={item.id}>
                      <a
                        href={item.to}
                        aria-current={current ? 'page' : undefined}
                        onClick={() => setOpen(false)}
                        className={`flex items-center gap-2.5 rounded-md px-2 py-1.5 text-[0.9375rem] transition-colors duration-100 ${
                          current ? 'bg-accent-soft font-medium text-accent' : 'text-fg-2 hover:bg-subtle hover:text-fg'
                        }`}
                      >
                        <Icon size={17} strokeWidth={1.75} aria-hidden="true" />
                        {item.label}
                      </a>
                    </li>
                  )
                })}
              </ul>
            </div>
          ))}
        </nav>

        <AccountMenu />
      </aside>

      {open && <div aria-hidden="true" className="fixed inset-0 z-30 bg-black/40 lg:hidden" onClick={() => setOpen(false)} />}

      <div className="min-w-0">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-line bg-surface/90 px-4 backdrop-blur sm:px-6">
          <button
            type="button"
            className="-ml-1.5 cursor-pointer rounded-md p-1.5 text-fg-2 hover:bg-subtle lg:hidden"
            onClick={() => setOpen(true)}
            aria-label="Open navigation"
          >
            <Menu size={20} />
          </button>
          <div className="ml-auto flex items-center gap-4 text-sm text-fg-2">{status}</div>
        </header>
        <main className="mx-auto max-w-360 px-4 py-6 sm:px-6 lg:px-8 lg:py-8">{children}</main>
      </div>
    </div>
  )
}

/** The signed-in user at the foot of the sidebar, with account actions. */
function AccountMenu() {
  const { me, signOut } = useSession()
  const [open, setOpen] = useState(false)
  const [changing, setChanging] = useState(false)
  const box = useRef<HTMLDivElement>(null)

  // Close on outside click or Escape.
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (box.current && !box.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const scope = me.allNamespaces ? 'All namespaces' : me.user.namespaces.join(', ')

  return (
    <div ref={box} className="relative shrink-0 border-t border-line p-3">
      {open && (
        <div role="menu" className="absolute right-3 bottom-full left-3 mb-2 rounded-lg border border-line bg-surface p-1 shadow-lg">
          <button
            type="button"
            role="menuitem"
            className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-sm hover:bg-subtle"
            onClick={() => {
              setOpen(false)
              setChanging(true)
            }}
          >
            <KeyRound size={16} aria-hidden="true" /> Change password
          </button>
          <button
            type="button"
            role="menuitem"
            className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-2 text-sm text-danger-fg hover:bg-danger-soft"
            onClick={() => void signOut()}
          >
            <LogOut size={16} aria-hidden="true" /> Sign out
          </button>
        </div>
      )}
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="flex w-full cursor-pointer items-center gap-3 rounded-md px-2 py-1.5 text-left hover:bg-subtle"
      >
        <span
          aria-hidden="true"
          className="flex size-8 shrink-0 items-center justify-center rounded-full bg-accent-soft text-sm font-semibold text-accent uppercase"
        >
          {me.user.username.slice(0, 1)}
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-medium">{me.user.username}</span>
          <span className="block truncate text-xs text-muted" title={scope}>
            {ROLE_LABEL[me.user.role]} · {scope}
          </span>
        </span>
        <ChevronsUpDown size={15} className="shrink-0 text-muted" aria-hidden="true" />
      </button>
      {changing && <ChangePasswordDialog onClose={() => setChanging(false)} />}
    </div>
  )
}

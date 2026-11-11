import { createContext, useContext } from 'react'
import type { Me, Permission } from '../types/api'

export interface SessionValue {
  me: Me
  /** Whether the user may do `p`, in namespace `ns` when given. */
  can: (p: Permission, ns?: string) => boolean
  signOut: () => Promise<void>
}

export const SessionContext = createContext<SessionValue | null>(null)

/** The signed-in user. Only usable below the sign-in gate. */
export function useSession(): SessionValue {
  const value = useContext(SessionContext)
  if (!value) throw new Error('useSession used outside the signed-in app')
  return value
}

/** Mirrors the backend's rule (rbac.Subject.Can) so the UI hides what would be refused. */
export function makeCan(me: Me): SessionValue['can'] {
  return (p, ns) => {
    if (!me.permissions.includes(p)) return false
    if (ns === undefined || me.allNamespaces) return true
    return me.user.namespaces.includes(ns)
  }
}

export const ROLE_LABEL: Record<Me['user']['role'], string> = {
  viewer: 'Viewer',
  developer: 'Developer',
  operator: 'Operator',
  admin: 'Administrator',
}

export const ROLE_DESCRIPTION: Record<Me['user']['role'], string> = {
  viewer: 'Sees everything in their namespaces, including logs. Changes nothing.',
  developer: 'Viewer, plus restart workloads and delete pods.',
  operator: 'Developer, plus scale workloads. With all namespaces, also installs and manages Helm releases.',
  admin: 'Everything, plus managing users and reading the audit log. Always has all namespaces.',
}

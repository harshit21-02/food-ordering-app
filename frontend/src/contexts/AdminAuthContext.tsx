import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { adminApi, adminTokenStore, type StaffUser } from '../lib/api'

type State =
  | { status: 'unknown' }
  | { status: 'guest' }
  | { status: 'authed'; staff: StaffUser }

type AdminAuthContextValue = {
  state: State
  signIn: (jwt: string, staff: StaffUser) => void
  signOut: () => Promise<void>
}

const AdminAuthContext = createContext<AdminAuthContextValue | null>(null)

export function AdminAuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<State>({ status: 'unknown' })

  useEffect(() => {
    const token = adminTokenStore.get()
    if (!token) {
      setState({ status: 'guest' })
      return
    }
    adminApi
      .me()
      .then((staff) => setState({ status: 'authed', staff }))
      .catch(() => {
        adminTokenStore.clear()
        setState({ status: 'guest' })
      })
  }, [])

  const signIn = (jwt: string, staff: StaffUser) => {
    adminTokenStore.set(jwt)
    setState({ status: 'authed', staff })
  }

  const signOut = async () => {
    try { await adminApi.logout() } catch { /* ignore */ }
    adminTokenStore.clear()
    setState({ status: 'guest' })
  }

  return <AdminAuthContext.Provider value={{ state, signIn, signOut }}>{children}</AdminAuthContext.Provider>
}

export function useAdminAuth() {
  const ctx = useContext(AdminAuthContext)
  if (!ctx) throw new Error('useAdminAuth must be used within <AdminAuthProvider>')
  return ctx
}

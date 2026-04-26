import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
import { api, tokenStore, type Customer } from '../lib/api'

type AuthState =
  | { status: 'unknown' }
  | { status: 'guest' }
  | { status: 'authed'; customer: Customer }

type AuthContextValue = {
  state: AuthState
  // Called after a successful OTP verify with the JWT + customer.
  signIn: (jwt: string, customer: Customer) => void
  signOut: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'unknown' })

  // On mount, hydrate from localStorage and verify the token still works.
  useEffect(() => {
    const token = tokenStore.get()
    if (!token) {
      setState({ status: 'guest' })
      return
    }
    api
      .me()
      .then((customer) => setState({ status: 'authed', customer }))
      .catch(() => {
        tokenStore.clear()
        setState({ status: 'guest' })
      })
  }, [])

  const signIn = (jwt: string, customer: Customer) => {
    tokenStore.set(jwt)
    setState({ status: 'authed', customer })
  }

  const signOut = async () => {
    try {
      await api.logout()
    } catch {
      // ignore network errors on logout — we still want to clear locally
    }
    tokenStore.clear()
    setState({ status: 'guest' })
  }

  return <AuthContext.Provider value={{ state, signIn, signOut }}>{children}</AuthContext.Provider>
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within <AuthProvider>')
  return ctx
}

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { adminApi } from '../lib/api'
import { useAdminAuth } from '../contexts/AdminAuthContext'

type Step =
  | { kind: 'email' }
  | { kind: 'otp'; email: string; devOtp?: string }

const EMAIL_RE = /^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/

export default function AdminLogin() {
  const { signIn } = useAdminAuth()
  const nav = useNavigate()
  const [step, setStep] = useState<Step>({ kind: 'email' })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [email, setEmail] = useState('')
  const [code, setCode] = useState('')

  async function submitEmail(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!EMAIL_RE.test(email)) {
      setError('Enter a valid email address')
      return
    }
    setBusy(true)
    try {
      const res = await adminApi.requestOtp(email)
      setStep({ kind: 'otp', email, devOtp: res.dev_otp })
      setCode('')
    } catch (e: any) {
      setError(e.message ?? 'Failed to send OTP')
    } finally {
      setBusy(false)
    }
  }

  async function submitOtp(e: React.FormEvent) {
    e.preventDefault()
    if (step.kind !== 'otp') return
    setError(null)
    if (!/^\d{6}$/.test(code)) {
      setError('OTP must be 6 digits')
      return
    }
    setBusy(true)
    try {
      const res = await adminApi.verifyOtp(step.email, code)
      signIn(res.jwt, res.staff)
      nav('/admin', { replace: true })
    } catch (e: any) {
      setError(e.message ?? 'Verification failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-page">
      <div className="login-card">
        <div className="head">
          <h1>Tealogy Admin</h1>
          <div className="tagline">Staff sign-in</div>
        </div>

        <div className="body">
          {step.kind === 'email' && (
            <form onSubmit={submitEmail}>
              <p>Sign in with your registered staff email. Manager and staff both use this page.</p>

              <label className="field-label">Email</label>
              <input
                autoFocus
                type="email"
                inputMode="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value.trim())}
                placeholder="admin@tealogy.in"
                className="field-input"
              />

              {error && <p className="error-text">{error}</p>}
              <button type="submit" disabled={busy} className="btn-primary">
                {busy ? 'Sending OTP…' : 'Send OTP'}
              </button>
            </form>
          )}

          {step.kind === 'otp' && (
            <form onSubmit={submitOtp}>
              <p>
                We've sent a 6-digit code to <strong>{step.email}</strong>. Check your inbox.
              </p>
              {step.devOtp && (
                <div className="dev-banner">
                  Dev mode — your OTP is <code>{step.devOtp}</code>
                </div>
              )}

              <label className="field-label">OTP</label>
              <input
                autoFocus
                type="text"
                inputMode="numeric"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, ''))}
                placeholder="––––––"
                className="field-input otp"
              />
              {error && <p className="error-text">{error}</p>}
              <button type="submit" disabled={busy} className="btn-primary">
                {busy ? 'Verifying…' : 'Verify & continue'}
              </button>
              <button
                type="button"
                onClick={() => setStep({ kind: 'email' })}
                disabled={busy}
                className="btn-secondary"
              >
                Use a different email
              </button>
            </form>
          )}
        </div>
      </div>
    </div>
  )
}

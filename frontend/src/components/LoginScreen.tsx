import { useState } from 'react'
import { api } from '../lib/api'
import { useAuth } from '../contexts/AuthContext'

type Step =
  | { kind: 'email' }
  | { kind: 'otp'; email: string; devOtp?: string }

const EMAIL_RE = /^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$/

export default function LoginScreen({ orgName }: { orgName: string }) {
  const { signIn } = useAuth()
  const [step, setStep] = useState<Step>({ kind: 'email' })
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // email-step state
  const [email, setEmail] = useState('')
  const [phone, setPhone] = useState('') // optional, captured for the cafe's records
  // otp-step state
  const [code, setCode] = useState('')

  async function submitEmail(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    if (!EMAIL_RE.test(email)) {
      setError('Enter a valid email address')
      return
    }
    if (phone && !/^\+[1-9]\d{7,14}$/.test(phone)) {
      setError('Phone (optional) must be in international form, e.g. +919876543210')
      return
    }
    setBusy(true)
    try {
      const res = await api.requestOtp(email, phone || undefined)
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
      const res = await api.verifyOtp(step.email, code)
      signIn(res.jwt, res.customer)
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
          <h1>{orgName}</h1>
          <div className="tagline">Yaar Mera Kulhad</div>
        </div>

        <div className="body">
          {step.kind === 'email' && (
            <form onSubmit={submitEmail}>
              <p>Sign in with your email to view the menu and place an order at your table.</p>

              <label className="field-label">Email</label>
              <input
                autoFocus
                type="email"
                inputMode="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value.trim())}
                placeholder="you@example.com"
                className="field-input"
              />

              <label className="field-label" style={{ marginTop: 14 }}>
                Phone (optional)
              </label>
              <input
                type="tel"
                inputMode="tel"
                autoComplete="tel"
                value={phone}
                onChange={(e) => setPhone(e.target.value.trim())}
                placeholder="+919876543210"
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
                We've sent a 6-digit code to <strong>{step.email}</strong>. Check your inbox
                (and spam folder).
              </p>

              {step.devOtp && (
                <div className="dev-banner">
                  Dev mode — your OTP is <code>{step.devOtp}</code>
                </div>
              )}

              <label className="field-label">Enter OTP</label>
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

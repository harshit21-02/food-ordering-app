import { load } from '@cashfreepayments/cashfree-js'
import { useEffect, useMemo, useState } from 'react'
import { useParams } from 'react-router-dom'
import { api, assetUrl, type ContextResponse, type MenuItem, type Order } from '../lib/api'
import { useAuth } from '../contexts/AuthContext'
import LoginScreen from '../components/LoginScreen'

type Loadable<T> =
  | { kind: 'loading' }
  | { kind: 'ok'; data: T }
  | { kind: 'error'; message: string }

const CATEGORY_ORDER = [
  'Desi Chai',
  'Flavoured Tea',
  'Iced Tea',
  'Coffee',
  'Cold Coffee',
  'Milk',
  'Ice Crush',
  'Cheesecake Milk Shake',
  'Shakes',
  'Fruit Shakes',
  'Coolers',
  'Tea-Snacks',
  'Sandwich',
  'Maggi',
  'Desi',
  'Momos',
  'Corn',
  'Burger',
  'Rice Bowl',
]

const CATEGORY_ICON: Record<string, string> = {
  'Desi Chai': '🍵',
  'Flavoured Tea': '🌿',
  'Iced Tea': '🧊',
  'Coffee': '☕',
  'Cold Coffee': '🧋',
  'Milk': '🥛',
  'Ice Crush': '🍧',
  'Cheesecake Milk Shake': '🍰',
  'Shakes': '🥤',
  'Fruit Shakes': '🍓',
  'Coolers': '🍹',
  'Tea-Snacks': '🍞',
  'Sandwich': '🥪',
  'Maggi': '🍜',
  'Desi': '🥘',
  'Momos': '🥟',
  'Corn': '🌽',
  'Burger': '🍔',
  'Rice Bowl': '🍚',
}

const STATUS_LABELS: Record<string, string> = {
  queued: 'Queued',
  cooking: 'Cooking',
  prepared: 'Ready',
  completed: 'Completed',
  cancelled: 'Cancelled',
}

const GST_RATE = 0.05

const slug = (s: string) => 'cat-' + s.toLowerCase().replace(/[^a-z0-9]+/g, '-')

export default function MenuPage() {
  const { orgId, tableCode } = useParams<{ orgId: string; tableCode: string }>()
  const { state, signOut } = useAuth()

  const [ctx, setCtx] = useState<Loadable<ContextResponse>>({ kind: 'loading' })
  const [menu, setMenu] = useState<Loadable<MenuItem[]>>({ kind: 'loading' })
  const [activeOrder, setActiveOrder] = useState<Order | null>(null)
  const [cart, setCart] = useState<Map<number, number>>(new Map())
  const [placing, setPlacing] = useState(false)
  const [placeError, setPlaceError] = useState<string | null>(null)
  const [justPlaced, setJustPlaced] = useState(false)
  const [pendingPayment, setPendingPayment] = useState<{ amount: number; code: string } | null>(null)
  const [paymentSuccess, setPaymentSuccess] = useState(false)

  useEffect(() => {
    const url = new URL(window.location.href)
    if (url.searchParams.get('paid') === '1') {
      setPaymentSuccess(true)
      url.searchParams.delete('paid')
      window.history.replaceState({}, '', url.toString())
      setTimeout(() => setPaymentSuccess(false), 5000)
    }
  }, [])

  useEffect(() => {
    if (!orgId || !tableCode) return
    api.getContext(orgId, tableCode)
      .then((data) => setCtx({ kind: 'ok', data }))
      .catch((e: Error) => setCtx({ kind: 'error', message: e.message }))
  }, [orgId, tableCode])

  useEffect(() => {
    if (!orgId || !tableCode) return
    if (state.status !== 'authed') return
    api.getMenu(orgId)
      .then((data) => setMenu({ kind: 'ok', data: data.items }))
      .catch((e: Error) => setMenu({ kind: 'error', message: e.message }))
    api.getActiveOrder(orgId, tableCode)
      .then((order) => setActiveOrder(order))
      .catch(() => setActiveOrder(null))
  }, [orgId, tableCode, state.status])

  const grouped = useMemo(() => {
    if (menu.kind !== 'ok') return null
    const groups = new Map<string, MenuItem[]>()
    for (const item of menu.data) {
      const key = item.category ?? 'Uncategorised'
      const list = groups.get(key) ?? []
      list.push(item)
      groups.set(key, list)
    }
    const order = (cat: string) => {
      const i = CATEGORY_ORDER.indexOf(cat)
      return i === -1 ? Number.MAX_SAFE_INTEGER : i
    }
    return Array.from(groups.entries()).sort(([a], [b]) => order(a) - order(b))
  }, [menu])

  const placedByMenuItemID = useMemo(() => {
    const map = new Map<number, number>()
    if (!activeOrder) return map
    for (const it of activeOrder.items) {
      if (!it.menu_item_id) continue
      map.set(it.menu_item_id, (map.get(it.menu_item_id) ?? 0) + it.quantity)
    }
    return map
  }, [activeOrder])

  const cartLines = useMemo(() => {
    if (menu.kind !== 'ok') return []
    const byID = new Map(menu.data.map((m) => [m.id, m]))
    return Array.from(cart.entries())
      .map(([id, delta]) => ({ item: byID.get(id), delta }))
      .filter((x): x is { item: MenuItem; delta: number } => !!x.item && x.delta !== 0)
  }, [cart, menu])

  const cartSubtotal = cartLines.reduce((s, l) => s + l.item.price * l.delta, 0)
  const cartGst = cartSubtotal > 0 ? cartSubtotal * GST_RATE : 0
  const cartGrandTotal = cartSubtotal + cartGst
  const cartCount = cartLines.reduce((s, l) => s + Math.abs(l.delta), 0)

  function changeQty(itemID: number, delta: number) {
    setCart((prev) => {
      const next = new Map<number, number>(prev)
      const cur = next.get(itemID) ?? 0
      const clamped = Math.max(0, cur + delta)
      if (clamped === 0) next.delete(itemID)
      else next.set(itemID, clamped)
      return next
    })
  }

  async function place() {
    if (!ctx || ctx.kind !== 'ok') return
    if (cartLines.length === 0) return
    setPlacing(true)
    setPlaceError(null)
    const subtotal = cartSubtotal
    try {
      const order = await api.placeOrAppend({
        org_id: ctx.data.org.id,
        table_id: ctx.data.table.id,
        items: cartLines.map((l) => ({ menu_item_id: l.item.id, delta: l.delta })),
      })
      setActiveOrder(order.items.length > 0 ? order : null)
      setCart(new Map())
      if (subtotal > 0) {
        const total = parseFloat((subtotal * (1 + GST_RATE)).toFixed(2))
        setPendingPayment({ amount: total, code: order.public_code })
      } else {
        setJustPlaced(true)
        setTimeout(() => setJustPlaced(false), 4000)
      }
      window.scrollTo({ top: 0, behavior: 'smooth' })
    } catch (e: any) {
      setPlaceError(e.message ?? 'Failed to update order')
    } finally {
      setPlacing(false)
    }
  }

  // ----- render -----

  if (ctx.kind === 'loading' || state.status === 'unknown') {
    return (
      <div className="status-wrap">
        <span className="spinner" /> loading…
      </div>
    )
  }

  if (ctx.kind === 'error') {
    return (
      <div className="status-wrap error">
        could not load this table — {ctx.message}
      </div>
    )
  }

  if (state.status !== 'authed') {
    return <LoginScreen orgName={ctx.data.org.name} />
  }

  const customerLabel = state.customer.email
    ?? state.customer.mobile_number
    ?? `Customer #${state.customer.id}`

  return (
    <div className="app-shell">
      <div className="brand-bar">
        <h1>{ctx.data.org.name}</h1>
        <div className="tagline">Yaar Mera Kulhad</div>
        <div className="since">Since 2018</div>
      </div>

      <div className="sub-bar">
        <div className="meta">
          <span className="table-tag">
            {ctx.data.table.label ?? `Table ${ctx.data.table.code}`}
          </span>
          <span style={{ color: 'var(--muted-2)', fontSize: 12 }}>
            {customerLabel}
          </span>
        </div>
        <button className="btn-ghost" onClick={signOut}>Sign out</button>
      </div>

      {grouped && grouped.length > 0 && (
        <nav className="cat-nav">
          {grouped.map(([cat]) => (
            <a key={cat} href={`#${slug(cat)}`} className="cat-chip">
              <span className="ico">{CATEGORY_ICON[cat] ?? '🍴'}</span>
              {cat}
            </a>
          ))}
        </nav>
      )}

      {paymentSuccess && (
        <div className="placed-toast" style={{ background: 'linear-gradient(180deg,#ecfdf5,#d1fae5)', borderColor: '#a7f3d0', color: '#065f46' }}>
          <span style={{ fontSize: 18 }}>✓</span>
          Payment received! Your order is confirmed.
        </div>
      )}

      {justPlaced && (
        <div className="placed-toast">
          <span style={{ fontSize: 18 }}>✓</span>
          Order placed. Kitchen has been notified.
        </div>
      )}

      {activeOrder && <RunningPanel order={activeOrder} />}

      {menu.kind === 'loading' && (
        <div className="status-wrap"><span className="spinner" /> loading menu…</div>
      )}
      {menu.kind === 'error' && (
        <div className="status-wrap error">menu failed to load — {menu.message}</div>
      )}

      {grouped && grouped.map(([category, items]) => (
        <section key={category} id={slug(category)} className="section">
          <div className="section-title">
            <span className="ico">{CATEGORY_ICON[category] ?? '🍴'}</span>
            <h2>{category}</h2>
            <span className="rule" />
          </div>
          <div className="cards">
            {items.map((it) => {
              const placed = placedByMenuItemID.get(it.id) ?? 0
              const delta = cart.get(it.id) ?? 0
              const total = placed + delta
              return (
                <article className="card" key={it.id}>
                  <div className="thumb">
                    {it.image_url ? (
                      <img src={assetUrl(it.image_url)} alt={it.name} />
                    ) : (
                      CATEGORY_ICON[category] ?? '🍴'
                    )}
                  </div>
                  <div className="info">
                    <div className="name">{it.name}</div>
                    {it.description && <div className="desc">{it.description}</div>}
                    <div className="price">
                      <span className="currency">₹</span>{it.price.toFixed(2)}
                    </div>
                  </div>
                  <CardQty
                    total={total}
                    disableMinus={delta <= 0}
                    onMinus={() => changeQty(it.id, -1)}
                    onPlus={() => changeQty(it.id, +1)}
                  />
                </article>
              )
            })}
          </div>
        </section>
      ))}

      {cartCount > 0 && (
        <div className="cart-bar">
          <div className="summary">
            <span className="small">
              {activeOrder ? 'Update your order' : 'Place order'}
            </span>
            {cartCount} item{cartCount > 1 ? 's' : ''}
            {' · '}
            {cartGrandTotal >= 0 ? '+' : '−'}₹{Math.abs(cartGrandTotal).toFixed(2)}
            {cartSubtotal > 0 && (
              <span className="gst-note"> incl. GST</span>
            )}
          </div>
          <button onClick={place} disabled={placing}>
            {placing
              ? 'Saving…'
              : activeOrder
                ? 'Save changes'
                : 'Place order'}
          </button>
        </div>
      )}

      {placeError && (
        <div className="status-wrap error" style={{ paddingTop: 16 }}>
          {placeError}
        </div>
      )}

      {pendingPayment && (
        <PayModal
          amount={pendingPayment.amount}
          code={pendingPayment.code}
          onCounter={() => {
            setPendingPayment(null)
            setJustPlaced(true)
            setTimeout(() => setJustPlaced(false), 4000)
          }}
        />
      )}
    </div>
  )
}

function CardQty({
  total,
  disableMinus,
  onMinus,
  onPlus,
}: {
  total: number
  disableMinus: boolean
  onMinus: () => void
  onPlus: () => void
}) {
  return (
    <div className={`qty${total > 0 ? ' has-items' : ''}`}>
      <button onClick={onMinus} disabled={disableMinus} aria-label="Decrease">−</button>
      <span className="count">{total}</span>
      <button onClick={onPlus} aria-label="Increase">+</button>
    </div>
  )
}

function RunningPanel({ order }: { order: Order }) {
  const gst = order.total_amount * GST_RATE
  const grandTotal = order.total_amount + gst
  return (
    <div className="running-panel">
      <div className="head">
        <div className="left">
          <span className="status-pill">{STATUS_LABELS[order.status] ?? order.status}</span>
          Current order
        </div>
        <span className="code">{order.public_code}</span>
      </div>

      <ul className="lines">
        {order.items.map((it) => (
          <li key={it.id}>
            <span className="qty-px">×{it.quantity}</span>
            <span className="nm">{it.item_name}</span>
            <span className="px">₹{(it.unit_price * it.quantity).toFixed(2)}</span>
          </li>
        ))}
      </ul>

      <div className="total">
        <span className="label">Subtotal</span>
        <span>₹{order.total_amount.toFixed(2)}</span>
      </div>
      <div className="total" style={{ fontSize: 13, marginTop: 2, opacity: 0.85 }}>
        <span className="label">GST (5%)</span>
        <span>₹{gst.toFixed(2)}</span>
      </div>
      <div className="total" style={{ marginTop: 4, paddingTop: 8, borderTop: '1.5px solid rgba(232,212,155,0.8)' }}>
        <span className="label">Total</span>
        <span>₹{grandTotal.toFixed(2)}</span>
      </div>

      <div className="hint">Add items below — tap "Save changes" to update your order.</div>
    </div>
  )
}

function PayModal({
  amount,
  code,
  onCounter,
}: {
  amount: number
  code: string
  onCounter: () => void
}) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const subtotal = amount / (1 + GST_RATE)
  const gst = amount - subtotal

  async function payCashfree() {
    setLoading(true)
    setError(null)
    try {
      const returnUrl = window.location.origin + window.location.pathname + '?paid=1'
      const { payment_session_id } = await api.createPaymentSession(code, returnUrl)
      const cfMode = (import.meta.env.VITE_CASHFREE_ENV ?? 'sandbox') as 'sandbox' | 'production'
      const cashfree = await load({ mode: cfMode })
      cashfree.checkout({ paymentSessionId: payment_session_id, redirectTarget: '_self' })
    } catch (e: any) {
      setError(e.message ?? 'Could not start payment')
      setLoading(false)
    }
  }

  return (
    <div className="modal-backdrop">
      <div className="modal">
        <h3>How would you like to pay?</h3>
        <p className="muted">Order <strong>#{code}</strong> is with the kitchen.</p>
        <div className="pay-breakdown">
          <div className="pb-row">
            <span>Subtotal</span>
            <span>₹{subtotal.toFixed(2)}</span>
          </div>
          <div className="pb-row">
            <span>GST (5%)</span>
            <span>₹{gst.toFixed(2)}</span>
          </div>
          <div className="pb-row pb-total">
            <span>Total</span>
            <span>₹{amount.toFixed(2)}</span>
          </div>
        </div>
        {error && <div className="error-text">{error}</div>}
        <button className="btn-upi" onClick={payCashfree} disabled={loading}>
          {loading ? 'Opening payment…' : `Pay ₹${amount.toFixed(2)} online`}
        </button>
        <button className="btn-secondary" style={{ width: '100%', marginTop: 8, color: 'var(--muted-2)', borderColor: 'var(--border)' }} onClick={onCounter}>
          Pay at counter
        </button>
      </div>
    </div>
  )
}
